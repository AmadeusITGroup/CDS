package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"google.golang.org/grpc"
)

const kArtifactUploadChunkSize = 1024 * 1024

// projectDeployPlan is what the client prepares for the agent to deploy a
// project: the devcontainer.json, sent as is, and the files it references.
type projectDeployPlan struct {
	projectName   string
	configBytes   []byte
	containerName string
	remoteUser    string
	labels        map[string]string
	artifacts     []artifactCandidate
}

type artifactCandidate struct {
	descriptor *cdspb.ArtifactDescriptor
	data       []byte
}

func buildProjectDeployPlan(projectName string) (projectDeployPlan, error) {
	configDir := db.ProjectConfig(projectName)
	if strings.TrimSpace(configDir) == cg.EmptyStr {
		return projectDeployPlan{}, cerr.NewError(fmt.Sprintf("Project %q has no devcontainer configuration directory", projectName))
	}

	configFile := filepath.Join(configDir, containerconf.KProjectDefaultConfigFile)
	configBytes, err := cos.ReadFile(configFile)
	if err != nil {
		return projectDeployPlan{}, cerr.AppendErrorFmt("Failed to read devcontainer configuration %s", err, configFile)
	}
	config, err := containerconf.ParseBytes(bytes.NewReader(configBytes))
	if err != nil {
		return projectDeployPlan{}, cerr.AppendErrorFmt("Failed to parse devcontainer configuration %s", err, configFile)
	}

	artifacts, err := collectArtifactCandidates(config, configDir)
	if err != nil {
		return projectDeployPlan{}, cerr.AppendError("Failed to collect deployment artifacts", err)
	}

	containerName := engine.GetDevcontainerNameForConfig(projectName, config)
	if existing := db.ProjectContainersName(projectName); len(existing) > 0 {
		// keep the name of the deployed container, e.g. when generated
		containerName = existing[0]
	}

	return projectDeployPlan{
		projectName:   projectName,
		configBytes:   configBytes,
		containerName: containerName,
		remoteUser:    engine.ResolveRemoteUserFromConfig(config),
		labels:        projectContainerLabels(projectName),
		artifacts:     artifacts,
	}, nil
}

// projectContainerLabels are added to the labels set by the agent.
func projectContainerLabels(projectName string) map[string]string {
	labels := map[string]string{engine.KCDSContainerProjectLabel: projectName}
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
		contentType := "application/octet-stream"
		if len(data) > 0 {
			contentType = http.DetectContentType(data)
		}
		candidates = append(candidates, artifactCandidate{
			descriptor: &cdspb.ArtifactDescriptor{
				Identifier: required.Identifier,
				Type:       contentType,
				Size:       int64(len(data)),
				Digest:     fmt.Sprintf("sha256:%x", sha256.Sum256(data)),
			},
			data: data,
		})
	}
	return candidates, nil
}

func artifactSourceData(source containerconf.SourceRef) ([]byte, error) {
	switch source.Type {
	case containerconf.SourceTypeLocalFS:
		return cos.ReadFile(source.Ref)
	case containerconf.SourceTypeInline:
		return source.Data, nil
	default:
		return nil, cerr.NewError(fmt.Sprintf("artifact source type %q is not supported", source.Type))
	}
}

// stageProjectArtifacts uploads the artifacts the agent does not have yet.
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

// uploadArtifact streams the artifact in chunks, the first one carrying the
// project and the descriptor.
func uploadArtifact(ctx context.Context, client cdspb.ArtifactServiceClient, projectName string, candidate artifactCandidate) error {
	identifier := candidate.descriptor.GetIdentifier()
	stream, err := client.UploadArtifact(ctx)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to open upload stream for artifact %s", err, identifier)
	}

	uploadID := strings.TrimPrefix(candidate.descriptor.GetDigest(), "sha256:")
	data := candidate.data
	for offset := 0; ; {
		end := min(offset+kArtifactUploadChunkSize, len(data))
		chunk := &cdspb.UploadArtifactChunk{
			UploadId: uploadID,
			Offset:   int64(offset),
			Content:  data[offset:end],
			Finish:   end == len(data),
		}
		if offset == 0 {
			chunk.ProjectName = projectName
			chunk.Descriptor_ = candidate.descriptor
		}
		if err := stream.Send(chunk); err != nil {
			return cerr.AppendErrorFmt("Failed to upload artifact %s", err, identifier)
		}
		if chunk.Finish {
			break
		}
		offset = end
	}

	response, err := stream.CloseAndRecv()
	if err != nil {
		return cerr.AppendErrorFmt("Failed to finish artifact upload %s", err, identifier)
	}
	if response.GetReceived().GetDigest() != candidate.descriptor.GetDigest() {
		return cerr.NewError(fmt.Sprintf("artifact %s digest mismatch after upload", identifier))
	}
	return nil
}

// deployProject stages the artifacts and deploys the project container, then
// records it in the project.
func deployProject(ctx context.Context, services agentServices, plan projectDeployPlan) (string, error) {
	if err := stageProjectArtifacts(ctx, services.artifact, plan); err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to stage deployment artifacts", err)
	}

	stream, err := services.container.DeployContainer(ctx, &cdspb.DeployContainerRequest{
		ProjectName:        plan.projectName,
		ContainerName:      plan.containerName,
		Labels:             plan.labels,
		DevcontainerConfig: plan.configBytes,
		OverrideImageTag:   db.OverrideImageTag(plan.projectName),
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
	if err := recordProjectContainer(ctx, services.container, plan.projectName, containerName, plan.remoteUser, bo.KContainerStatusRunning); err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to record the deployed container", err)
	}
	return containerName, nil
}

// rebuildProject stages the artifacts and has the agent rebuild the project
// container from the current devcontainer.json.
func rebuildProject(ctx context.Context, services agentServices, plan projectDeployPlan) error {
	if err := stageProjectArtifacts(ctx, services.artifact, plan); err != nil {
		return cerr.AppendError("Failed to stage deployment artifacts", err)
	}

	response, err := services.container.RebuildContainer(ctx, &cdspb.RebuildContainerRequest{
		ContainerName:      plan.containerName,
		OverrideImageTag:   db.OverrideImageTag(plan.projectName),
		DevcontainerConfig: plan.configBytes,
	})
	if err != nil {
		return cerr.AppendErrorFmt("Failed to rebuild container %s through agent", err, plan.containerName)
	}
	if response.GetError() != cg.EmptyStr {
		return cerr.NewError(response.GetError())
	}
	return recordProjectContainer(ctx, services.container, plan.projectName, plan.containerName, plan.remoteUser, bo.KContainerStatusRunning)
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
			if progress.GetPhase() == "output" {
				// raw command output streamed from the agent, print as is
				clog.Debug(progress.GetMessage())
			} else {
				clog.Info(progress.GetMessage())
			}
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
