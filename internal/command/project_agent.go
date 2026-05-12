package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/bootstrap"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"google.golang.org/grpc"
)

const (
	artifactUploadChunkSize = 1024 * 1024
	projectContainerLabel   = "io.cds.project"
)

type projectDeployPlan struct {
	projectName   string
	configDir     string
	config        *containerconf.Config
	configBytes   []byte
	containerName string
	remoteUser    string
	labels        map[string]string
	artifacts     []artifactCandidate
}

type artifactCandidate struct {
	required   containerconf.RequiredArtifact
	descriptor *cdspb.ArtifactDescriptor
	data       []byte
}

func withProjectAgent(projectName string, callback func(agentServices, context.Context) error) error {
	hostName := projectAgentHost(projectName)
	if hostName == cg.KLocalhost {
		if err := ensureLocalAgentStarted(); err != nil {
			return err
		}
	}
	return stubCallback(callback).executeForHost(hostName)
}

func projectAgentHost(projectName string) string {
	hostName := strings.TrimSpace(db.ProjectHostName(projectName))
	if hostName == cg.EmptyStr {
		return cg.KLocalhost
	}
	return addressHost(hostName)
}

func ensureLocalAgentStarted() error {
	if err := bootstrap.StartAgent(cg.KLocalhost); err != nil {
		if !errors.As(err, &bootstrap.StartOnRunError{}) {
			return err
		}
	}
	return nil
}

func deployProjectOnAgent(ctx context.Context, services agentServices, projectName string) (string, error) {
	plan, err := buildProjectDeployPlan(projectName)
	if err != nil {
		return cg.EmptyStr, err
	}
	return deployProjectPlanOnAgent(ctx, services, plan)
}

func deployProjectPlanOnAgent(ctx context.Context, services agentServices, plan projectDeployPlan) (string, error) {
	if err := stageProjectArtifacts(ctx, services.artifact, plan); err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to stage deployment artifacts", err)
	}

	stream, err := services.container.DeployContainer(ctx, &cdspb.DeployContainerRequest{
		ProjectName:        plan.projectName,
		ContainerName:      plan.containerName,
		Labels:             plan.labels,
		DevcontainerConfig: plan.configBytes,
	})
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to deploy container through agent", err)
	}

	response, err := receiveDeployResponse(stream)
	if err != nil {
		return cg.EmptyStr, err
	}
	if response.GetError() != cg.EmptyStr {
		return cg.EmptyStr, cerr.NewError(response.GetError())
	}

	containerName := response.GetContainerName()
	if containerName == cg.EmptyStr {
		containerName = plan.containerName
	}

	if err := setProjectContainerFromAgent(ctx, services.container, plan.projectName, containerName, plan.remoteUser, bo.KContainerStatusRunning); err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to synchronize deployed container state", err)
	}
	return containerName, nil
}

func receiveDeployResponse(stream grpc.ServerStreamingClient[cdspb.DeployContainerEvent]) (*cdspb.DeployContainerResponse, error) {
	var response *cdspb.DeployContainerResponse
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, cerr.AppendError("Failed to receive deploy progress", err)
		}
		if progress := event.GetProgress(); progress != nil {
			clog.Info(progress.GetMessage())
		}
		if eventResponse := event.GetResponse(); eventResponse != nil {
			response = eventResponse
		}
	}
	if response == nil {
		return nil, cerr.NewError("agent deploy finished without a final response")
	}
	return response, nil
}

