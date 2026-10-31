package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *containerServiceServer) ListContainers(ctx context.Context, req *cdspb.ListContainerRequest) (*cdspb.ListContainersResponse, error) {
	containers, err := s.core.containerOps().ListContainers()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list containers: %v", err)
	}

	pbContainers := make([]*cdspb.Container, 0, len(containers))
	for _, c := range containers {
		pbContainers = append(pbContainers, s.toProto(c))
	}
	return &cdspb.ListContainersResponse{Containers: pbContainers}, nil
}

func (s *containerServiceServer) GetContainer(ctx context.Context, req *cdspb.GetContainerRequest) (*cdspb.GetContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	container, err := s.core.containerOps().GetContainer(req.GetContainerName())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get container %s: %v", req.GetContainerName(), err)
	}
	return &cdspb.GetContainerResponse{Container: s.toProto(container)}, nil
}

func (s *containerServiceServer) StartContainer(ctx context.Context, req *cdspb.StartContainerRequest) (*cdspb.StartContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	if err := s.core.containerOps().StartContainer(req.GetContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "start container %s: %v", req.GetContainerName(), err)
	}
	return &cdspb.StartContainerResponse{Output: fmt.Sprintf("Container %s started", req.GetContainerName())}, nil
}

func (s *containerServiceServer) StopContainer(ctx context.Context, req *cdspb.StopContainerRequest) (*cdspb.StopContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	if err := s.core.containerOps().StopContainer(req.GetContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "stop container %s: %v", req.GetContainerName(), err)
	}
	return &cdspb.StopContainerResponse{Output: fmt.Sprintf("Container %s stopped", req.GetContainerName())}, nil
}

// DeleteContainer removes the container and its recorded deployment. When a
// project is given, the artifacts staged for the project are removed too.
func (s *containerServiceServer) DeleteContainer(ctx context.Context, req *cdspb.DeleteContainerRequest) (*cdspb.DeleteContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}
	if req.GetProjectName() != cg.EmptyStr {
		if err := validateRPCName("project_name", req.GetProjectName()); err != nil {
			return nil, err
		}
	}

	if err := s.core.containerOps().DeleteContainer(req.GetContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete container %s: %v", req.GetContainerName(), err)
	}
	if err := s.core.deployments().remove(req.GetContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete deployment of %s: %v", req.GetContainerName(), err)
	}
	if req.GetProjectName() != cg.EmptyStr {
		if err := s.deleteProjectStaging(req.GetProjectName()); err != nil {
			return nil, status.Errorf(codes.Internal, "delete staged artifacts of project %s: %v", req.GetProjectName(), err)
		}
	}
	return &cdspb.DeleteContainerResponse{Output: fmt.Sprintf("Container %s deleted", req.GetContainerName())}, nil
}

func (s *containerServiceServer) deleteProjectStaging(projectName string) error {
	root, err := projectStagingRoot(s.core.stagingDir(), projectName)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(root); err != nil {
		return cerr.AppendErrorFmt("failed to remove %s", err, root)
	}
	return nil
}

func (s *containerServiceServer) RenameContainer(ctx context.Context, req *cdspb.RenameContainerRequest) (*cdspb.RenameContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}
	if err := validateRPCName("new_container_name", req.GetNewContainerName()); err != nil {
		return nil, err
	}

	if err := s.core.containerOps().RenameContainer(req.GetContainerName(), req.GetNewContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "rename container %s to %s: %v", req.GetContainerName(), req.GetNewContainerName(), err)
	}
	if err := s.core.deployments().rename(req.GetContainerName(), req.GetNewContainerName()); err != nil {
		return nil, status.Errorf(codes.Internal, "rename deployment of %s: %v", req.GetContainerName(), err)
	}
	return &cdspb.RenameContainerResponse{Output: fmt.Sprintf("Container %s renamed to %s", req.GetContainerName(), req.GetNewContainerName())}, nil
}

// ExecuteInContainer runs a command in the container. The command exit code is
// part of the response: only a failure to run the command is an RPC error.
func (s *containerServiceServer) ExecuteInContainer(ctx context.Context, req *cdspb.ExecuteInContainerRequest) (*cdspb.ExecuteInContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetCommand()) == cg.EmptyStr {
		return nil, status.Error(codes.InvalidArgument, "command is required")
	}

	output, exitCode, err := s.core.containerOps().ExecuteInContainer(ctx, req.GetContainerName(), req.GetCommand(), req.GetUser())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "execute in container %s: %v", req.GetContainerName(), err)
	}
	return &cdspb.ExecuteInContainerResponse{
		Output:   boundedHead(output),
		ExitCode: int32(exitCode),
	}, nil
}

