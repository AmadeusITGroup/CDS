package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestArtifactService(t *testing.T) *artifactServiceServer {
	t.Helper()
	dir := t.TempDir()
	return &artifactServiceServer{
		stagingDir: dir,
	}
}

func TestPrepareArtifacts_EmptyProject(t *testing.T) {
	svc := newTestArtifactService(t)
	_, err := svc.PrepareArtifacts(context.Background(), &cdspb.PrepareArtifactsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPrepareArtifacts_InvalidProject(t *testing.T) {
	svc := newTestArtifactService(t)
	_, err := svc.PrepareArtifacts(context.Background(), &cdspb.PrepareArtifactsRequest{ProjectName: "../test-project"})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestPrepareArtifacts_AllMissing(t *testing.T) {
	svc := newTestArtifactService(t)
	dockerfileIdentifier := mustResourceIdentifier(t, containerconf.KindDockerfile, "Dockerfile")
	resp, err := svc.PrepareArtifacts(context.Background(), &cdspb.PrepareArtifactsRequest{
		ProjectName: "test-project",
		Descriptors: []*cdspb.ArtifactDescriptor{
			{Identifier: dockerfileIdentifier, Size: 100},
			{Identifier: "resource/config/bashrc", Size: 50},
		},
	})
	require.NoError(t, err)
	assert.Empty(t, resp.GetFound())
	assert.Len(t, resp.GetMissing(), 2)
}

func TestPrepareArtifacts_SomeFound(t *testing.T) {
	svc := newTestArtifactService(t)
	dockerfileIdentifier := mustResourceIdentifier(t, containerconf.KindDockerfile, "Dockerfile")

	// Stage one artifact
	path, err := svc.artifactPath("test-project", dockerfileIdentifier)
	require.NoError(t, err)
	content := []byte("FROM ubuntu:22.04\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, content, 0600))
	digest := computeSHA256(content)

	resp, err := svc.PrepareArtifacts(context.Background(), &cdspb.PrepareArtifactsRequest{
		ProjectName: "test-project",
		Descriptors: []*cdspb.ArtifactDescriptor{
			{Identifier: dockerfileIdentifier, Size: int64(len(content)), Digest: digest},
			{Identifier: "resource/config/bashrc", Size: 50},
		},
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetFound(), 1)
	assert.Equal(t, dockerfileIdentifier, resp.GetFound()[0].GetIdentifier())
	assert.Len(t, resp.GetMissing(), 1)
	assert.Equal(t, "resource/config/bashrc", resp.GetMissing()[0].GetIdentifier())
}

func TestPrepareArtifacts_DigestMismatch(t *testing.T) {
	svc := newTestArtifactService(t)
	fileIdentifier := mustResourceIdentifier(t, "file", "file.txt")

	path, err := svc.artifactPath("test-project", fileIdentifier)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte("old content"), 0600))

	resp, err := svc.PrepareArtifacts(context.Background(), &cdspb.PrepareArtifactsRequest{
		ProjectName: "test-project",
		Descriptors: []*cdspb.ArtifactDescriptor{
			{Identifier: fileIdentifier, Digest: "sha256:wrongdigest"},
		},
	})
	require.NoError(t, err)
	assert.Empty(t, resp.GetFound())
	assert.Len(t, resp.GetMissing(), 1)
}

func TestUploadArtifact(t *testing.T) {
	svc := newTestArtifactService(t)
	content := []byte("FROM ubuntu:22.04\nRUN apt-get update\n")
	dockerfileIdentifier := mustResourceIdentifier(t, containerconf.KindDockerfile, "Dockerfile")

	stream := &mockUploadStream{
		chunks: []*cdspb.UploadArtifactChunk{
			{
				UploadId:    "upload-1",
				ProjectName: "test-project",
				Descriptor_: &cdspb.ArtifactDescriptor{
					Identifier: dockerfileIdentifier,
					Type:       "text/plain",
				},
				Offset:  0,
				Content: content[:20],
			},
			{
				UploadId: "upload-1",
				Offset:   20,
				Content:  content[20:],
				Finish:   true,
			},
		},
	}

	err := svc.UploadArtifact(stream)
	require.NoError(t, err)
	require.NotNil(t, stream.response)

	received := stream.response.GetReceived()
	assert.Equal(t, dockerfileIdentifier, received.GetIdentifier())
	assert.Equal(t, int64(len(content)), received.GetSize())
	assert.Equal(t, computeSHA256(content), received.GetDigest())

	stagedPath, err := svc.artifactPath("test-project", dockerfileIdentifier)
	require.NoError(t, err)
	staged, err := os.ReadFile(stagedPath)
	require.NoError(t, err)
	assert.Equal(t, content, staged)
}

func TestUploadArtifact_MissingProject(t *testing.T) {
	svc := newTestArtifactService(t)
	stream := &mockUploadStream{
		chunks: []*cdspb.UploadArtifactChunk{
			{
				Descriptor_: &cdspb.ArtifactDescriptor{Identifier: "file.txt"},
				Content:     []byte("data"),
				Finish:      true,
			},
		},
	}
	err := svc.UploadArtifact(stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUploadArtifact_InvalidIdentifier(t *testing.T) {
	svc := newTestArtifactService(t)
	stream := &mockUploadStream{
		chunks: []*cdspb.UploadArtifactChunk{
			{
				ProjectName: "test-project",
				Descriptor_: &cdspb.ArtifactDescriptor{Identifier: "../outside.txt"},
				Content:     []byte("data"),
				Finish:      true,
			},
		},
	}

	err := svc.UploadArtifact(stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUploadArtifact_MissingDescriptor(t *testing.T) {
	svc := newTestArtifactService(t)
	stream := &mockUploadStream{
		chunks: []*cdspb.UploadArtifactChunk{
			{
				ProjectName: "test-project",
				Content:     []byte("data"),
				Finish:      true,
			},
		},
	}
	err := svc.UploadArtifact(stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUploadArtifact_NegativeSize(t *testing.T) {
	svc := newTestArtifactService(t)
	stream := &mockUploadStream{
		chunks: []*cdspb.UploadArtifactChunk{
			{
				ProjectName: "test-project",
				Descriptor_: &cdspb.ArtifactDescriptor{
					Identifier: mustResourceIdentifier(t, containerconf.KindDockerfile, "Dockerfile"),
					Size:       -1,
				},
				Content: []byte("data"),
				Finish:  true,
			},
		},
	}
	err := svc.UploadArtifact(stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUploadArtifact_EmptyStream(t *testing.T) {
	svc := newTestArtifactService(t)
	stream := &mockUploadStream{}

	err := svc.UploadArtifact(stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Nil(t, stream.response)
}

func TestUploadArtifact_DigestMismatchDoesNotCommit(t *testing.T) {
	svc := newTestArtifactService(t)
	identifier := mustResourceIdentifier(t, containerconf.KindDockerfile, "Dockerfile")
	stream := &mockUploadStream{
		chunks: []*cdspb.UploadArtifactChunk{
			{
				UploadId:    "upload-1",
				ProjectName: "test-project",
				Descriptor_: &cdspb.ArtifactDescriptor{
					Identifier: identifier,
					Digest:     "sha256:wrong",
				},
				Content: []byte("FROM ubuntu:22.04\n"),
				Finish:  true,
			},
		},
	}

	err := svc.UploadArtifact(stream)
	require.Error(t, err)
	assert.Equal(t, codes.DataLoss, status.Code(err))
	assert.Nil(t, stream.response)

	stagedPath, err := svc.artifactPath("test-project", identifier)
	require.NoError(t, err)
	_, err = os.Stat(stagedPath)
	assert.True(t, os.IsNotExist(err))
}

func TestUploadArtifact_OffsetMismatch(t *testing.T) {
	svc := newTestArtifactService(t)
	identifier := mustResourceIdentifier(t, containerconf.KindDockerfile, "Dockerfile")
	stream := &mockUploadStream{
		chunks: []*cdspb.UploadArtifactChunk{
			{
				UploadId:    "upload-1",
				ProjectName: "test-project",
				Descriptor_: &cdspb.ArtifactDescriptor{Identifier: identifier},
				Content:     []byte("ab"),
			},
			{
				UploadId: "upload-1",
				Offset:   1,
				Content:  []byte("cd"),
				Finish:   true,
			},
		},
	}

	err := svc.UploadArtifact(stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Nil(t, stream.response)
}

// mockUploadStream simulates a client-streaming artifact upload.
type mockUploadStream struct {
	grpc.ClientStreamingServer[cdspb.UploadArtifactChunk, cdspb.UploadArtifactResponse]
	chunks   []*cdspb.UploadArtifactChunk
	index    int
	response *cdspb.UploadArtifactResponse
}

func (m *mockUploadStream) Recv() (*cdspb.UploadArtifactChunk, error) {
	if m.index >= len(m.chunks) {
		return nil, io.EOF
	}
	chunk := m.chunks[m.index]
	m.index++
	return chunk, nil
}

func (m *mockUploadStream) SendAndClose(resp *cdspb.UploadArtifactResponse) error {
	m.response = resp
	return nil
}

func computeSHA256(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func mustResourceIdentifier(t *testing.T, kind, logicalName string) string {
	t.Helper()
	identifier, err := containerconf.ResourceIdentifier(kind, logicalName)
	require.NoError(t, err)
	return identifier
}
