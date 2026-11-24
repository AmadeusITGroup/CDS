package orchestration

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
	"github.com/amadeusitgroup/cds/internal/systemd"
	"github.com/coreos/go-systemd/v22/unit"
)

const (
	kindEngineName = "kind"
	// kindProviderEnv forces kind onto the podman provider; without it kind
	// defaults to docker. It is still experimental in kind, hence the name.
	kindProviderEnv = "KIND_EXPERIMENTAL_PROVIDER=podman"
	// kindSupervisionUnit is the plain systemd user unit that supervises the
	// kind control-plane node. Quadlet cannot adopt a kind-created node, so a
	// plain unit running `podman start --attach` / `podman stop` is used. It
	// is installed but NOT enabled at boot:
	// restarting a kind node across a reboot has known IP issues (kind#2272,
	// kind#3071), so the cluster is started on demand, not by systemd at boot.
	kindSupervisionUnit = "cds-kind.service"
)

// kindProvider implements ClusterProvider over kind + podman, running entirely
// on the agent host.
type kindProvider struct {
	// engineName is the container engine kind runs on (podman).
	engineName string
	// runner executes a local shell command and returns its output. Injected
	// for tests; defaults to the real local shell.
	runner func(command string) (string, error)
	// runnerIn is runner with stdin, used to hand secrets to a command without
	// putting them on its command line. Injected for tests.
	runnerIn func(command string, stdin []byte) (string, error)
	// runnerCtx is runner bound to a context, used for the long-running
	// cluster creation so a cancelled request kills it.
	runnerCtx func(ctx context.Context, command string) (string, error)
	// units supervises the kind node through a systemd user unit. Injected for
	// tests; defaults to the agent's local host.
	units *systemd.UnitManager
	// podmanPath is the absolute podman path baked into the supervision unit
	// (systemd units need absolute ExecStart paths). Resolved lazily.
	podmanPath string
}

var _ ClusterProvider = (*kindProvider)(nil)

// NewKindProvider returns a kind cluster provider for the agent host.
func NewKindProvider() *kindProvider {
	return &kindProvider{
		engineName: cg.EmptyStr, // default engine (podman) in the engine layer
		runner:     runLocal,
		runnerIn:   runLocalWithInput,
		runnerCtx:  runLocalContext,
		units:      systemd.NewUnitManager(systemd.LocalHost()),
	}
}

func runLocal(command string) (string, error) {
	return shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{&shexec.DefaultShEvent{ExeCmd: command}})
}

func runLocalContext(ctx context.Context, command string) (string, error) {
	return shexec.RunLocalCmdWithOutputContext(ctx, []shexec.ExecuteEvent{&shexec.DefaultShEvent{ExeCmd: command}})
}

func runLocalWithInput(command string, stdin []byte) (string, error) {
	return shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{
		&shexec.StdinShEvent{ExecuteEvent: &shexec.DefaultShEvent{ExeCmd: command}, Input: stdin},
	})
}

func (k *kindProvider) Name() string { return kindEngineName }

// Build creates the cluster if absent, streaming progress. It first ensures the
// IPv4-only `kind` podman network exists (kind would otherwise create an
// IPv6-enabled one that fails on IPv6-disabled hosts), then runs
// `kind create cluster`. kind create is idempotent-ish: it errors if the
// cluster already exists, so Build treats an existing cluster as success and
// starts it when it is stopped (CDS-041).
// Cancelling ctx kills `kind create cluster` (CDS-029).
func (k *kindProvider) Build(ctx context.Context, configFile string, report ProgressReporter) error {
	if err := reportf(report, "network", "Ensuring IPv4-only kind network"); err != nil {
		return err
	}
	if err := engine.EnsureKindNetwork(k.engineName); err != nil {
		return cerr.AppendError("failed to ensure kind network", err)
	}

	if exists, err := k.clusterExists(); err != nil {
		return err
	} else if exists {
		return bringUpExistingCluster(k.liveStatus(), k.Start, report)
	}

	if err := reportf(report, "cluster", fmt.Sprintf("Creating kind cluster %s", ClusterName)); err != nil {
		return err
	}
	if err := k.createCluster(ctx, configFile); err != nil {
		return err
	}
	return reportf(report, "cluster", fmt.Sprintf("Cluster %s created", ClusterName))
}