func buildProjectDeployPlan(projectName string) (projectDeployPlan, error) {
	configDir := db.ProjectConfig(projectName)
	if strings.TrimSpace(configDir) == cg.EmptyStr {
		return projectDeployPlan{}, cerr.NewError(fmt.Sprintf("Project %q has no devcontainer configuration directory", projectName))
	}

	rawConfig, err := os.ReadFile(projectConfigFile(configDir))
	if err != nil {
		return projectDeployPlan{}, cerr.AppendError(fmt.Sprintf("Failed to read devcontainer configuration for project %s", projectName), err)
	}

	configBytes, err := applyProjectConfigOverrides(projectName, rawConfig)
	if err != nil {
		return projectDeployPlan{}, err
	}

	parsedConfig, err := containerconf.ParseBytes(bytes.NewReader(configBytes))
	if err != nil {
		return projectDeployPlan{}, cerr.AppendError("Failed to parse devcontainer configuration", err)
	}

	artifacts, err := collectArtifactCandidates(parsedConfig, configDir)
	if err != nil {
		return projectDeployPlan{}, cerr.AppendError("Failed to collect deployment artifacts", err)
	}

	containerName := buildProjectDeployContainerName(projectName, projectAgentHost(projectName), parsedConfig)
	return projectDeployPlan{
		projectName:   projectName,
		configDir:     configDir,
		config:        parsedConfig,
		configBytes:   configBytes,
		containerName: containerName,
		remoteUser:    engine.ResolveRemoteUserFromConfig(parsedConfig),
		labels:        projectContainerLabels(projectName),
		artifacts:     artifacts,
	}, nil
}

func projectConfigFile(configDir string) string {
	return filepath.Join(configDir, containerconf.KProjectDefaultConfigFile)
}

func applyProjectConfigOverrides(projectName string, rawConfig []byte) ([]byte, error) {
	overrideTag := strings.TrimSpace(db.OverrideImageTag(projectName))
	if overrideTag == cg.EmptyStr {
		return rawConfig, nil
	}

	var config map[string]any
	if err := json.Unmarshal(stripFullLineJSONComments(rawConfig), &config); err != nil {
		return nil, cerr.AppendError("Failed to apply image tag override to devcontainer configuration", err)
	}
	config[containerconf.KOverrideImageTag] = overrideTag

	updated, err := json.Marshal(config)
	if err != nil {
		return nil, cerr.AppendError("Failed to serialize devcontainer configuration with image tag override", err)
	}
	return updated, nil
}

func stripFullLineJSONComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return []byte(strings.Join(kept, "\n"))
}

func buildProjectDeployContainerName(projectName, hostName string, config *containerconf.Config) string {
	agentHostName, err := bootstrapHostName(hostName)
	if err != nil || agentHostName == cg.EmptyStr {
		agentHostName = hostName
	}
	if agentHostName == cg.KLocalhost {
		if localHostName, hostErr := os.Hostname(); hostErr == nil && localHostName != cg.EmptyStr {
			agentHostName = localHostName
		}
	}
	if config != nil {
		if configuredName, ok := config.Get(containerconf.KName).(string); ok && strings.TrimSpace(configuredName) != cg.EmptyStr {
			return fmt.Sprintf("%s-%s-%s", projectName, strings.TrimSpace(configuredName), agentHostName)
		}
	}
	return engine.GetDevcontainerNameForConfig(projectName, config)
}

func projectContainerLabels(projectName string) map[string]string {
	labels := map[string]string{
		engine.KCDSContainerBasicLabel: engine.KCDSContainerBasicLabelValue,
		projectContainerLabel:          projectName,
	}
	if flavourName := db.ProjectFlavourName(projectName); flavourName != cg.EmptyStr {
		labels[engine.KCDSContainerFlavourNameLabel] = flavourName
	}
	return labels
}

func collectArtifactCandidates(config *containerconf.Config, configDir string) ([]artifactCandidate, error) {
	requiredArtifacts, err := containerconf.CollectArtifacts(config, configDir)
	if err != nil {
		return nil, err
	}

	candidates := make([]artifactCandidate, 0, len(requiredArtifacts))
	for _, required := range requiredArtifacts {
		data, err := artifactSourceData(required.Source)
		if err != nil {
			return nil, cerr.AppendErrorFmt("Failed to read artifact %s", err, required.Identifier)
		}
		candidates = append(candidates, artifactCandidate{
			required: required,
			descriptor: &cdspb.ArtifactDescriptor{
				Identifier: required.Identifier,
				Type:       artifactContentType(data),
				Size:       int64(len(data)),
				Digest:     artifactDigest(data),
			},
			data: data,
		})
	}
	return candidates, nil
}

