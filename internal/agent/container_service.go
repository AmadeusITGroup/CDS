package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/engine"
	"github.com/amadeusitgroup/cds/internal/shexec"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ContainerOps abstracts container engine operations so the gRPC service
// layer can be tested without running real docker/podman commands.
type ContainerOps interface {
	ListContainers() (bo.Containers, error)
	GetContainer(name string) (bo.Container, error)
	StartContainer(name string) error
	StopContainer(name string) error
	DeleteContainer(name string) error
	RenameContainer(name, newName string) error
	ExecuteInContainer(containerName, command, user string) (string, int, error)
	DeployContainer(spec deployContainerSpec, report deployProgressReporter) (deployContainerResult, error)
}

type defaultContainerOps struct{}

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

func (d defaultContainerOps) ListContainers() (bo.Containers, error) {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_PS)
	es.SetFormat(engine.K_FCONTAINER_ID_STATUS)

	output, err := engine.ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	containers, err := engine.ParseContainersInfo(output, engine.K_FCONTAINER_ID_STATUS)
	if err != nil {
		return nil, fmt.Errorf("failed to parse containers info: %w", err)
	}
	return containers, nil
}

func (d defaultContainerOps) GetContainer(name string) (bo.Container, error) {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_INSPECT)
	es.SetContainerName(name)
	es.SetFormat(engine.K_FPORT)

	output, err := engine.ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return bo.Container{}, fmt.Errorf("failed to inspect container %s: %w", name, err)
	}

	container := bo.Container{Name: bo.ContainerName(name)}
	if err := engine.ParseContainerInfo(output, &container, engine.K_FPORT); err != nil {
		return bo.Container{}, fmt.Errorf("failed to parse container info for %s: %w", name, err)
	}

	engineStatus, err := engine.GetContainerStatus(name, "")
	if err != nil {
		return bo.Container{}, fmt.Errorf("failed to get container status for %s: %w", name, err)
	}
	container.Status = engineStatus
	return container, nil
}

func (d defaultContainerOps) StartContainer(name string) error {
	return engine.StartContainer(name)
}

func (d defaultContainerOps) StopContainer(name string) error {
	return engine.StopContainer(name)
}

func (d defaultContainerOps) DeleteContainer(name string) error {
	return engine.DeleteContainer(name)
}

func (d defaultContainerOps) RenameContainer(name, newName string) error {
	return engine.RenameContainer(name, newName)
}

func (d defaultContainerOps) ExecuteInContainer(containerName, command, user string) (string, int, error) {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_EXE)
	es.SetExecuteCmd(engine.K_EXEC_CUSTOM_CMD)
	es.SetContainerName(containerName)
	es.SetCustomCommand(command)
	if user != "" {
		es.SetRemoteUser(user)
	}

	output, err := engine.ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if err != nil {
		// TODO: extract actual exit code from exec error when available
		return output, 1, err
	}
	return output, 0, nil
}

func (d defaultContainerOps) DeployContainer(spec deployContainerSpec, report deployProgressReporter) (deployContainerResult, error) {
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
		return deployContainerResult{containerName: spec.containerName}, cerr.AppendError("failed to build deploy commands", err)
	}
	sshKeyCommands, err := buildSSHKeyInstallCommands(spec)
	if err != nil {
		return deployContainerResult{containerName: spec.containerName}, cerr.AppendError("failed to build SSH key install commands", err)
	}
	cmds = append(cmds, sshKeyCommands...)

	var output strings.Builder
	for _, cmd := range cmds {
		if report != nil {
			if err := report(deployProgress{phase: "execute", message: deployCommandDescription(cmd)}); err != nil {
				return deployContainerResult{containerName: spec.containerName, output: output.String()}, err
			}
		}

		stdout, err := shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{cmd})
		if stdout != "" {
			output.WriteString(stdout)
		}
		if err != nil {
			return deployContainerResult{
				containerName: spec.containerName,
				output:        output.String(),
			}, cerr.AppendError("failed to execute deploy command", err)
		}
	}

	return deployContainerResult{
		containerName: spec.containerName,
		output:        output.String(),
	}, nil
}

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
	if description := strings.TrimSpace(cmd.Description()); description != "" {
		return description
	}
	return "Running deploy command"
}

func newDefaultContainerOps() ContainerOps {
	return defaultContainerOps{}
}

func (s *containerServiceServer) ops() ContainerOps {
	if s.engineOps == nil {
		return newDefaultContainerOps()
	}
	return s.engineOps
}

func parseDeployContainerConfig(raw []byte) (*containerconf.Config, error) {
	config, err := containerconf.ParseBytes(bytes.NewReader(raw))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid devcontainer_config: %v", err)
	}
	return config, nil
}

