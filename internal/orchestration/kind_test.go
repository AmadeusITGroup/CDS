package orchestration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/systemd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner records the shell commands issued and replies from a scripted map
// (prefix match), defaulting to empty output / no error.
type fakeRunner struct {
	ran     []string
	stdins  []string
	replies map[string]string
	fail    map[string]bool
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{replies: map[string]string{}, fail: map[string]bool{}}
}

func (f *fakeRunner) run(command string) (string, error) {
	f.ran = append(f.ran, command)
	for prefix, out := range f.replies {
		if strings.Contains(command, prefix) {
			if f.fail[prefix] {
				return out, assertErr(prefix)
			}
			return out, nil
		}
	}
	return "", nil
}

// runIn records the command and its stdin, then replies like run.
func (f *fakeRunner) runIn(command string, stdin []byte) (string, error) {
	f.stdins = append(f.stdins, string(stdin))
	return f.run(command)
}

type cmdErr string

func (e cmdErr) Error() string { return string(e) }
func assertErr(s string) error { return cmdErr("command failed: " + s) }

func (f *fakeRunner) ranContaining(sub string) bool {
	for _, c := range f.ran {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// kindProviderWithRunner builds a provider whose shell is the fake runner and
// whose supervision unit path is pre-resolved (so tests never touch podman on
// disk). units is left nil: tests here exercise only runner-driven methods.
func kindProviderWithRunner(r *fakeRunner) *kindProvider {
	return &kindProvider{runner: r.run, runnerIn: r.runIn, runnerCtx: r.runCtx, podmanPath: "/usr/bin/podman"}
}

// runCtx records the command, then fails with ctx.Err when ctx is already
// done (what the real runner returns once it has killed the command).
func (f *fakeRunner) runCtx(ctx context.Context, command string) (string, error) {
	if err := ctx.Err(); err != nil {
		f.ran = append(f.ran, command)
		return "", err
	}
	return f.run(command)
}

func TestKindCreateClusterCancelledRemovesPartialCluster(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := k.createCluster(ctx, "")

	require.Error(t, err)
	require.Len(t, r.ran, 2)
	assert.Contains(t, r.ran[0], "kind create cluster --name=cds-kind")
	assert.Contains(t, r.ran[1], "kind delete cluster --name=cds-kind",
		"a killed create must not leave nodes the next Build takes for a cluster")
}

// TestKindCreateClusterCancelledStopsScope: under systemd a cancelled create
// stops its whole scope (kind's podman children too) before the delete, so
// they do not race it (CDS-076).
func TestKindCreateClusterCancelledStopsScope(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX systemd-run stub")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "systemd-run"), []byte("#!/bin/sh\nexit 0\n"), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("INVOCATION_ID", "test")
	r := newFakeRunner()
	k := kindProviderWithRunner(r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.Error(t, k.createCluster(ctx, ""))

	require.Len(t, r.ran, 3)
	assert.Contains(t, r.ran[0], "--unit=cds-kind-create.scope")
	assert.Equal(t, "systemctl --user stop cds-kind-create.scope", r.ran[1])
	assert.Contains(t, r.ran[2], "kind delete cluster --name=cds-kind")
}

// TestKindBoundedWaitsHonourCancel: the API server, ingress webhook and
// service-account waits run under the request ctx, so a cancelled request
// does not hold the cluster lock for up to 3 min (CDS-076).
func TestKindBoundedWaitsHonourCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := kindProviderWithRunner(newFakeRunner()).waitAPIServer(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")

	err = kindProviderWithRunner(newFakeRunner()).DeployIngress(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")

	r := newFakeRunner()
	err = kindProviderWithRunner(r).CreateRegistrySecret(ctx, "registry.cds.internal", 5000, "u", "p")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
	assert.False(t, r.ranContaining("patch serviceaccount"), "nothing runs after a cancelled wait")
}

// TestKindCreateClusterRunsInDelegatedScope: under systemd, kind create runs in
// its own transient scope with Delegate=yes, so the node's conmon does not land
// in cds.service and an agent stop does not hang 90 s on it (CDS-058).
func TestKindCreateClusterRunsInDelegatedScope(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX systemd-run stub")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "systemd-run"), []byte("#!/bin/sh\nexit 0\n"), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("INVOCATION_ID", "test")
	r := newFakeRunner()
	k := kindProviderWithRunner(r)

	require.NoError(t, k.createCluster(context.Background(), ""))

	require.Len(t, r.ran, 1)
	assert.Equal(t,
		"systemd-run --user --scope --collect --quiet --unit=cds-kind-create.scope -p Delegate=yes -- bash -c 'KIND_EXPERIMENTAL_PROVIDER=podman kind create cluster --name=cds-kind'",
		r.ran[0])
}

func TestKindCreateClusterFailureKeepsClusterForDiagnosis(t *testing.T) {
	r := newFakeRunner()
	r.replies["kind create cluster"] = "boom"
	r.fail["kind create cluster"] = true
	k := kindProviderWithRunner(r)

	err := k.createCluster(context.Background(), "")

	require.Error(t, err)
	assert.False(t, r.ranContaining("kind delete cluster"), "only a cancelled create is cleaned up")
}

// TestRunLocalContextKillsCommand runs a real shell: a cancelled ctx must
// stop a long command promptly instead of letting it run to the end.
func TestRunLocalContextKillsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()

	_, err := runLocalContext(ctx, "sleep 30")

	require.Error(t, err)
	assert.Less(t, time.Since(start), 10*time.Second)
}

func TestKindClusterExistsDetectsCluster(t *testing.T) {
	r := newFakeRunner()
	r.replies["kind get clusters"] = "foo\ncds-kind\nbar\n"
	k := kindProviderWithRunner(r)

	exists, err := k.clusterExists()
	require.NoError(t, err)
	assert.True(t, exists)
	assert.True(t, r.ranContaining("KIND_EXPERIMENTAL_PROVIDER=podman kind get clusters"))
}

func TestKindKubeconfigReadsFromControlPlane(t *testing.T) {
	r := newFakeRunner()
	r.replies["kubectl config view"] = "apiVersion: v1\nkind: Config\n"
	k := kindProviderWithRunner(r)

	cfg, err := k.Kubeconfig()
	require.NoError(t, err)
	assert.Contains(t, cfg, "kind: Config")
	assert.True(t, r.ranContaining("podman exec -u root cds-kind-control-plane kubectl config view --minify --flatten"))
}

func TestStatusActionTable(t *testing.T) {
	assert.Equal(t, "none", statusAction(bo.KContainerStatusRunning))
	assert.Equal(t, "start", statusAction(bo.KContainerStatusExited))
	assert.Equal(t, "build", statusAction(bo.KContainerStatusDeleted))
	assert.Equal(t, "build", statusAction(bo.KContainerStatusUnknown))
}

// a Build on an existing but stopped cluster starts it instead of only
// reporting "already exists" (CDS-041); a started or running one is waited on
// until its API server is ready (CDS-050); an unreadable one is left alone.
func TestBringUpExistingClusterStartsAStoppedCluster(t *testing.T) {
	cases := []struct {
		status  bo.ContainerStatus
		started bool
		waited  bool
		last    string
	}{
		{bo.KContainerStatusExited, true, true, "Cluster cds-kind started"},
		{bo.KContainerStatusRunning, false, true, "Cluster cds-kind already exists"},
		{bo.KContainerStatusUnknown, false, false, "Cluster cds-kind already exists"},
	}
	for _, c := range cases {
		var order []string
		var lines []string
		err := bringUpExistingCluster(c.status,
			func() error { order = append(order, "start"); return nil },
			func() error { order = append(order, "ready"); return nil },
			func(_, message string) error { lines = append(lines, message); return nil })
		require.NoError(t, err)
		var want []string
		if c.started {
			want = append(want, "start")
		}
		if c.waited {
			want = append(want, "ready")
		}
		assert.Equal(t, want, order, "status %v", c.status)
		assert.Equal(t, c.last, lines[len(lines)-1], "status %v", c.status)
	}
}

func TestBringUpExistingClusterReportsStartFailure(t *testing.T) {
	err := bringUpExistingCluster(bo.KContainerStatusExited,
		func() error { return cmdErr("no systemd user session") }, func() error { return nil }, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no systemd user session")
}

// an API server that never gets ready fails the Build instead of letting the
// re-apply run against it (CDS-050)
func TestBringUpExistingClusterReportsAPIServerNotReady(t *testing.T) {
	err := bringUpExistingCluster(bo.KContainerStatusExited,
		func() error { return nil }, func() error { return cmdErr("no answer from /readyz") }, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API server is not ready")
}

// the wait loop polls /readyz through the control-plane node until it answers,
// run by a real shell against a stub podman that fails twice (CDS-050)
func TestAPIServerWaitCommandRetriesInARealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	dir := t.TempDir()
	stub := "#!/bin/sh\necho \"$@\" >> " + dir + "/calls\nn=$(wc -l < " + dir + "/calls)\n[ \"$n\" -ge 3 ]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "podman"), []byte(stub), 0o755))

	run := func(tries int) error {
		cmd := exec.Command("/bin/sh", "-c", apiServerWaitCommand(tries, 0))
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
		return cmd.Run()
	}

	require.NoError(t, run(5))
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	assert.Len(t, lines, 3, "stops polling once /readyz answers")
	assert.Equal(t, "exec cds-kind-control-plane kubectl --kubeconfig /etc/kubernetes/admin.conf get --raw /readyz", lines[0])

	require.NoError(t, os.Remove(filepath.Join(dir, "calls")))
	assert.Error(t, run(2), "gives up after the bounded tries")
}

func TestProviderSelectsKind(t *testing.T) {
	p, err := Provider("")
	require.NoError(t, err)
	assert.Equal(t, "kind", p.Name())

	p, err = Provider("kind")
	require.NoError(t, err)
	assert.Equal(t, "kind", p.Name())

	_, err = Provider("k3s")
	assert.Error(t, err)
}

// fakeHostOps is a systemd.HostOps whose systemctl is-active always reports
// inactive, exercising the Stop fallback that stops the node directly.
type fakeHostOps struct{}

func (fakeHostOps) Execute(name string, args ...string) (string, error) {
	// is-active -> "inactive" so UnitManager.IsActive returns false
	return "inactive", nil
}
func (fakeHostOps) Copy(localPath, remotePath string) error { return nil }

func TestKindStopFallsBackToPodmanWhenUnitInactive(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)
	k.units = systemd.NewUnitManager(fakeHostOps{})

	err := k.Stop()
	require.NoError(t, err)
	assert.True(t, r.ranContaining("podman stop cds-kind-control-plane"),
		"when the supervision unit is inactive, Stop stops the node directly")
}

func TestKindDeployIngressAppliesVendoredManifestOnStdin(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)

	require.NoError(t, k.DeployIngress(context.Background()))

	// the immutable one-shot admission Jobs are removed before the apply, so an
	// older install upgrades in place (CDS-028)
	require.GreaterOrEqual(t, len(r.ran), 2)
	assert.Contains(t, r.ran[0], "-n ingress-nginx delete job ingress-nginx-admission-create ingress-nginx-admission-patch --ignore-not-found")
	assert.Contains(t, r.ran[1], "apply -f -")

	// the vendored manifest goes on stdin; nothing is downloaded (CDS-028)
	require.Len(t, r.stdins, 1)
	assert.Equal(t, string(ingressManifest), r.stdins[0])
	assert.True(t, r.ranContaining("podman exec -i cds-kind-control-plane kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f -"))
	for _, c := range r.ran {
		assert.NotContains(t, c, "http", "no runtime manifest fetch")
	}
	// patched for CVE-2025-1974 (fixed in v1.11.5 / v1.12.1), images pinned by digest
	assert.Contains(t, r.stdins[0], "registry.k8s.io/ingress-nginx/controller:v1.15.1@sha256:")
	assert.NotContains(t, r.stdins[0], "controller:v1.10.1")
	assert.True(t, r.ranContaining("patch deployment ingress-nginx-controller"),
		"the node is still prepared after apply")
}

// CDS-062: DeployIngress returns only once the admission webhook answers, so
// the next documented step (create an Ingress) does not fail with
// "failed calling webhook ... connection refused".
func TestKindDeployIngressWaitsForAdmissionWebhook(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)

	require.NoError(t, k.DeployIngress(context.Background()))

	last := r.ran[len(r.ran)-1]
	assert.Contains(t, last, "podman exec cds-kind-control-plane sh -c")
	assert.Contains(t, last, "create ingress cds-ready-probe")
	assert.Contains(t, last, "--dry-run=server", "the probe goes through the webhook but creates nothing")
	assert.Contains(t, last, "seq 1 90", "the wait is bounded")
}

func TestKindDeployIngressFailsWhenWebhookNeverAnswers(t *testing.T) {
	r := newFakeRunner()
	r.replies["create ingress cds-ready-probe"] = "connection refused"
	r.fail["create ingress cds-ready-probe"] = true
	k := kindProviderWithRunner(r)

	err := k.DeployIngress(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ingress controller is not ready")
}

func TestIngressWaitCommandRunsInRealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	// fake podman runs `exec <node> <cmd...>` locally; fake kubectl fails
	// twice (webhook not up), then succeeds
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	podman := "#!/bin/sh\nshift 2\nexec \"$@\"\n"
	kubectl := "#!/bin/sh\necho \"$*\" >> " + filepath.Join(dir, "args") + "\nn=$(cat " + count + " 2>/dev/null || echo 0); n=$((n+1)); echo $n > " + count + "\n[ $n -ge 3 ]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "podman"), []byte(podman), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kubectl"), []byte(kubectl), 0o755))
	path := "PATH=" + dir + ":" + os.Getenv("PATH")

	cmd := exec.Command("/bin/sh", "-c", ingressWaitCommand(5, 0))
	cmd.Env = append(os.Environ(), path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	n, err := os.ReadFile(count)
	require.NoError(t, err)
	assert.Equal(t, "3\n", string(n), "retried until the probe passed")
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	require.NoError(t, err)
	assert.Contains(t, string(args), "--dry-run=server")

	// never ready: bounded, exits non-zero
	require.NoError(t, os.WriteFile(count, []byte("-100\n"), 0o644))
	cmd = exec.Command("/bin/sh", "-c", ingressWaitCommand(3, 0))
	cmd.Env = append(os.Environ(), path)
	require.Error(t, cmd.Run())
}