func artifactSourceData(source containerconf.SourceRef) ([]byte, error) {
	switch source.Type {
	case containerconf.SourceTypeLocalFS:
		return os.ReadFile(source.Ref)
	case containerconf.SourceTypeInline:
		return source.Data, nil
	default:
		return nil, cerr.NewError(fmt.Sprintf("artifact source type %q is not supported", source.Type))
	}
}

func artifactContentType(data []byte) string {
	if len(data) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(data)
}

func artifactDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum)
}

func stageProjectArtifacts(ctx context.Context, client cdspb.ArtifactServiceClient, plan projectDeployPlan) error {
	if len(plan.artifacts) == 0 {
		return nil
	}

	descriptors := make([]*cdspb.ArtifactDescriptor, 0, len(plan.artifacts))
	candidatesByID := make(map[string]artifactCandidate, len(plan.artifacts))
	for _, candidate := range plan.artifacts {
		descriptors = append(descriptors, candidate.descriptor)
		candidatesByID[candidate.descriptor.GetIdentifier()] = candidate
	}

	response, err := client.PrepareArtifacts(ctx, &cdspb.PrepareArtifactsRequest{
		Descriptors: descriptors,
		ProjectName: plan.projectName,
	})
	if err != nil {
		return cerr.AppendError("Failed to prepare artifact staging", err)
	}

	for _, missing := range response.GetMissing() {
		candidate, ok := candidatesByID[missing.GetIdentifier()]
		if !ok {
			return cerr.NewError(fmt.Sprintf("agent requested unknown artifact %q", missing.GetIdentifier()))
		}
		if err := uploadArtifact(ctx, client, plan.projectName, candidate); err != nil {
			return err
		}
	}
	return nil
}

func uploadArtifact(ctx context.Context, client cdspb.ArtifactServiceClient, projectName string, candidate artifactCandidate) error {
	stream, err := client.UploadArtifact(ctx)
	if err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to open upload stream for artifact %s", candidate.descriptor.GetIdentifier()), err)
	}

	uploadID := fmt.Sprintf("%s-%d", strings.ReplaceAll(candidate.descriptor.GetIdentifier(), "/", "-"), time.Now().UnixNano())
	sentAny := false
	for offset := 0; offset < len(candidate.data) || !sentAny; {
		end := offset + artifactUploadChunkSize
		if end > len(candidate.data) {
			end = len(candidate.data)
		}
		chunk := &cdspb.UploadArtifactChunk{
			UploadId: uploadID,
			Offset:   int64(offset),
			Content:  candidate.data[offset:end],
			Finish:   end == len(candidate.data),
		}
		if offset == 0 {
			chunk.ProjectName = projectName
			chunk.Descriptor_ = candidate.descriptor
		}
		if err := stream.Send(chunk); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to upload artifact %s", candidate.descriptor.GetIdentifier()), err)
		}
		sentAny = true
		if end == len(candidate.data) {
			break
		}
		offset = end
	}

	response, err := stream.CloseAndRecv()
	if err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to finish artifact upload %s", candidate.descriptor.GetIdentifier()), err)
	}
	if response.GetReceived().GetDigest() != candidate.descriptor.GetDigest() {
		return cerr.NewError(fmt.Sprintf("artifact %s digest mismatch after upload", candidate.descriptor.GetIdentifier()))
	}
	return nil
}

func syncProjectContainersFromAgent(ctx context.Context, client cdspb.ContainerServiceClient, projectName string, containerNames []string, removeAbsent bool) error {
	return syncProjectContainersFromAgentWithExpected(ctx, client, projectName, containerNames, removeAbsent, bo.KContainerStatusUnknown, false)
}

