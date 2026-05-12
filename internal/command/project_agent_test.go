package command

import (
	"context"
	"io"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestStageProjectArtifactsUploadsOnlyMissingArtifacts(t *testing.T) {
	ctx := context.Background()
	uploaded := make([]*cdspb.UploadArtifactChunk, 0)
	client := &recordingArtifactClient{
		missing: []string{"dockerfile"},
		onUpload: func(chunks []*cdspb.UploadArtifactChunk) {
			uploaded = append(uploaded, chunks...)
		},
	}
	dockerfileDescriptor := &cdspb.ArtifactDescriptor{
		Identifier: "dockerfile",
		Type:       "text/plain",
		Size:       14,
		Digest:     artifactDigest([]byte("FROM scratch\n")),
	}
	sshDescriptor := &cdspb.ArtifactDescriptor{
		Identifier: "ssh/public-key",
		Type:       "text/plain",
		Size:       7,
		Digest:     artifactDigest([]byte("ssh-rsa")),
	}

	err := stageProjectArtifacts(ctx, client, projectDeployPlan{
		projectName: "project-a",
		artifacts: []artifactCandidate{
			{
				required:   containerconf.RequiredArtifact{Identifier: "dockerfile"},
				descriptor: dockerfileDescriptor,
				data:       []byte("FROM scratch\n"),
			},
			{
				required:   containerconf.RequiredArtifact{Identifier: "ssh/public-key"},
				descriptor: sshDescriptor,
				data:       []byte("ssh-rsa"),
			},
		},
	})
	require.NoError(t, err)

	require.NotNil(t, client.prepareRequest)
	assert.Equal(t, "project-a", client.prepareRequest.GetProjectName())
	assert.Len(t, client.prepareRequest.GetDescriptors(), 2)
	if assert.Len(t, uploaded, 1) {
		assert.Equal(t, "project-a", uploaded[0].GetProjectName())
		assert.Equal(t, dockerfileDescriptor.GetIdentifier(), uploaded[0].GetDescriptor_().GetIdentifier())
		assert.Equal(t, []byte("FROM scratch\n"), uploaded[0].GetContent())
		assert.True(t, uploaded[0].GetFinish())
	}
}

func TestReceiveDeployResponseReturnsFinalResponse(t *testing.T) {
	response, err := receiveDeployResponse(&deployEventStream{
		events: []*cdspb.DeployContainerEvent{
			{Payload: &cdspb.DeployContainerEvent_Progress{Progress: &cdspb.DeployContainerProgress{Message: "building"}}},
			{Payload: &cdspb.DeployContainerEvent_Response{Response: &cdspb.DeployContainerResponse{ContainerName: "container-a"}}},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "container-a", response.GetContainerName())
}

func TestReceiveDeployResponseErrorsWithoutFinalResponse(t *testing.T) {
	response, err := receiveDeployResponse(&deployEventStream{
		events: []*cdspb.DeployContainerEvent{
			{Payload: &cdspb.DeployContainerEvent_Progress{Progress: &cdspb.DeployContainerProgress{Message: "building"}}},
		},
	})

	require.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "without a final response")
}

type recordingArtifactClient struct {
	cdspb.ArtifactServiceClient
	missing        []string
	prepareRequest *cdspb.PrepareArtifactsRequest
	onUpload       func([]*cdspb.UploadArtifactChunk)
}

func (c *recordingArtifactClient) PrepareArtifacts(_ context.Context, in *cdspb.PrepareArtifactsRequest, _ ...grpc.CallOption) (*cdspb.PrepareArtifactsResponse, error) {
	c.prepareRequest = in
	missing := make([]*cdspb.ArtifactDescriptor, 0, len(c.missing))
	for _, identifier := range c.missing {
		for _, descriptor := range in.GetDescriptors() {
			if descriptor.GetIdentifier() == identifier {
				missing = append(missing, descriptor)
				break
			}
		}
	}
	return &cdspb.PrepareArtifactsResponse{Missing: missing}, nil
}

func (c *recordingArtifactClient) UploadArtifact(context.Context, ...grpc.CallOption) (grpc.ClientStreamingClient[cdspb.UploadArtifactChunk, cdspb.UploadArtifactResponse], error) {
	return &recordingUploadStream{onClose: c.onUpload}, nil
}

type recordingUploadStream struct {
	chunks  []*cdspb.UploadArtifactChunk
	onClose func([]*cdspb.UploadArtifactChunk)
}

func (s *recordingUploadStream) Send(chunk *cdspb.UploadArtifactChunk) error {
	s.chunks = append(s.chunks, chunk)
	return nil
}

func (s *recordingUploadStream) CloseAndRecv() (*cdspb.UploadArtifactResponse, error) {
	if s.onClose != nil {
		s.onClose(s.chunks)
	}
	var descriptor *cdspb.ArtifactDescriptor
	for _, chunk := range s.chunks {
		if chunk.GetDescriptor_() != nil {
			descriptor = chunk.GetDescriptor_()
			break
		}
	}
	return &cdspb.UploadArtifactResponse{Received: descriptor}, nil
}

func (s *recordingUploadStream) Header() (metadata.MD, error) { return nil, nil }
func (s *recordingUploadStream) Trailer() metadata.MD         { return nil }
func (s *recordingUploadStream) CloseSend() error             { return nil }
func (s *recordingUploadStream) Context() context.Context     { return context.Background() }
func (s *recordingUploadStream) SendMsg(any) error            { return nil }
func (s *recordingUploadStream) RecvMsg(any) error            { return nil }

type deployEventStream struct {
	events []*cdspb.DeployContainerEvent
	index  int
	err    error
}

func (s *deployEventStream) Recv() (*cdspb.DeployContainerEvent, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.index >= len(s.events) {
		return nil, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *deployEventStream) Header() (metadata.MD, error) { return nil, nil }
func (s *deployEventStream) Trailer() metadata.MD         { return nil }
func (s *deployEventStream) CloseSend() error             { return nil }
func (s *deployEventStream) Context() context.Context     { return context.Background() }
func (s *deployEventStream) SendMsg(any) error            { return nil }
func (s *deployEventStream) RecvMsg(any) error            { return nil }