func TestKindPrepareNodeForIngress(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)

	require.NoError(t, k.prepareNodeForIngress())

	// the node is labelled ingress-ready and the controller is made to tolerate
	// the control-plane taints, so the stock kind manifest schedules on a
	// default single-node cluster.
	assert.True(t, r.ranContaining("label node cds-kind-control-plane ingress-ready=true"),
		"node is labelled ingress-ready")
	assert.True(t, r.ranContaining("patch deployment ingress-nginx-controller"),
		"controller is patched to tolerate taints")
}

func TestKindApplyMirrorsWritesHostsToml(t *testing.T) {
	r := newFakeRunner()
	r.replies["kind get nodes"] = "cds-kind-control-plane\n"
	k := kindProviderWithRunner(r)

	err := k.ApplyMirrors([]Mirror{{Registry: "docker.io", Endpoint: "https://mirror/docker"}})
	require.NoError(t, err)
	assert.True(t, r.ranContaining("certs.d/docker.io"))
	assert.True(t, r.ranContaining("hosts.toml"))
	assert.True(t, r.ranContaining("podman exec cds-kind-control-plane"))
}

// CDS-030: override_path makes containerd skip the /v2 prefix, so it must only
// be set when the endpoint carries its own API path; docker.io's server is
// registry-1.docker.io, not docker.io.
func TestMirrorHostsToml(t *testing.T) {
	cases := []struct {
		name string
		m    Mirror
		want string
	}{
		{
			name: "docker hub, endpoint without path",
			m:    Mirror{Registry: "docker.io", Endpoint: "https://registry-1.docker.io"},
			want: "server = \"https://registry-1.docker.io\"\n\n[host.\"https://registry-1.docker.io\"]\n  capabilities = [\"pull\", \"resolve\"]\n",
		},
		{
			name: "trailing slash is no path",
			m:    Mirror{Registry: "quay.io", Endpoint: "https://mirror.local/"},
			want: "server = \"https://quay.io\"\n\n[host.\"https://mirror.local/\"]\n  capabilities = [\"pull\", \"resolve\"]\n",
		},
		{
			name: "endpoint with API path",
			m:    Mirror{Registry: "reg.local:5000", Endpoint: "http://10.0.0.1:5000/v2/proxy"},
			want: "server = \"https://reg.local:5000\"\n\n[host.\"http://10.0.0.1:5000/v2/proxy\"]\n  capabilities = [\"pull\", \"resolve\"]\n  override_path = true\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, mirrorHostsToml(c.m))
		})
	}
}