// createCluster runs `kind create cluster`, killed when ctx is cancelled.
func (k *kindProvider) createCluster(ctx context.Context, configFile string) error {
	out, err := k.runnerCtx(ctx, kindCreateCommand(configFile))
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		// a killed kind create leaves its node containers behind, and the
		// next Build would take them for a ready cluster: remove them.
		k.deletePartialCluster()
	}
	return cerr.AppendErrorFmt("failed to create kind cluster %s: %s", err, ClusterName, out)
}

// deletePartialCluster removes what a cancelled `kind create cluster` left
// behind. Best-effort: the create error is what the caller reports.
func (k *kindProvider) deletePartialCluster() {
	if out, err := k.runner(kindDeleteCommand()); err != nil {
		clog.Warn(fmt.Sprintf("failed to remove partial kind cluster %s: %s", ClusterName, out), err)
	}
}

func kindDeleteCommand() string {
	return fmt.Sprintf("%s kind delete cluster --name=%s", kindProviderEnv, ClusterName)
}

// kindCreateCommand builds the `kind create cluster` command. configFile comes
// from a gRPC request / devcontainer.json (untrusted), so it is shell-quoted.
func kindCreateCommand(configFile string) string {
	cmd := fmt.Sprintf("%s kind create cluster --name=%s", kindProviderEnv, ClusterName)
	if configFile != cg.EmptyStr {
		cmd += " --config=" + shellQuote(configFile)
	}
	return cmd
}

// clusterExists reports whether the kind cluster is already known to kind.
func (k *kindProvider) clusterExists() (bool, error) {
	out, err := k.runner(fmt.Sprintf("%s kind get clusters", kindProviderEnv))
	if err != nil {
		return false, cerr.AppendError("failed to list kind clusters", err)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == ClusterName {
			return true, nil
		}
	}
	return false, nil
}

// Start supervises the (already created) cluster node through its systemd user
// unit. The node is stopped first so `podman start --attach` in the unit owns
// it, then the unit is installed and started.
func (k *kindProvider) Start() error {
	if !k.units.Available() {
		return cerr.NewError("a systemd user session is required to start the cluster")
	}
	// The kind node runs on the `kind` podman network; a `podman system reset`
	// (host maintenance) wipes it. Re-ensure it so start/restart after a reset
	// is self-healing, mirroring Build.
	if err := engine.EnsureKindNetwork(k.engineName); err != nil {
		return cerr.AppendError("failed to ensure kind network", err)
	}
	unitDir, err := k.units.UserUnitDir()
	if err != nil {
		return err
	}
	content, err := k.supervisionUnit()
	if err != nil {
		return err
	}
	if _, err := k.units.Reconcile(unitDir, systemd.Unit{Name: kindSupervisionUnit, Content: content}); err != nil {
		return cerr.AppendError("failed to install kind supervision unit", err)
	}
	// hand a running node to the unit: stop it so ExecStart's `start --attach`
	// re-owns it (a no-op error when already stopped is tolerated).
	if _, err := k.runner(fmt.Sprintf("podman stop %s || true", ControlPlaneNode)); err != nil {
		clog.Debug(fmt.Sprintf("pre-start stop of %s returned: %v", ControlPlaneNode, err))
	}
	if err := k.units.Start(kindSupervisionUnit); err != nil {
		return cerr.AppendError("failed to start kind supervision unit", err)
	}
	return nil
}

// Stop stops the cluster. When the supervision unit is active it is stopped
// through systemd; otherwise (the cluster was built but never started under
// supervision, so the node runs as a plain kind-created container) the node is
// stopped directly. Either way a stopped cluster is the success condition.
func (k *kindProvider) Stop() error {
	if k.units.IsActive(kindSupervisionUnit) {
		if err := k.units.Stop(kindSupervisionUnit); err != nil {
			return cerr.AppendError("failed to stop kind supervision unit", err)
		}
		return nil
	}
	if out, err := k.runner(fmt.Sprintf("podman stop %s", ControlPlaneNode)); err != nil {
		return cerr.AppendErrorFmt("failed to stop cluster node %s: %s", err, ControlPlaneNode, out)
	}
	return nil
}

