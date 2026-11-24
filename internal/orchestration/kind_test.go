package orchestration

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

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
	return &kindProvider{runner: r.run, runnerIn: r.runIn, podmanPath: "/usr/bin/podman"}
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

	require.NoError(t, k.CreateRegistrySecret("cds.sec.io", 5000, "cdsu", password))

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
	entry := cfg.Auths["cds.sec.io:5000"]
	assert.Equal(t, "cdsu", entry["username"])
	assert.Equal(t, password, entry["password"])
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("cdsu:"+password)), entry["auth"])
}

func TestKindTrustRegistryOnNodesWritesCA(t *testing.T) {
	r := newFakeRunner()
	r.replies["kind get nodes"] = "cds-kind-control-plane\n"
	k := kindProviderWithRunner(r)

	require.NoError(t, k.TrustRegistryOnNodes("cds.sec.io", 5000, "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----"))
	assert.True(t, r.ranContaining("/etc/containerd/certs.d/cds.sec.io:5000/ca.crt"))
}

func TestKindTrustRegistryNoopWhenNoCA(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)
	require.NoError(t, k.TrustRegistryOnNodes("cds.sec.io", 5000, ""))
	assert.Empty(t, r.ran)
}

func TestKindCreateCommandQuotesConfigFile(t *testing.T) {
	assert.NotContains(t, kindCreateCommand(""), "--config")
	assert.Equal(t,
		"KIND_EXPERIMENTAL_PROVIDER=podman kind create cluster --name=cds-kind --config='x; touch pwned'",
		kindCreateCommand("x; touch pwned"))
	assert.Contains(t, kindCreateCommand("a'b"), `--config='a'\''b'`)
}