func TestKindApplyMirrorsNoOverridePathForBareEndpoint(t *testing.T) {
	r := newFakeRunner()
	r.replies["kind get nodes"] = "cds-kind-control-plane\n"
	k := kindProviderWithRunner(r)

	require.NoError(t, k.ApplyMirrors([]Mirror{{Registry: "docker.io", Endpoint: "https://registry-1.docker.io"}}))
	assert.False(t, r.ranContaining("override_path"), "a bare endpoint keeps containerd's /v2 prefix")
}

func TestKindApplyMirrorsRejectsHostile(t *testing.T) {
	bad := []Mirror{
		{Registry: "docker.io; touch /pwned", Endpoint: "https://m"},
		{Registry: "docker.io/../x", Endpoint: "https://m"},
		{Registry: "docker.io", Endpoint: "file:///etc/passwd"},
		{Registry: "docker.io", Endpoint: "https://"},
		{Registry: "docker.io", Endpoint: "not a url"},
	}
	for _, m := range bad {
		r := newFakeRunner()
		k := kindProviderWithRunner(r)
		require.Error(t, k.ApplyMirrors([]Mirror{m}), "%+v", m)
		assert.Empty(t, r.ran, "nothing runs for %+v", m)
	}
	r := newFakeRunner()
	r.replies["kind get nodes"] = "n\n"
	require.NoError(t, kindProviderWithRunner(r).ApplyMirrors(
		[]Mirror{{Registry: "reg.local:5000", Endpoint: "http://10.0.0.1:5000/v2"}}))
}