// DeployContainer creates the container described by the devcontainer
// configuration, using the artifacts staged for the project, and streams the
// progress. Deploy failures are reported in the final response.
func (s *containerServiceServer) DeployContainer(req *cdspb.DeployContainerRequest, stream grpc.ServerStreamingServer[cdspb.DeployContainerEvent]) error {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return err
	}
	if err := validateRPCName("project_name", req.GetProjectName()); err != nil {
		return err
	}

	spec, err := s.deploySpec(req)
	if err != nil {
		return err
	}

	if err := sendDeployProgress(stream, "init", fmt.Sprintf("Starting deployment of container %s", req.GetContainerName())); err != nil {
		return err
	}

	// When the devcontainer requests orchestration, ensure the cluster and
	// registry (concurrently) and resolve the material decorated into the
	// container, before deploying it.
	if spec.orchestration.Requested {
		decorated, decErr := s.ensureAndResolveDecoration(stream.Context(), spec, func(progress deployProgress) error {
			return sendDeployProgress(stream, progress.phase, progress.message)
		})
		if decErr != nil {
			return sendDeployResponse(stream, deployContainerResult{containerName: req.GetContainerName()}, decErr)
		}
		spec = decorated
	}

	result, err := s.core.containerOps().DeployContainer(stream.Context(), spec, func(progress deployProgress) error {
		return sendDeployProgress(stream, progress.phase, progress.message)
	})
	if err == nil {
		if errRecord := s.core.deployments().save(req); errRecord != nil {
			// the container is deployed, it just can't be rebuilt by the agent
			clog.Warn(fmt.Sprintf("Failed to record deployment of %s, it won't be rebuildable", req.GetContainerName()), errRecord)
		}
	}
	return sendDeployResponse(stream, result, err)
}

// RebuildContainer deletes the container and deploys it again from the
// recorded deployment, or the given configuration, and the artifacts staged
// for its project.
func (s *containerServiceServer) RebuildContainer(ctx context.Context, req *cdspb.RebuildContainerRequest) (*cdspb.RebuildContainerResponse, error) {
	if err := validateRPCName("container_name", req.GetContainerName()); err != nil {
		return nil, err
	}

	deployReq, err := s.core.deployments().load(req.GetContainerName())
	if errors.Is(err, errDeploymentNotFound) {
		return nil, status.Errorf(codes.NotFound, "no deployment recorded for container %s", req.GetContainerName())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load deployment of %s: %v", req.GetContainerName(), err)
	}
	deployReq.OverrideImageTag = req.GetOverrideImageTag()
	if len(req.GetDevcontainerConfig()) > 0 {
		deployReq.DevcontainerConfig = req.GetDevcontainerConfig()
	}

	spec, err := s.deploySpec(deployReq)
	if err != nil {
		return nil, err
	}

	response := &cdspb.RebuildContainerResponse{ContainerName: req.GetContainerName()}
	if err := s.core.containerOps().DeleteContainer(req.GetContainerName()); err != nil {
		response.Error = cerr.AppendErrorFmt("failed to delete container %s", err, req.GetContainerName()).Error()
		return response, nil
	}

	result, err := s.core.containerOps().DeployContainer(ctx, spec, func(progress deployProgress) error {
		clog.Debug(fmt.Sprintf("Rebuilding %s [%s]: %s", req.GetContainerName(), progress.phase, progress.message))
		return nil
	})
	response.Output = result.output
	if err != nil {
		response.Error = boundedHead(err.Error())
		return response, nil
	}
	if errRecord := s.core.deployments().save(deployReq); errRecord != nil {
		clog.Warn(fmt.Sprintf("Failed to record deployment of %s", req.GetContainerName()), errRecord)
	}
	return response, nil
}

// deploySpec builds the engine deploy inputs from a deploy request.
func (s *containerServiceServer) deploySpec(req *cdspb.DeployContainerRequest) (deployContainerSpec, error) {
	config, err := containerconf.ParseBytes(bytes.NewReader(req.GetDevcontainerConfig()))
	if err != nil {
		return deployContainerSpec{}, status.Errorf(codes.InvalidArgument, "invalid devcontainer_config: %v", err)
	}
	if err := rejectUnsupportedFeatures(config); err != nil {
		return deployContainerSpec{}, err
	}
	if err := validateConfigTypes(config); err != nil {
		return deployContainerSpec{}, err
	}
	if tag := strings.TrimSpace(req.GetOverrideImageTag()); tag != cg.EmptyStr {
		config.Set(containerconf.KOverrideImageTag, tag)
	}

	// containers deployed by the agent are always flagged as cds managed
	labels := make(map[string]string, len(req.GetLabels())+1)
	for k, v := range req.GetLabels() {
		labels[k] = v
	}
	labels[engine.KCDSContainerBasicLabel] = engine.KCDSContainerBasicLabelValue

	return deployContainerSpec{
		projectName:   req.GetProjectName(),
		containerName: req.GetContainerName(),
		labels:        labels,
		config:        config,
		orchestration: config.OrchestrationConfig(),
		resourceProvider: stagingResourceProvider{
			stagingDir:  s.core.stagingDir(),
			projectName: req.GetProjectName(),
		},
	}, nil
}

