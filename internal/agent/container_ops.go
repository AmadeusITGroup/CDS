package agent

import (
	"fmt"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
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
	for _, cmd := range cmds {
		if err := report(deployProgress{phase: "execute", message: deployCommandDescription(cmd)}); err != nil {
			result.output = output.String()
			return result, err
		}

		stdout, err := shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{cmd})
		output.WriteString(stdout)
		if err != nil {
			result.output = output.String()
			return result, cerr.AppendError("failed to execute deploy command", err)
		}
	}

	result.output = output.String()
	return result, nil
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