func TestKindApplyMirrorsNoopWhenEmpty(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)
	require.NoError(t, k.ApplyMirrors(nil))
	assert.Empty(t, r.ran, "no commands run when there are no mirrors")
}

func TestKindCreateRegistrySecretKeepsPasswordOffCommandLine(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)
	const password = "s3cr3t-Pw"

	require.NoError(t, k.CreateRegistrySecret(context.Background(), "registry.cds.internal", 5000, "cdsu", password))

	// the password must not appear in any command line (ps, /proc, error text)
	for _, c := range r.ran {
		assert.NotContains(t, c, password, "password leaked into command: %s", c)
	}
	assert.True(t, r.ranContaining("podman exec -i cds-kind-control-plane kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f -"))
	assert.True(t, r.ranContaining("patch serviceaccount default"))

	// it goes on stdin, as a dockerconfigjson Secret
	require.Len(t, r.stdins, 1)
	var secret struct {
		Kind     string            `json:"kind"`
		Type     string            `json:"type"`
		Metadata map[string]string `json:"metadata"`
		Data     map[string]string `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.stdins[0]), &secret))
	assert.Equal(t, "Secret", secret.Kind)
	assert.Equal(t, "kubernetes.io/dockerconfigjson", secret.Type)
	assert.Equal(t, "cdsregistrycred", secret.Metadata["name"])
	raw, err := base64.StdEncoding.DecodeString(secret.Data[".dockerconfigjson"])
	require.NoError(t, err)
	var cfg struct {
		Auths map[string]map[string]string `json:"auths"`
	}
	require.NoError(t, json.Unmarshal(raw, &cfg))
	entry := cfg.Auths["registry.cds.internal:5000"]
	assert.Equal(t, "cdsu", entry["username"])
	assert.Equal(t, password, entry["password"])
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("cdsu:"+password)), entry["auth"])
}

func TestKindTrustRegistryOnNodesWritesCA(t *testing.T) {
	r := newFakeRunner()
	r.replies["kind get nodes"] = "cds-kind-control-plane\n"
	k := kindProviderWithRunner(r)

	require.NoError(t, k.TrustRegistryOnNodes("registry.cds.internal", 5000, "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----"))
	assert.True(t, r.ranContaining("/etc/containerd/certs.d/registry.cds.internal:5000/ca.crt"))
}

func TestKindTrustRegistryNoopWhenNoCA(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)
	require.NoError(t, k.TrustRegistryOnNodes("registry.cds.internal", 5000, ""))
	assert.Empty(t, r.ran)
}

func TestKindCreateCommandQuotesConfigFile(t *testing.T) {
	assert.NotContains(t, kindCreateCommand(""), "--config")
	assert.Equal(t,
		"KIND_EXPERIMENTAL_PROVIDER=podman kind create cluster --name=cds-kind --config='x; touch pwned'",
		kindCreateCommand("x; touch pwned"))
	assert.Contains(t, kindCreateCommand("a'b"), `--config='a'\''b'`)
}

// the supervision unit must not be enable-able at boot: an [Install] section
// would let `systemctl --user enable` start the cluster at login (CDS-035).
func TestKindSupervisionUnitHasNoInstallSection(t *testing.T) {
	data, err := kindProviderWithRunner(newFakeRunner()).supervisionUnit()
	require.NoError(t, err)
	unitText := string(data)
	assert.Contains(t, unitText, "ExecStart=/usr/bin/podman start --attach "+ControlPlaneNode)
	assert.NotContains(t, unitText, "[Install]")
	assert.NotContains(t, unitText, "WantedBy")
}