// rejectUnsupportedFeatures fails deployments that declare devcontainer
// features. The agent does not resolve or install features yet (the deploy
// path never supplies engine.WithResolvedFeatures), so accepting them would
// report a successful deployment with the requested tools silently missing.
// Rejecting explicitly is clearer than that silent omission; feature support
// can lift this check once the resolution and staging of feature resources are
// implemented end to end.
func rejectUnsupportedFeatures(config *containerconf.Config) error {
	if config == nil || !config.IsSet(containerconf.KFeatures) {
		return nil
	}
	features, ok := config.Get(containerconf.KFeatures).(map[string]interface{})
	if !ok || len(features) == 0 {
		return nil
	}
	names := make([]string, 0, len(features))
	for name := range features {
		names = append(names, name)
	}
	sort.Strings(names)
	return status.Errorf(codes.Unimplemented,
		"devcontainer features are not supported yet, remove %v from the configuration", names)
}

// validateConfigTypes rejects devcontainer configurations whose values have a
// type the engine cannot use, returning InvalidArgument. This runs before any
// destructive work (RebuildContainer deletes the container before building), so
// a malformed configuration fails cleanly instead of being caught deep in the
// command builder — where an unchecked type assertion would otherwise take the
// whole agent down.
func validateConfigTypes(config *containerconf.Config) error {
	if config == nil {
		return nil
	}
	if config.IsSet(containerconf.KRunArgs) {
		runArgs, ok := config.Get(containerconf.KRunArgs).([]interface{})
		if !ok {
			return status.Errorf(codes.InvalidArgument, "runArgs must be an array of strings")
		}
		for _, arg := range runArgs {
			if _, ok := arg.(string); !ok {
				return status.Errorf(codes.InvalidArgument, "runArgs entries must be strings, got %T (%v)", arg, arg)
			}
		}
	}
	if config.IsSet(containerconf.KAppPort) {
		if err := validateAppPortTypes(config.Get(containerconf.KAppPort)); err != nil {
			return err
		}
	}
	// lifecycle commands and the image are resolved as strings by the engine
	// (engine.expand), which casts interface{} to string: a non-string value
	// (the devcontainer spec also allows a command as an array) would otherwise
	// only fail deep in BuildCommands — after RebuildContainer has already
	// deleted the old container. Reject the unsupported shapes here instead.
	for _, key := range []string{containerconf.KPostCreateCommand, containerconf.KInitializeCommand, containerconf.KImage} {
		if config.IsSet(key) {
			if _, ok := config.Get(key).(string); !ok {
				return status.Errorf(codes.InvalidArgument, "%s must be a string, got %T", key, config.Get(key))
			}
		}
	}
	return nil
}

// validateAppPortTypes accepts appPort as a scalar (string or integer) or an
// array of such scalars, rejecting other shapes with InvalidArgument.
func validateAppPortTypes(raw interface{}) error {
	elements, isList := raw.([]interface{})
	if !isList {
		elements = []interface{}{raw}
	}
	for _, element := range elements {
		switch v := element.(type) {
		case string:
		case int, int64:
		case float64:
			if v != float64(int64(v)) {
				return status.Errorf(codes.InvalidArgument, "appPort %v is not an integer", v)
			}
		default:
			return status.Errorf(codes.InvalidArgument, "appPort entries must be strings or integers, got %T (%v)", element, element)
		}
	}
	return nil
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
		// a progress that could not be sent means the stream is broken
		if _, isStatus := status.FromError(deployErr); isStatus {
			return deployErr
		}
		// the error may embed command output; bound it like the success payload
		response.Error = boundedHead(deployErr.Error())
	}

	if err := stream.Send(&cdspb.DeployContainerEvent{
		Payload: &cdspb.DeployContainerEvent_Response{Response: response},
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send deploy response: %v", err)
	}
	return nil
}

// toProto converts a container into its API representation, reporting the agent host.
func (s *containerServiceServer) toProto(c bo.Container) *cdspb.Container {
	portMapping := make(map[string]int32, len(c.Pmapping))
	for k, v := range c.Pmapping {
		portMapping[k] = int32(v)
	}
	return &cdspb.Container{
		Id:          string(c.Id),
		Name:        string(c.Name),
		Status:      bo.FContainerStatus(c.Status),
		Host:        s.core.hostname(),
		PortMapping: portMapping,
	}
}