func syncProjectContainersFromAgentWithExpected(ctx context.Context, client cdspb.ContainerServiceClient, projectName string, containerNames []string, removeAbsent bool, expectedStatus bo.ContainerStatus, overrideExpected bool) error {
	containerNames = uniqueContainerNames(containerNames)
	if len(containerNames) == 0 {
		clog.Info("No containers found in configuration, nothing to sync.")
		return nil
	}

	containersByName, err := listAgentContainersByName(ctx, client)
	if err != nil {
		return err
	}

	absentContainerNames := make([]string, 0)
	for _, containerName := range containerNames {
		remoteUser := db.ProjectContainerRemoteUser(projectName, containerName)
		container, ok := containersByName[containerName]
		if !ok {
			if removeAbsent {
				absentContainerNames = append(absentContainerNames, containerName)
			} else {
				clog.Warn(fmt.Sprintf("Container %q is absent on agent host; keeping local project state.", containerName))
			}
			continue
		}
		containerInfo := protoContainerToBO(container, remoteUser)
		if overrideExpected {
			containerInfo.ExpectedStatus = expectedStatus
		}
		if err := db.UpsertProjectContainer(projectName, containerInfo); err != nil {
			return cerr.AppendErrorFmt("Failed to synchronize project container %s in configuration", err, containerName)
		}
		if err := upsertProjectContainerSSHConfig(projectName, containerInfo, false); err != nil {
			return cerr.AppendErrorFmt("Failed to synchronize SSH config for project container %s", err, containerName)
		}
	}

	if len(absentContainerNames) > 0 {
		if err := db.RemoveProjectContainers(projectName, absentContainerNames); err != nil {
			return cerr.AppendError("Failed to remove absent project containers from configuration", err)
		}
		for _, containerName := range absentContainerNames {
			if err := removeManagedSSHHostEntry(containerName); err != nil {
				return cerr.AppendErrorFmt("Failed to remove SSH config for project container %s", err, containerName)
			}
		}
	}
	return nil
}

func listAgentContainersByName(ctx context.Context, client cdspb.ContainerServiceClient) (map[string]*cdspb.Container, error) {
	response, err := client.ListContainers(ctx, &cdspb.ListContainerRequest{})
	if err != nil {
		return nil, cerr.AppendError("Failed to list containers from agent", err)
	}

	containersByName := make(map[string]*cdspb.Container, len(response.GetContainers()))
	for _, container := range response.GetContainers() {
		if container.GetName() != cg.EmptyStr {
			containersByName[container.GetName()] = container
		}
	}
	return containersByName, nil
}

func startProjectContainersOnAgent(ctx context.Context, client cdspb.ContainerServiceClient, projectName string) error {
	return changeProjectContainersState(ctx, client, projectName, bo.KContainerStatusRunning, func(containerName string, status bo.ContainerStatus) error {
		if status == bo.KContainerStatusRunning {
			clog.Warn(fmt.Sprintf("Container %q is already running. Skipping.", containerName))
			return nil
		}
		_, err := client.StartContainer(ctx, &cdspb.StartContainerRequest{ContainerName: containerName})
		return err
	})
}

func stopProjectContainersOnAgent(ctx context.Context, client cdspb.ContainerServiceClient, projectName string) error {
	return changeProjectContainersState(ctx, client, projectName, bo.KContainerStatusExited, func(containerName string, status bo.ContainerStatus) error {
		if status == bo.KContainerStatusExited || status == bo.KContainerStatusDeleted {
			clog.Warn(fmt.Sprintf("Container %q is not running. Skipping.", containerName))
			return nil
		}
		_, err := client.StopContainer(ctx, &cdspb.StopContainerRequest{ContainerName: containerName})
		return err
	})
}

func changeProjectContainersState(ctx context.Context, client cdspb.ContainerServiceClient, projectName string, expectedStatus bo.ContainerStatus, change func(string, bo.ContainerStatus) error) error {
	containerNames := db.ProjectContainersName(projectName)
	if len(containerNames) == 0 {
		return cerr.NewError(fmt.Sprintf("Project %q has no containers configured", projectName))
	}

	containersByName, err := listAgentContainersByName(ctx, client)
	if err != nil {
		return err
	}

	for _, containerName := range containerNames {
		found, ok := containersByName[containerName]
		if !ok {
			return cerr.NewError(fmt.Sprintf("container %q was not found on agent host", containerName))
		}
		if err := change(containerName, bo.SContainerStatus(found.GetStatus())); err != nil {
			return cerr.AppendErrorFmt("Failed to change state for container %s", err, containerName)
		}
	}

	return syncProjectContainersFromAgentWithExpected(ctx, client, projectName, containerNames, false, expectedStatus, true)
}

