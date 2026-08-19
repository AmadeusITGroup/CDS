package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type artifactServiceServer struct {
	cdspb.UnimplementedArtifactServiceServer
	stagingDir string
}

func newArtifactServiceServer(stagingDir string) *artifactServiceServer {
	if stagingDir == "" {
		stagingDir = defaultArtifactStagingDir()
	}
	return &artifactServiceServer{
		stagingDir: stagingDir,
	}
}

func (s *artifactServiceServer) artifactPath(projectName, identifier string) (string, error) {
	artifact, err := resolveProjectArtifact(s.stagingDir, projectName, identifier)
	if err != nil {
		return "", err
	}
	return artifact.path, nil
}

func (s *artifactServiceServer) PrepareArtifacts(ctx context.Context, req *cdspb.PrepareArtifactsRequest) (*cdspb.PrepareArtifactsResponse, error) {
	if err := validateRPCName("project_name", req.GetProjectName()); err != nil {
		return nil, err
	}

	var found, missing []*cdspb.ArtifactDescriptor
	for _, desc := range req.GetDescriptors() {
		if desc == nil {
			return nil, status.Error(codes.InvalidArgument, "descriptor is required")
		}

		normalizedIdentifier, err := containerconf.NormalizeIdentifier(desc.GetIdentifier())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid artifact identifier %q: %v", desc.GetIdentifier(), err)
		}

		path, err := s.artifactPath(req.GetProjectName(), normalizedIdentifier)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid artifact identifier %q: %v", desc.GetIdentifier(), err)
		}

		normalizedDesc := proto.Clone(desc).(*cdspb.ArtifactDescriptor)
		normalizedDesc.Identifier = normalizedIdentifier
		if fileMatchesDescriptor(path, normalizedDesc) {
			found = append(found, normalizedDesc)
		} else {
			missing = append(missing, normalizedDesc)
		}
	}

	return &cdspb.PrepareArtifactsResponse{
		Found:   found,
		Missing: missing,
	}, nil
}

func (s *artifactServiceServer) UploadArtifact(stream grpc.ClientStreamingServer[cdspb.UploadArtifactChunk, cdspb.UploadArtifactResponse]) error {
	var projectName string
	var uploadID string
	var desc *cdspb.ArtifactDescriptor
	var normalizedIdentifier string
	var destPath string
	var tempPath string
	var file *os.File
	var totalWritten int64

	defer func() {
		if file != nil {
			_ = file.Close()
		}
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}()

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "receive chunk: %v", err)
		}

		if file == nil {
			projectName = chunk.GetProjectName()
			uploadID = chunk.GetUploadId()
			desc = chunk.GetDescriptor_()

			if projectName == "" {
				return status.Error(codes.InvalidArgument, "project_name is required on first chunk")
			}
			if err := validateRPCName("project_name", projectName); err != nil {
				return err
			}
			if desc == nil || desc.GetIdentifier() == "" {
				return status.Error(codes.InvalidArgument, "descriptor with identifier is required on first chunk")
			}
			if desc.GetSize() < 0 {
				return status.Error(codes.InvalidArgument, "artifact size must not be negative")
			}

			normalizedIdentifier, err = containerconf.NormalizeIdentifier(desc.GetIdentifier())
			if err != nil {
				return status.Errorf(codes.InvalidArgument, "invalid artifact identifier %q: %v", desc.GetIdentifier(), err)
			}

			destPath, err = s.artifactPath(projectName, normalizedIdentifier)
			if err != nil {
				return status.Errorf(codes.InvalidArgument, "invalid artifact identifier %q: %v", desc.GetIdentifier(), err)
			}
			if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
				return status.Errorf(codes.Internal, "create staging directory: %v", err)
			}

			file, err = os.CreateTemp(filepath.Dir(destPath), ".upload-*")
			if err != nil {
				return status.Errorf(codes.Internal, "create temporary staging file: %v", err)
			}
			tempPath = file.Name()
			clog.Debug(fmt.Sprintf("Receiving artifact %s for project %s", desc.GetIdentifier(), projectName))
		} else {
			if chunk.GetProjectName() != "" && chunk.GetProjectName() != projectName {
				return status.Error(codes.InvalidArgument, "project_name cannot change during upload")
			}
			if chunk.GetUploadId() != "" && uploadID != "" && chunk.GetUploadId() != uploadID {
				return status.Error(codes.InvalidArgument, "upload_id cannot change during upload")
			}
			if chunk.GetDescriptor_() != nil {
				return status.Error(codes.InvalidArgument, "descriptor must only be sent on first chunk")
			}
		}

		if chunk.GetOffset() != totalWritten {
			return status.Errorf(codes.InvalidArgument, "chunk offset %d does not match expected offset %d", chunk.GetOffset(), totalWritten)
		}

		n, err := file.Write(chunk.GetContent())
		if err != nil {
			return status.Errorf(codes.Internal, "write chunk: %v", err)
		}
		if n != len(chunk.GetContent()) {
			return status.Errorf(codes.Internal, "write chunk: %v", io.ErrShortWrite)
		}
		totalWritten += int64(n)

		if chunk.GetFinish() {
			break
		}
	}

	if desc == nil {
		return status.Error(codes.InvalidArgument, "upload stream is empty")
	}

	if file != nil {
		if err := file.Close(); err != nil {
			return status.Errorf(codes.Internal, "close staging file: %v", err)
		}
		file = nil
	}

	if desc.GetSize() > 0 && desc.GetSize() != totalWritten {
		return status.Errorf(codes.DataLoss, "artifact size mismatch: expected %d bytes, received %d", desc.GetSize(), totalWritten)
	}

	digest, err := fileSHA256(tempPath)
	if err != nil {
		return status.Errorf(codes.Internal, "compute staging digest: %v", err)
	}
	if desc.GetDigest() != "" && desc.GetDigest() != digest {
		return status.Errorf(codes.DataLoss, "artifact digest mismatch: expected %s, received %s", desc.GetDigest(), digest)
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		return status.Errorf(codes.Internal, "commit staging file: %v", err)
	}
	tempPath = ""

	received := &cdspb.ArtifactDescriptor{
		Identifier: normalizedIdentifier,
		Type:       desc.GetType(),
		Size:       totalWritten,
		Digest:     digest,
	}

	return stream.SendAndClose(&cdspb.UploadArtifactResponse{
		Received: received,
	})
}

func fileMatchesDescriptor(path string, desc *cdspb.ArtifactDescriptor) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.Mode().IsRegular() {
		return false
	}

	if desc.GetSize() > 0 && info.Size() != desc.GetSize() {
		return false
	}

	if desc.GetDigest() != "" {
		digest, err := fileSHA256(path)
		if err != nil {
			return false
		}
		if digest != desc.GetDigest() {
			return false
		}
	}

	return true
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}