func sendDeployProgress(stream grpc.ServerStreamingServer[cdspb.DeployContainerEvent], phase, message string) error {
	if err := stream.Send(&cdspb.DeployContainerEvent{
		Payload: &cdspb.DeployContainerEvent_Progress{
			Progress: &cdspb.DeployContainerProgress{
				Phase:   phase,
				Message: message,
			},
		},
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send progress: %v", err)
	}
	return nil
}

func sendDeployResponse(stream grpc.ServerStreamingServer[cdspb.DeployContainerEvent], result deployContainerResult, deployErr error) error {
	response := &cdspb.DeployContainerResponse{
		ContainerName: result.containerName,
		Output:        result.output,
	}
	if deployErr != nil {
		response.Error = deployErr.Error()
	}

	if err := stream.Send(&cdspb.DeployContainerEvent{
		Payload: &cdspb.DeployContainerEvent_Response{Response: response},
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send deploy response: %v", err)
	}
	return nil
}

func (s *containerServiceServer) ListContainers(ctx context.Context, req *cdspb.ListContainerRequest) (*cdspb.ListContainersResponse, error) {
	containers, err := s.ops().ListContainers()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list containers: %v", err)
	}

	hostname, _ := os.Hostname()
	pbContainers := make([]*cdspb.Container, 0, len(containers))
	for _, c := range containers {
		pbContainers = append(pbContainers, boContainerToProto(c, hostname))
	}
	return &cdspb.ListContainersResponse{Containers: pbContainers}, nil
}

func (s *containerServiceServer) GetContainer(ctx context.Context, req *cdspb.GetContainerRequest) (*cdspb.GetContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	container, err := s.ops().GetContainer(req.GetContainerName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get container %s: %v", req.GetContainerName(), err)
	}

	hostname, _ := os.Hostname()
	return &cdspb.GetContainerResponse{Container: boContainerToProto(container, hostname)}, nil
}

func (s *containerServiceServer) StartContainer(ctx context.Context, req *cdspb.StartContainerRequest) (*cdspb.StartContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	if err := s.ops().StartContainer(req.GetContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "start container %s: %v", req.GetContainerName(), err)
	}
	return &cdspb.StartContainerResponse{Output: fmt.Sprintf("Container %s started", req.GetContainerName())}, nil
}

func (s *containerServiceServer) StopContainer(ctx context.Context, req *cdspb.StopContainerRequest) (*cdspb.StopContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	if err := s.ops().StopContainer(req.GetContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "stop container %s: %v", req.GetContainerName(), err)
	}
	return &cdspb.StopContainerResponse{Output: fmt.Sprintf("Container %s stopped", req.GetContainerName())}, nil
}

func (s *containerServiceServer) DeleteContainer(ctx context.Context, req *cdspb.DeleteContainerRequest) (*cdspb.DeleteContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	if err := s.ops().DeleteContainer(req.GetContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete container %s: %v", req.GetContainerName(), err)
	}
	return &cdspb.DeleteContainerResponse{Output: fmt.Sprintf("Container %s deleted", req.GetContainerName())}, nil
}

func (s *containerServiceServer) RenameContainer(ctx context.Context, req *cdspb.RenameContainerRequest) (*cdspb.RenameContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}
	if err := validateRPCName("new_container_name", req.GetNewContainerName()); err != nil {
		return nil, err
	}

	if err := s.ops().RenameContainer(req.GetContainerName(), req.GetNewContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "rename container %s to %s: %v", req.GetContainerName(), req.GetNewContainerName(), err)
	}
	return &cdspb.RenameContainerResponse{Output: fmt.Sprintf("Container %s renamed to %s", req.GetContainerName(), req.GetNewContainerName())}, nil
}

func (s *containerServiceServer) ExecuteInContainer(ctx context.Context, req *cdspb.ExecuteInContainerRequest) (*cdspb.ExecuteInContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}
	if req.GetCommand() == "" {
		return nil, status.Error(codes.InvalidArgument, "command is required")
	}

	output, exitCode, err := s.ops().ExecuteInContainer(req.GetContainerName(), req.GetCommand(), req.GetUser())
	if err != nil {
		return &cdspb.ExecuteInContainerResponse{
			Output:   output,
			ExitCode: int32(exitCode),
		}, nil
	}
	return &cdspb.ExecuteInContainerResponse{
		Output:   output,
		ExitCode: int32(exitCode),
	}, nil
}

func (s *containerServiceServer) DeployContainer(req *cdspb.DeployContainerRequest, stream grpc.ServerStreamingServer[cdspb.DeployContainerEvent]) error {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return err
	}
	if err := validateRPCName("project_name", req.GetProjectName()); err != nil {
		return err
	}

	config, err := parseDeployContainerConfig(req.GetDevcontainerConfig())
	if err != nil {
		return err
	}

	if err := sendDeployProgress(stream, "init", fmt.Sprintf("Starting deployment of container %s", req.GetContainerName())); err != nil {
		return err
	}

	spec := deployContainerSpec{
		projectName:      req.GetProjectName(),
		containerName:    req.GetContainerName(),
		labels:           req.GetLabels(),
		config:           config,
		resourceProvider: newStagingResourceProvider(s.stagingDir, req.GetProjectName()),
	}
	result, err := s.ops().DeployContainer(spec, func(progress deployProgress) error {
		return sendDeployProgress(stream, progress.phase, progress.message)
	})
	if err != nil {
		if _, ok := status.FromError(err); ok {
			return err
		}
		return sendDeployResponse(stream, result, err)
	}

	return sendDeployResponse(stream, result, nil)
}

func (s *containerServiceServer) RebuildContainer(ctx context.Context, req *cdspb.RebuildContainerRequest) (*cdspb.RebuildContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	return nil, status.Error(codes.Unimplemented, "rebuild pipeline is not wired to cached deploy state yet")
}

// boContainerToProto converts a business object container to its protobuf representation.
func boContainerToProto(c bo.Container, hostname string) *cdspb.Container {
	portMapping := make(map[string]int32, len(c.Pmapping))
	for k, v := range c.Pmapping {
		portMapping[k] = int32(v)
	}
	return &cdspb.Container{
		Id:          string(c.Id),
		Name:        string(c.Name),
		Status:      bo.FContainerStatus(c.Status),
		Host:        hostname,
		PortMapping: portMapping,
	}
}
