package orchestration

import (
	"fmt"
	"io"
	"os/exec"
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
	// plain unit running `podman start --attach` / `podman stop` is used (see
	// the phase-0 supervision spike). It is installed but NOT enabled at boot:
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
		units:      systemd.NewUnitManager(systemd.LocalHost()),
	}
}

func runLocal(command string) (string, error) {
	return shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{&shexec.DefaultShEvent{ExeCmd: command}})
}

func (k *kindProvider) Name() string { return kindEngineName }

// Build creates the cluster if absent, streaming progress. It first ensures the
// IPv4-only `kind` podman network exists (kind would otherwise create an
// IPv6-enabled one that fails on IPv6-disabled hosts), then runs
// `kind create cluster`. kind create is idempotent-ish: it errors if the
// cluster already exists, so Build treats an existing cluster as success.
func (k *kindProvider) Build(configFile string, report ProgressReporter) error {
	if err := reportf(report, "network", "Ensuring IPv4-only kind network"); err != nil {
		return err
	}
	if err := engine.EnsureKindNetwork(k.engineName); err != nil {
		return cerr.AppendError("failed to ensure kind network", err)
	}

	if exists, err := k.clusterExists(); err != nil {
		return err
	} else if exists {
		return reportf(report, "cluster", fmt.Sprintf("Cluster %s already exists", ClusterName))
	}

	if err := reportf(report, "cluster", fmt.Sprintf("Creating kind cluster %s", ClusterName)); err != nil {
		return err
	}
	cmd := fmt.Sprintf("%s kind create cluster --name=%s", kindProviderEnv, ClusterName)
	if configFile != cg.EmptyStr {
		cmd += fmt.Sprintf(" --config=%s", configFile)
	}
	if out, err := k.runner(cmd); err != nil {
		return cerr.AppendErrorFmt("failed to create kind cluster %s: %s", err, ClusterName, out)
	}
	return reportf(report, "cluster", fmt.Sprintf("Cluster %s created", ClusterName))
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

// Delete removes the supervision unit and the kind cluster. force is accepted
// for interface parity (the agent decides "in use"); kind delete is
// unconditional and returns zero even if the cluster is absent.
func (k *kindProvider) Delete(force bool) error {
	if unitDir, err := k.units.UserUnitDir(); err == nil {
		if err := k.units.Remove(unitDir, kindSupervisionUnit); err != nil {
			clog.Warn("failed to remove kind supervision unit", err)
		}
	}
	cmd := fmt.Sprintf("%s kind delete cluster --name=%s", kindProviderEnv, ClusterName)
	if out, err := k.runner(cmd); err != nil {
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
		// installed but intentionally NOT WantedBy any boot target: the cluster
		// is started on demand, not at boot (kind restart IP issues).
		{Section: "Install", Name: "WantedBy", Value: "default.target"},
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

// statusAction maps a live cluster status to the lifecycle action that would
// bring it to a running, supervised state. It is the status->action table the
// agent uses to decide what to do on a run/start request.
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
