package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// kMaxArtifactSize bounds the size of one staged artifact. Artifacts are
// configuration files (Dockerfiles, keys, scripts...), not payloads.
const kMaxArtifactSize int64 = 64 << 20

// PrepareArtifacts tells which of the advertised artifacts are already staged
// and which ones the client still has to upload.
// Artifacts without digest are always reported missing since they can't be verified.
func (s *artifactServiceServer) PrepareArtifacts(ctx context.Context, req *cdspb.PrepareArtifactsRequest) (*cdspb.PrepareArtifactsResponse, error) {
	if err := validateRPCName("project_name", req.GetProjectName()); err != nil {
		return nil, err
	}

	var found, missing []*cdspb.ArtifactDescriptor
	for _, desc := range req.GetDescriptors() {
		if desc == nil {
			return nil, status.Error(codes.InvalidArgument, "descriptor is required")
		}

		normalized, path, err := stagedArtifactPath(s.core.stagingDir(), req.GetProjectName(), desc.GetIdentifier())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, cerr.Message(err))
		}

		normalizedDesc := proto.Clone(desc).(*cdspb.ArtifactDescriptor)
		normalizedDesc.Identifier = normalized
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

// upload tracks one artifact being received. The content is written into a
// temporary file next to its destination, then atomically renamed once
// verified.
type upload struct {
	projectName string
	uploadID    string
	desc        *cdspb.ArtifactDescriptor
	identifier  string
	destPath    string
	file        *os.File
	written     int64
}

func (u *upload) cleanup() {
	if u.file != nil {
		_ = u.file.Close()
		_ = os.Remove(u.file.Name())
	}
}

func (s *artifactServiceServer) startUpload(chunk *cdspb.UploadArtifactChunk) (*upload, error) {
	if err := validateRPCName("project_name", chunk.GetProjectName()); err != nil {
		return nil, err
	}
	desc := chunk.GetDescriptor_()
	if desc == nil || desc.GetIdentifier() == cg.EmptyStr {
		return nil, status.Error(codes.InvalidArgument, "descriptor with identifier is required on first chunk")
	}
	if desc.GetSize() < 0 || desc.GetSize() > kMaxArtifactSize {
		return nil, status.Errorf(codes.InvalidArgument, "artifact size %d is out of bounds [0, %d]", desc.GetSize(), kMaxArtifactSize)
	}

	normalized, destPath, err := stagedArtifactPath(s.core.stagingDir(), chunk.GetProjectName(), desc.GetIdentifier())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, cerr.Message(err))
	}
	if err := os.MkdirAll(filepath.Dir(destPath), cg.KPermDir); err != nil {
		return nil, status.Errorf(codes.Internal, "create staging directory: %v", err)
	}
	file, err := os.CreateTemp(filepath.Dir(destPath), ".upload-*")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create temporary staging file: %v", err)
	}

	clog.Debug(fmt.Sprintf("Receiving artifact %s for project %s", normalized, chunk.GetProjectName()))
	return &upload{
		projectName: chunk.GetProjectName(),
		uploadID:    chunk.GetUploadId(),
		desc:        desc,
		identifier:  normalized,
		destPath:    destPath,
		file:        file,
	}, nil
}

func (u *upload) write(chunk *cdspb.UploadArtifactChunk, first bool) error {
	if !first {
		if chunk.GetProjectName() != cg.EmptyStr && chunk.GetProjectName() != u.projectName {
			return status.Error(codes.InvalidArgument, "project_name cannot change during upload")
		}
		if chunk.GetUploadId() != cg.EmptyStr && chunk.GetUploadId() != u.uploadID {
			return status.Error(codes.InvalidArgument, "upload_id cannot change during upload")
		}
		if chunk.GetDescriptor_() != nil {
			return status.Error(codes.InvalidArgument, "descriptor must only be sent on first chunk")
		}
	}
	if chunk.GetOffset() != u.written {
		return status.Errorf(codes.InvalidArgument, "chunk offset %d does not match expected offset %d", chunk.GetOffset(), u.written)
	}
	if u.written+int64(len(chunk.GetContent())) > kMaxArtifactSize {
		return status.Errorf(codes.InvalidArgument, "artifact exceeds the maximum size of %d bytes", kMaxArtifactSize)
	}

	// os.File.Write returns an error on short writes
	n, err := u.file.Write(chunk.GetContent())
	u.written += int64(n)
	if err != nil {
		return status.Errorf(codes.Internal, "write chunk: %v", err)
	}
	return nil
}

// commit verifies the received content against the descriptor and moves it to
// its staging location.
func (u *upload) commit() (*cdspb.ArtifactDescriptor, error) {
	tmpPath := u.file.Name()
	if err := u.file.Close(); err != nil {
		return nil, status.Errorf(codes.Internal, "close staging file: %v", err)
	}
	u.file = nil
	defer func() { _ = os.Remove(tmpPath) }()

	if u.desc.GetSize() > 0 && u.desc.GetSize() != u.written {
		return nil, status.Errorf(codes.DataLoss, "artifact size mismatch: expected %d bytes, received %d", u.desc.GetSize(), u.written)
	}
	digest, err := fileSHA256(tmpPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "compute staging digest: %v", err)
	}
	if u.desc.GetDigest() != cg.EmptyStr && u.desc.GetDigest() != digest {
		return nil, status.Errorf(codes.DataLoss, "artifact digest mismatch: expected %s, received %s", u.desc.GetDigest(), digest)
	}
	if err := os.Rename(tmpPath, u.destPath); err != nil {
		return nil, status.Errorf(codes.Internal, "commit staging file: %v", err)
	}

	return &cdspb.ArtifactDescriptor{
		Identifier: u.identifier,
		Type:       u.desc.GetType(),
		Size:       u.written,
		Digest:     digest,
	}, nil
}

// UploadArtifact receives one artifact as a stream of chunks. The first chunk
// carries the project, the upload id and the descriptor.
func (s *artifactServiceServer) UploadArtifact(stream grpc.ClientStreamingServer[cdspb.UploadArtifactChunk, cdspb.UploadArtifactResponse]) error {
	var u *upload
	defer func() {
		if u != nil {
			u.cleanup()
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

		first := u == nil
		if first {
			if u, err = s.startUpload(chunk); err != nil {
				return err
			}
		}
		if err := u.write(chunk, first); err != nil {
			return err
		}
		if chunk.GetFinish() {
			break
		}
	}

	if u == nil {
		return status.Error(codes.InvalidArgument, "upload stream is empty")
	}

	received, err := u.commit()
	if err != nil {
		return err
	}
	return stream.SendAndClose(&cdspb.UploadArtifactResponse{Received: received})
}

func fileMatchesDescriptor(path string, desc *cdspb.ArtifactDescriptor) bool {
	if desc.GetDigest() == cg.EmptyStr {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if desc.GetSize() > 0 && info.Size() != desc.GetSize() {
		return false
	}
	digest, err := fileSHA256(path)
	return err == nil && digest == desc.GetDigest()
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("failed to open %s", err, path)
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("failed to read %s", err, path)
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}
