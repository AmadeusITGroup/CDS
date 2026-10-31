package agent

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BuildRegistry creates the secure registry if absent, streaming progress.
func (s *registryServiceServer) BuildRegistry(req *cdspb.BuildRegistryRequest, stream grpc.ServerStreamingServer[cdspb.BuildRegistryEvent]) error {
	if err := sendRegistryProgress(stream, "init", "Starting registry build"); err != nil {
		return err
	}
	buildErr := s.core.registryOps().BuildRegistry(req.GetSecure(), func(p registryProgress) error {
		return sendRegistryProgress(stream, p.phase, p.message)
	})
	if buildErr == nil {
		// a rebuilt registry may present a fresh CA; re-decorate running
		// orchestration containers so their trust store stays valid.
		s.core.redecorateContainers(stream.Context(), func(p deployProgress) error {
			return sendRegistryProgress(stream, p.phase, p.message)
		})
	}
	return sendRegistryResponse(stream, registry.Name, buildErr)
}

func (s *registryServiceServer) StartRegistry(ctx context.Context, req *cdspb.StartRegistryRequest) (*cdspb.StartRegistryResponse, error) {
	if err := s.core.registryOps().StartRegistry(); err != nil {
		return nil, status.Errorf(codes.Internal, "start registry: %v", err)
	}
	return &cdspb.StartRegistryResponse{Output: fmt.Sprintf("Registry %s started", registry.Name)}, nil
}

func (s *registryServiceServer) StopRegistry(ctx context.Context, req *cdspb.StopRegistryRequest) (*cdspb.StopRegistryResponse, error) {
	if err := s.core.registryOps().StopRegistry(); err != nil {
		return nil, status.Errorf(codes.Internal, "stop registry: %v", err)
	}
	return &cdspb.StopRegistryResponse{Output: fmt.Sprintf("Registry %s stopped", registry.Name)}, nil
}

func (s *registryServiceServer) DeleteRegistry(ctx context.Context, req *cdspb.DeleteRegistryRequest) (*cdspb.DeleteRegistryResponse, error) {
	if err := s.core.registryOps().DeleteRegistry(req.GetForce()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete registry: %v", err)
	}
	return &cdspb.DeleteRegistryResponse{Output: fmt.Sprintf("Registry %s deleted", registry.Name)}, nil
}

func (s *registryServiceServer) GetRegistry(ctx context.Context, req *cdspb.GetRegistryRequest) (*cdspb.GetRegistryResponse, error) {
	reg, err := s.core.registryOps().GetRegistry()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get registry: %v", err)
	}
	return &cdspb.GetRegistryResponse{Registry: s.registryToProto(reg)}, nil
}

func (s *registryServiceServer) registryToProto(r registry.Registry) *cdspb.Registry {
	return &cdspb.Registry{
		Name:     r.Name,
		Status:   r.Status,
		Port:     int32(r.Port),
		Hostname: r.Hostname,
		Host:     s.core.hostname(),
	}
}

func sendRegistryProgress(stream grpc.ServerStreamingServer[cdspb.BuildRegistryEvent], phase, message string) error {
	if err := stream.Send(&cdspb.BuildRegistryEvent{
		Payload: &cdspb.BuildRegistryEvent_Progress{
			Progress: &cdspb.BuildRegistryProgress{Phase: phase, Message: message},
		},
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send registry progress: %v", err)
	}
	return nil
}

func sendRegistryResponse(stream grpc.ServerStreamingServer[cdspb.BuildRegistryEvent], name string, buildErr error) error {
	response := &cdspb.BuildRegistryResponse{Name: name}
	if buildErr != nil {
		if _, isStatus := status.FromError(buildErr); isStatus {
			return buildErr
		}
		response.Error = boundedHead(buildErr.Error())
	}
	if err := stream.Send(&cdspb.BuildRegistryEvent{
		Payload: &cdspb.BuildRegistryEvent_Response{Response: response},
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send registry response: %v", err)
	}
	return nil
}