func clearProjectContainersOnAgent(ctx context.Context, client cdspb.ContainerServiceClient, projectName string, additionalContainerNames ...string) error {
	containerNames := uniqueContainerNames(append(db.ProjectContainersName(projectName), additionalContainerNames...))
	if len(containerNames) == 0 {
		clog.Warn("No containers found in configuration, nothing to clear.")
		return nil
	}

	containersByName, err := listAgentContainersByName(ctx, client)
	if err != nil {
		return err
	}

	deleteErrors := make([]error, 0)
	deleteCandidates := make([]string, 0, len(containerNames))
	for _, containerName := range containerNames {
		if _, ok := containersByName[containerName]; !ok {
			clog.Warn(fmt.Sprintf("Container %q is already absent on agent host. Skipping.", containerName))
			continue
		}
		deleteCandidates = append(deleteCandidates, containerName)
		if _, err := client.DeleteContainer(ctx, &cdspb.DeleteContainerRequest{ContainerName: containerName}); err != nil {
			deleteErrors = append(deleteErrors, cerr.AppendErrorFmt("Failed to delete container %s", err, containerName))
		}
	}

	if err := syncProjectContainersFromAgent(ctx, client, projectName, deleteCandidates, true); err != nil {
		if len(deleteErrors) == 0 {
			return err
		}
		deleteErrors = append(deleteErrors, err)
	}
	if len(deleteErrors) > 0 {
		return cerr.AppendMultipleErrors("Failed to clear all project containers", deleteErrors)
	}
	return nil
}

func uniqueContainerNames(containerNames []string) []string {
	seen := make(map[string]struct{}, len(containerNames))
	unique := make([]string, 0, len(containerNames))
	for _, containerName := range containerNames {
		containerName = strings.TrimSpace(containerName)
		if containerName == cg.EmptyStr {
			continue
		}
		if _, ok := seen[containerName]; ok {
			continue
		}
		seen[containerName] = struct{}{}
		unique = append(unique, containerName)
	}
	return unique
}

func setProjectContainerFromAgent(ctx context.Context, client cdspb.ContainerServiceClient, projectName, containerName, remoteUser string, expectedStatus bo.ContainerStatus) error {
	response, err := client.GetContainer(ctx, &cdspb.GetContainerRequest{ContainerName: containerName})
	if err != nil {
		return cerr.AppendErrorFmt("Failed to get deployed container %s", err, containerName)
	}

	container := protoContainerToBO(response.GetContainer(), remoteUser)
	container.ExpectedStatus = expectedStatus
	if err := db.UpsertProjectContainer(projectName, container); err != nil {
		return err
	}
	return upsertProjectContainerSSHConfig(projectName, container, true)
}

func protoContainerToBO(container *cdspb.Container, remoteUser string) bo.Container {
	portMapping := make(bo.PortMapping, len(container.GetPortMapping()))
	for containerPort, hostPort := range container.GetPortMapping() {
		portMapping[containerPort] = int(hostPort)
	}
	status := bo.SContainerStatus(container.GetStatus())
	return bo.Container{
		Id:             bo.ContainerID(container.GetId()),
		Name:           bo.ContainerName(container.GetName()),
		Pmapping:       portMapping,
		Status:         status,
		ExpectedStatus: status,
		RemoteUser:     bo.ContainerRemoteUser(remoteUser),
	}
}

func agentVersionForHost(hostName string) (string, error) {
	var version string
	err := stubCallback(func(c agentServices, ctx context.Context) error {
		reply, err := c.info.GetVersion(ctx, &cdspb.GetVersionRequest{})
		if err != nil {
			return cerr.AppendError("Failed to get agent version", err)
		}
		version = reply.GetCurrent()
		return nil
	}).executeForHost(hostName)
	return version, err
}