// Delete removes the supervision unit and the kind cluster. Deleting a kind
// cluster destroys the node container and therefore ALL workloads running in
// it (kind has no persistence for a deleted cluster); this is why the agent
// guards the delete against containers still attached to the cluster unless
// forced. A cluster stop/start, by contrast, only stops the node container and
// preserves the pods. force is accepted for interface parity (the agent decides
// "in use"); kind delete is unconditional and returns zero even if the cluster
// is absent.
func (k *kindProvider) Delete(force bool) error {
	if unitDir, err := k.units.UserUnitDir(); err == nil {
		if err := k.units.Remove(unitDir, kindSupervisionUnit); err != nil {
			clog.Warn("failed to remove kind supervision unit", err)
		}
	}
	if out, err := k.runner(kindDeleteCommand()); err != nil {
		return cerr.AppendErrorFmt("failed to delete kind cluster %s: %s", err, ClusterName, out)
	}
	return nil
}

// Status returns the live cluster view from the control-plane node status.
func (k *kindProvider) Status() (Cluster, error) {
	status, err := engine.GetContainerStatus(ControlPlaneNode, k.engineName)
	if err != nil {
		return Cluster{}, cerr.AppendErrorFmt("failed to get status of %s", err, ControlPlaneNode)
	}
	return Cluster{Name: ClusterName, Node: ControlPlaneNode, Status: status}, nil
}

// Kubeconfig reads the cluster kubeconfig from the control-plane node.
func (k *kindProvider) Kubeconfig() (string, error) {
	cmd := fmt.Sprintf(
		"podman exec -u root %s kubectl config view --minify --flatten",
		ControlPlaneNode,
	)
	out, err := k.runner(cmd)
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("failed to read kind kubeconfig", err)
	}
	return out, nil
}

// registrySecretManifest builds the kubernetes.io/dockerconfigjson Secret that
// `kubectl create secret docker-registry` would generate, as JSON (valid YAML
// for `kubectl apply -f -`).
func registrySecretManifest(server, username, password string) ([]byte, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	dockerConfig, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			server: map[string]string{"username": username, "password": password, "auth": auth},
		},
	})
	if err != nil {
		return nil, cerr.AppendError("failed to encode registry pull secret", err)
	}
	manifest, err := json.Marshal(map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"type":       "kubernetes.io/dockerconfigjson",
		"metadata":   map[string]string{"name": kRegistryPullSecret, "namespace": "default"},
		"data":       map[string]string{".dockerconfigjson": base64.StdEncoding.EncodeToString(dockerConfig)},
	})
	if err != nil {
		return nil, cerr.AppendError("failed to encode registry pull secret", err)
	}
	return manifest, nil
}

// kRegistryPullSecret is the Kubernetes docker-registry secret CDS creates so
// pods can pull from the local secure registry; the default service account is
// patched to use it.
const kRegistryPullSecret = "cdsregistrycred"

// CreateRegistrySecret creates (idempotently) the docker-registry pull secret
// in the default namespace and patches the default service account to
// reference it, so Deployments can pull from the local secure registry without
// per-pod imagePullSecrets.
func (k *kindProvider) CreateRegistrySecret(registryHost string, port int, username, password string) error {
	server := fmt.Sprintf("%s:%d", registryHost, port)
	kc := "kubectl --kubeconfig /etc/kubernetes/admin.conf"

	// The manifest carries the password, so it goes on stdin of
	// `podman exec -i ... apply -f -`, never on a command line: an argument is
	// readable by any local user via ps or /proc, and a failed command is echoed
	// into the error returned to the client.
	manifest, err := registrySecretManifest(server, username, password)
	if err != nil {
		return err
	}
	apply := fmt.Sprintf("podman exec -i %s %s apply -f -", ControlPlaneNode, kc)
	if out, err := k.runnerIn(apply, manifest); err != nil {
		return cerr.AppendErrorFmt("failed to create registry pull secret: %s", err, out)
	}

	// On a freshly created cluster the default service account is created by a
	// controller shortly after the namespace; patching it too early fails with
	// "not found". Wait for it (bounded) before patching.
	waitSA := fmt.Sprintf(
		"podman exec %s sh -c %s",
		ControlPlaneNode,
		shellQuote(fmt.Sprintf("for i in $(seq 1 30); do %s -n default get serviceaccount default >/dev/null 2>&1 && exit 0; sleep 1; done; exit 1", kc)),
	)
	if out, err := k.runner(waitSA); err != nil {
		return cerr.AppendErrorFmt("default service account did not appear: %s", err, out)
	}

	patch := fmt.Sprintf(`{"imagePullSecrets":[{"name":"%s"}]}`, kRegistryPullSecret)
	patchSA := fmt.Sprintf("podman exec %s %s patch serviceaccount default -p %s", ControlPlaneNode, kc, shellQuote(patch))
	if out, err := k.runner(patchSA); err != nil {
		return cerr.AppendErrorFmt("failed to patch default service account: %s", err, out)
	}
	return nil
}

