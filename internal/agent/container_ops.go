package agent

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

// containerOps abstracts the container engine so that the gRPC layer can be
// tested without running podman/docker.
type containerOps interface {
	ListContainers() (bo.Containers, error)
	GetContainer(name string) (bo.Container, error)
	StartContainer(name string) error
	StopContainer(name string) error
	DeleteContainer(name string) error
	RenameContainer(name, newName string) error
	ExecuteInContainer(containerName, command, user string) (output string, exitCode int, err error)
	DeployContainer(spec deployContainerSpec, report deployProgressReporter) (deployContainerResult, error)
}

type deployResourceProvider interface {
	FetchFile(containerconf.ResourceType, string) ([]byte, error)
	FileExists(containerconf.ResourceType, string) bool
}

type deployContainerSpec struct {
	projectName      string
	containerName    string
	labels           map[string]string
	config           *containerconf.Config
	resourceProvider deployResourceProvider
}

type deployProgress struct {
	phase   string
	message string
}

type deployProgressReporter func(deployProgress) error

type deployContainerResult struct {
	containerName string
	output        string
}

// defaultContainerOps runs the container engine on the agent host.
type defaultContainerOps struct{}

var _ containerOps = defaultContainerOps{}

// ListContainers lists the containers managed by cds only.
func (defaultContainerOps) ListContainers() (bo.Containers, error) {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_PS)
	es.AddArg("--filter", fmt.Sprintf("label=%s=%s", engine.KCDSContainerBasicLabel, engine.KCDSContainerBasicLabelValue))
	es.SetFormat(engine.K_FCONTAINER_ID_STATUS)

	output, err := engine.ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return nil, cerr.AppendError("failed to list containers", err)
	}

	containers, err := engine.ParseContainersInfo(output, engine.K_FCONTAINER_ID_STATUS)
	if err != nil {
		return nil, cerr.AppendError("failed to parse containers info", err)
	}
	return containers, nil
}

func (defaultContainerOps) GetContainer(name string) (bo.Container, error) {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_INSPECT)
	es.SetContainerName(name)
	es.SetFormat(engine.K_FPORT)

	output, err := engine.ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return bo.Container{}, cerr.AppendErrorFmt("failed to inspect container %s", err, name)
	}

	container := bo.Container{Name: bo.ContainerName(name)}
	if err := engine.ParseContainerInfo(output, &container, engine.K_FPORT); err != nil {
		return bo.Container{}, cerr.AppendErrorFmt("failed to parse container info for %s", err, name)
	}

	engineStatus, err := engine.GetContainerStatus(name, cg.EmptyStr)
	if err != nil {
		return bo.Container{}, cerr.AppendErrorFmt("failed to get container status for %s", err, name)
	}
	container.Status = engineStatus
	return container, nil
}

func (defaultContainerOps) StartContainer(name string) error {
	return engine.StartContainer(name)
}

func (defaultContainerOps) StopContainer(name string) error {
	return engine.StopContainer(name)
}

func (defaultContainerOps) DeleteContainer(name string) error {
	return engine.DeleteContainer(name)
}

func (defaultContainerOps) RenameContainer(name, newName string) error {
	return engine.RenameContainer(name, newName)
}

// ExecuteInContainer runs command in the container and returns its output and
// exit code. A non-zero exit code is a command result, not an error.
func (defaultContainerOps) ExecuteInContainer(containerName, command, user string) (string, int, error) {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_EXE)
	es.SetExecuteCmd(engine.K_EXEC_CUSTOM_CMD)
	es.SetContainerName(containerName)
	es.SetCustomCommand(command)
	if user != cg.EmptyStr {
		es.SetRemoteUser(user)
	}

	cmds, err := es.BuildCommands()
	if err != nil {
		return cg.EmptyStr, -1, cerr.AppendError("failed to build execute command", err)
	}
	if len(cmds) != 1 {
		return cg.EmptyStr, -1, cerr.NewError(fmt.Sprintf("expected 1 execute command, got %d", len(cmds)))
	}
	return shexec.RunLocalCmdWithExitCode(cmds[0])
}