// ingressManifest is the upstream ingress-nginx kind manifest
// (deploy/static/provider/kind/deploy.yaml at tag controller-v1.15.1, the
// last release, patched for CVE-2025-1974), vendored so nothing is fetched at
// runtime (CDS-028). Its images are pinned by digest upstream.
// shortcut: bumped by hand; ingress-nginx is retired, the move off it is A5.
//
//go:embed manifests/ingress-nginx-kind.yaml
var ingressManifest []byte

// DeployIngress installs the nginx ingress controller on the cluster by
// applying the vendored kind manifest via kubectl on the control-plane node,
// on stdin. It is best-effort idempotent (kubectl apply).
func (k *kindProvider) DeployIngress() error {
	// The admission Jobs are one-shot and their pod template is immutable, so
	// applying a newer manifest over a cluster set up by an older one fails
	// with "field is immutable" (CDS-028 upgrade). Delete them first: apply
	// re-creates them, and they are a no-op when the webhook secret exists.
	delJobs := fmt.Sprintf(
		"podman exec %s kubectl --kubeconfig /etc/kubernetes/admin.conf -n ingress-nginx delete job ingress-nginx-admission-create ingress-nginx-admission-patch --ignore-not-found",
		ControlPlaneNode,
	)
	if out, err := k.runner(delJobs); err != nil {
		return cerr.AppendErrorFmt("failed to remove old ingress admission jobs: %s", err, out)
	}
	apply := fmt.Sprintf("podman exec -i %s kubectl --kubeconfig /etc/kubernetes/admin.conf apply -f -", ControlPlaneNode)
	if out, err := k.runnerIn(apply, ingressManifest); err != nil {
		return cerr.AppendErrorFmt("failed to apply ingress on cluster: %s", err, out)
	}
	// On a default single-node kind cluster the only node is the control plane,
	// which carries NoSchedule taints (control-plane and the legacy master). The
	// pinned upstream kind manifest does not tolerate them, so the controller
	// would stay Pending. Patch the controller deployment to tolerate any taint
	// so it schedules on the single node. This is version-independent and does
	// not depend on the (EOL) manifest's own tolerations. Best effort: a patch
	// failure does not fail ingress (the manifest is already applied).
	if err := k.prepareNodeForIngress(); err != nil {
		clog.Warn("failed to prepare node for ingress", err)
	}
	return nil
}

// prepareNodeForIngress makes a default single-node kind cluster able to run
// the nginx ingress controller, which the stock kind manifest only schedules on
// a cluster purpose-built for ingress. Two gaps are closed:
//   - the controller's nodeSelector requires the ingress-ready=true node label,
//     which a default (non-ingress) kind cluster does not set;
//   - the only node is the control plane, whose NoSchedule taints the pinned
//     manifest does not tolerate.
//
// The node is labelled ingress-ready and the controller Deployment is patched
// to tolerate any taint. Best effort and version-independent.
func (k *kindProvider) prepareNodeForIngress() error {
	label := fmt.Sprintf(
		"podman exec %s kubectl --kubeconfig /etc/kubernetes/admin.conf label node %s ingress-ready=true --overwrite",
		ControlPlaneNode, ControlPlaneNode,
	)
	if out, err := k.runner(label); err != nil {
		return cerr.AppendErrorFmt("failed to label node for ingress: %s", err, out)
	}
	patch := `{"spec":{"template":{"spec":{"tolerations":[{"operator":"Exists"}]}}}}`
	tolerate := fmt.Sprintf(
		"podman exec %s kubectl --kubeconfig /etc/kubernetes/admin.conf -n ingress-nginx patch deployment ingress-nginx-controller --type merge -p %s",
		ControlPlaneNode, shellQuote(patch),
	)
	if out, err := k.runner(tolerate); err != nil {
		return cerr.AppendErrorFmt("failed to patch ingress tolerations: %s", err, out)
	}
	return nil
}

// ApplyMirrors installs a containerd pull-through mirror (hosts.toml) for each
// configured registry on every kind node. Image pulls for Registry are then
// served from Endpoint. Idempotent; a no-op when mirrors is empty.
func (k *kindProvider) ApplyMirrors(mirrors []Mirror) error {
	if len(mirrors) == 0 {
		return nil
	}
	for _, m := range mirrors {
		if err := validateMirror(m); err != nil {
			return err
		}
	}
	nodes, err := k.nodes()
	if err != nil {
		return err
	}
	for _, node := range nodes {
		for _, m := range mirrors {
			if err := k.applyMirrorOnNode(node, m); err != nil {
				return cerr.AppendErrorFmt("failed to apply mirror %s on node %s", err, m.Registry, node)
			}
		}
	}
	return nil
}

// TrustRegistryOnNodes writes the registry CA into every kind node's containerd
// per-registry trust dir (/etc/containerd/certs.d/<host>:<port>/ca.crt) so
// containerd can pull from the secure registry over TLS. Without this, pods
// fail with "certificate signed by unknown authority".
func (k *kindProvider) TrustRegistryOnNodes(registryHost string, port int, caPEM string) error {
	if caPEM == cg.EmptyStr {
		return nil
	}
	nodes, err := k.nodes()
	if err != nil {
		return err
	}
	dir := fmt.Sprintf("/etc/containerd/certs.d/%s:%d", registryHost, port)
	for _, node := range nodes {
		script := fmt.Sprintf("set -eu; mkdir -p %s; printf '%%s' %s > %s/ca.crt", dir, shellQuote(caPEM), dir)
		cmd := fmt.Sprintf("podman exec %s sh -c %s", node, shellQuote(script))
		if out, err := k.runner(cmd); err != nil {
			return cerr.AppendErrorFmt("failed to install registry CA on node %s: %s", err, node, out)
		}
	}
	return nil
}