func (defaultContainerOps) DeployContainer(spec deployContainerSpec, report deployProgressReporter) (deployContainerResult, error) {
	result := deployContainerResult{containerName: spec.containerName}

	ce := engine.NewContainerEngine(
		engine.WithContainerConfig(spec.config),
		engine.WithResourceProvider(spec.resourceProvider),
	)
	ce.SetAction(engine.K_ACTION_RUN)
	ce.SetRunType(engine.K_RUN_DEV_CONTAINER)
	ce.SetContainerName(spec.containerName)
	ce.AddContainerLabels(spec.labels)

	cmds, err := ce.BuildCommands()
	if err != nil {
		return result, cerr.AppendError("failed to build deploy commands", err)
	}
	sshKeyCommands, err := buildSSHKeyInstallCommands(spec)
	if err != nil {
		return result, cerr.AppendError("failed to build SSH key install commands", err)
	}
	cmds = append(cmds, sshKeyCommands...)

	var output strings.Builder
	created := false
	for _, cmd := range cmds {
		if err := report(deployProgress{phase: "execute", message: deployCommandDescription(cmd)}); err != nil {
			result.output = output.String()
			compensateFailedDeploy(created, spec.containerName)
			return result, err
		}

		stdout, runErr := shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{detachContainerHelpers(cmd)})
		output.WriteString(stdout)
		if runErr != nil {
			result.output = output.String()
			// Only clean up a container this deployment created. A creation
			// failure (e.g. a name collision with a pre-existing container)
			// must not remove that other container.
			if commandCreatesContainer(cmd) {
				return result, cerr.AppendError("failed to create container", runErr)
			}
			compensateFailedDeploy(created, spec.containerName)
			return result, cerr.AppendError("failed to execute deploy command", runErr)
		}
		if commandCreatesContainer(cmd) {
			created = true
		}
	}

	result.output = output.String()
	return result, nil
}

// commandCreatesContainer reports whether running cmd brings the container into
// existence, i.e. the engine `run` step.
func commandCreatesContainer(cmd shexec.ExecuteEvent) bool {
	creator, ok := cmd.(interface{ CreatesContainer() bool })
	return ok && creator.CreatesContainer()
}

// detachContainerHelpers wraps the container-creating command so that the
// container runtime's long-lived helpers (conmon, rootlessport) start in their
// own transient systemd scope instead of the agent's service cgroup. Without
// this, restarting or stopping cds.service (e.g. during an agent upgrade) kills
// those helpers under the default KillMode=control-group, leaving a running
// container with no host port listener. The agent's lifetime is thus decoupled
// from the containers it deploys.
//
// Only the creating command is wrapped, and only when the agent itself runs
// under systemd (INVOCATION_ID is set for unit processes) and systemd-run is
// available. Otherwise the command is returned unchanged.
func detachContainerHelpers(cmd shexec.ExecuteEvent) shexec.ExecuteEvent {
	if !commandCreatesContainer(cmd) || !runningUnderSystemd() || !systemdRunAvailable() {
		return cmd
	}
	// --scope runs the command in a transient scope unit; --collect reclaims it
	// once it exits. The scope is a sibling of cds.service, so a restart of the
	// service no longer tears down the container helpers placed in this scope.
	wrapped := fmt.Sprintf("systemd-run --user --scope --collect --quiet -- bash -c %s", cg.ShellSingleQuote(cmd.Cmd()))
	return &shexec.DefaultShEvent{ExeCmd: wrapped, DescriptionCmd: cmd.Description()}
}

// runningUnderSystemd reports whether this process runs inside a systemd unit.
// systemd exports INVOCATION_ID to the processes of the units it starts.
func runningUnderSystemd() bool {
	return os.Getenv("INVOCATION_ID") != cg.EmptyStr
}

func systemdRunAvailable() bool {
	_, err := exec.LookPath("systemd-run")
	return err == nil
}

// compensateFailedDeploy removes a container created earlier in a deployment
// that then failed, so that a partial deploy does not leave an unmanaged
// container behind. It is best effort: a container the client never recorded
// could otherwise only be removed by hand. Nothing is removed when the
// container was not created by this deployment.
func compensateFailedDeploy(created bool, containerName string) {
	if !created {
		return
	}
	if err := engine.DeleteContainer(containerName); err != nil {
		clog.Warn(fmt.Sprintf("Failed to remove container %s after a failed deployment, it may need manual cleanup", containerName), err)
	}
}

// buildSSHKeyInstallCommands installs the staged public key, if any, for the
// remote user of the container.
func buildSSHKeyInstallCommands(spec deployContainerSpec) ([]shexec.ExecuteEvent, error) {
	identifier, err := containerconf.SingletonIdentifier(containerconf.KindPubKey)
	if err != nil {
		return nil, err
	}
	if spec.resourceProvider == nil || !spec.resourceProvider.FileExists(containerconf.ResourceTypeFile, identifier) {
		return nil, nil
	}

	ce := engine.NewContainerEngine(
		engine.WithContainerConfig(spec.config),
		engine.WithResourceProvider(spec.resourceProvider),
	)
	ce.SetAction(engine.K_ACTION_EXE)
	ce.SetExecuteCmd(engine.K_EXEC_CMD_SSH)
	ce.SetContainerName(spec.containerName)
	ce.SetRemoteUser(engine.ResolveRemoteUserFromConfig(spec.config))
	return ce.BuildCommands()
}

func deployCommandDescription(cmd shexec.ExecuteEvent) string {
	if description := strings.TrimSpace(cmd.Description()); description != cg.EmptyStr {
		return description
	}
	return "Running deploy command"
}