// nodes lists the kind cluster node container names.
func (k *kindProvider) nodes() ([]string, error) {
	out, err := k.runner(fmt.Sprintf("%s kind get nodes --name=%s", kindProviderEnv, ClusterName))
	if err != nil {
		return nil, cerr.AppendError("failed to list kind nodes", err)
	}
	var nodes []string
	for _, line := range strings.Split(out, "\n") {
		if n := strings.TrimSpace(line); n != "" {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

// applyMirrorOnNode writes the containerd hosts.toml for one registry on one
// node. containerd reads /etc/containerd/certs.d/<registry>/hosts.toml.
func (k *kindProvider) applyMirrorOnNode(node string, m Mirror) error {
	dir := shellQuote(fmt.Sprintf("/etc/containerd/certs.d/%s", m.Registry))
	script := fmt.Sprintf("set -eu; mkdir -p %s; printf '%%s' %s > %s/hosts.toml", dir, shellQuote(mirrorHostsToml(m)), dir)
	cmd := fmt.Sprintf("podman exec %s sh -c %s", node, shellQuote(script))
	if out, err := k.runner(cmd); err != nil {
		return cerr.AppendErrorFmt("failed to write hosts.toml: %s", err, out)
	}
	return nil
}

// kDockerHubServer is Docker Hub's registry API host; "docker.io" itself does
// not serve the registry API (containerd maps it the same way).
const kDockerHubServer = "https://registry-1.docker.io"

// mirrorHostsToml renders the containerd hosts.toml for one mirror (CDS-030).
// override_path tells containerd the endpoint already holds the API root, so
// it stops adding /v2: set it only when the endpoint has a path of its own.
func mirrorHostsToml(m Mirror) string {
	server := "https://" + m.Registry
	if m.Registry == "docker.io" {
		server = kDockerHubServer
	}
	toml := fmt.Sprintf("server = %s\n\n[host.%s]\n  capabilities = [\"pull\", \"resolve\"]\n", strconv.Quote(server), strconv.Quote(m.Endpoint))
	if u, err := url.Parse(m.Endpoint); err == nil && strings.Trim(u.Path, "/") != cg.EmptyStr {
		toml += "  override_path = true\n"
	}
	return toml
}

// kRegistryRe matches host[:port]: DNS labels or an IPv4 address, no path.
var kRegistryRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:[0-9]{1,5})?$`)

// validateMirror rejects a mirror whose Registry is not host[:port] or whose
// Endpoint is not an http(s) URL: both come from devcontainer.json and end up
// in a root shell script and a TOML file inside the kind node.
func validateMirror(m Mirror) error {
	if !kRegistryRe.MatchString(m.Registry) {
		return cerr.NewError(fmt.Sprintf("invalid mirror registry %q: expected host[:port]", m.Registry))
	}
	u, err := url.Parse(m.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == cg.EmptyStr {
		return cerr.NewError(fmt.Sprintf("invalid mirror endpoint %q: expected an http(s) URL", m.Endpoint))
	}
	return nil
}

// shellQuote single-quotes s for safe embedding in a shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// supervisionUnit builds the systemd user unit that supervises the kind node.
func (k *kindProvider) supervisionUnit() ([]byte, error) {
	podman, err := k.resolvePodman()
	if err != nil {
		return nil, err
	}
	options := []*unit.UnitOption{
		{Section: "Unit", Name: "Description", Value: "CDS kind cluster node (User)"},
		{Section: "Unit", Name: "After", Value: "network.target"},
		{Section: "Service", Name: "Type", Value: "simple"},
		{Section: "Service", Name: "ExecStart", Value: fmt.Sprintf("%s start --attach %s", podman, ControlPlaneNode)},
		{Section: "Service", Name: "ExecStop", Value: fmt.Sprintf("%s stop %s", podman, ControlPlaneNode)},
		{Section: "Service", Name: "Delegate", Value: "yes"},
		{Section: "Service", Name: "Restart", Value: "no"},
		// installed but intentionally without an [Install] section, so it can
		// never be enabled at boot/login: the cluster is started on demand
		// (kind restart IP issues, kind#2272/#3071; CDS-035).
	}
	data, err := io.ReadAll(unit.Serialize(options))
	if err != nil {
		return nil, cerr.AppendError("failed to serialize kind supervision unit", err)
	}
	return data, nil
}

// resolvePodman returns the absolute podman path for the supervision unit.
func (k *kindProvider) resolvePodman() (string, error) {
	if k.podmanPath != cg.EmptyStr {
		return k.podmanPath, nil
	}
	path, err := exec.LookPath("podman")
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("failed to resolve podman path for kind supervision unit", err)
	}
	k.podmanPath = path
	return path, nil
}

// reportf sends a progress line, tolerating a nil reporter.
func reportf(report ProgressReporter, phase, message string) error {
	if report == nil {
		return nil
	}
	return report(phase, message)
}

// liveStatus is the control-plane node status, Unknown when it cannot be read
// (then Build keeps the existing cluster as it is).
func (k *kindProvider) liveStatus() bo.ContainerStatus {
	cluster, err := k.Status()
	if err != nil {
		clog.Warn(fmt.Sprintf("failed to read status of cluster %s", ClusterName), err)
		return bo.KContainerStatusUnknown
	}
	return cluster.Status
}

// bringUpExistingCluster finishes a Build on a cluster kind already knows: a
// stopped one is started, so `orc run` (and a deploy) never leaves it down
// (CDS-041). Any other status keeps the cluster as it is.
func bringUpExistingCluster(status bo.ContainerStatus, start func() error, report ProgressReporter) error {
	if statusAction(status) != "start" {
		return reportf(report, "cluster", fmt.Sprintf("Cluster %s already exists", ClusterName))
	}
	if err := reportf(report, "cluster", fmt.Sprintf("Cluster %s exists but is stopped, starting it", ClusterName)); err != nil {
		return err
	}
	if err := start(); err != nil {
		return cerr.AppendErrorFmt("failed to start stopped cluster %s", err, ClusterName)
	}
	return reportf(report, "cluster", fmt.Sprintf("Cluster %s started", ClusterName))
}

// statusAction maps a live cluster status to the lifecycle action that would
// bring it to a running, supervised state. Build uses it on an existing
// cluster to start a stopped one.
func statusAction(status bo.ContainerStatus) string {
	switch status {
	case bo.KContainerStatusRunning:
		return "none"
	case bo.KContainerStatusExited:
		return "start"
	case bo.KContainerStatusDeleted, bo.KContainerStatusUnknown:
		return "build"
	default:
		return "build"
	}
}
