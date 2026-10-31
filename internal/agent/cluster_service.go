package agent

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/orchestration"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BuildCluster creates the kind cluster if absent, streaming progress. It
// mirrors DeployContainer: progress events followed by a terminal response, in
// a oneof envelope.
func (s *clusterServiceServer) BuildCluster(req *cdspb.BuildClusterRequest, stream grpc.ServerStreamingServer[cdspb.BuildClusterEvent]) error {
	if err := sendClusterProgress(stream, "init", "Starting cluster build"); err != nil {
		return err
	}

	buildErr := s.core.clusterOps().BuildCluster(req.GetEngine(), req.GetConfigFile(), func(p clusterProgress) error {
		return sendClusterProgress(stream, p.phase, p.message)
	})
	if buildErr == nil {
		// a rebuilt cluster issues a fresh kubeconfig; re-decorate running
		// orchestration containers so they do not keep stale cluster access.
		s.core.redecorateContainers(stream.Context(), func(p deployProgress) error {
			return sendClusterProgress(stream, p.phase, p.message)
		})
	}
	return sendClusterResponse(stream, orchestration.ClusterName, buildErr)
}

// StartCluster starts the existing cluster.
func (s *clusterServiceServer) StartCluster(ctx context.Context, req *cdspb.StartClusterRequest) (*cdspb.StartClusterResponse, error) {
	if err := s.core.clusterOps().StartCluster(); err != nil {
		return nil, status.Errorf(codes.Internal, "start cluster: %v", err)
	}
	return &cdspb.StartClusterResponse{Output: fmt.Sprintf("Cluster %s started", orchestration.ClusterName)}, nil
}

// StopCluster stops the running cluster.
func (s *clusterServiceServer) StopCluster(ctx context.Context, req *cdspb.StopClusterRequest) (*cdspb.StopClusterResponse, error) {
	if err := s.core.clusterOps().StopCluster(); err != nil {
		return nil, status.Errorf(codes.Internal, "stop cluster: %v", err)
	}
	return &cdspb.StopClusterResponse{Output: fmt.Sprintf("Cluster %s stopped", orchestration.ClusterName)}, nil
}

// DeleteCluster removes the cluster.
func (s *clusterServiceServer) DeleteCluster(ctx context.Context, req *cdspb.DeleteClusterRequest) (*cdspb.DeleteClusterResponse, error) {
	if err := s.core.clusterOps().DeleteCluster(req.GetForce()); err != nil {
		return nil, status.Errorf(codes.Internal, "delete cluster: %v", err)
	}
	return &cdspb.DeleteClusterResponse{Output: fmt.Sprintf("Cluster %s deleted", orchestration.ClusterName)}, nil
}

// GetCluster returns the current cluster summary.
func (s *clusterServiceServer) GetCluster(ctx context.Context, req *cdspb.GetClusterRequest) (*cdspb.GetClusterResponse, error) {
	cluster, err := s.core.clusterOps().GetCluster()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get cluster: %v", err)
	}
	return &cdspb.GetClusterResponse{Cluster: s.clusterToProto(cluster)}, nil
}

// GetKubeconfig returns the cluster kubeconfig.
func (s *clusterServiceServer) GetKubeconfig(ctx context.Context, req *cdspb.GetKubeconfigRequest) (*cdspb.GetKubeconfigResponse, error) {
	kubeconfig, err := s.core.clusterOps().GetKubeconfig()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get kubeconfig: %v", err)
	}
	return &cdspb.GetKubeconfigResponse{Kubeconfig: kubeconfig}, nil
}

// clusterToProto converts a cluster into its API representation, reporting the
// agent host.
func (s *clusterServiceServer) clusterToProto(c orchestration.Cluster) *cdspb.Cluster {
	return &cdspb.Cluster{
		Name:   c.Name,
		Node:   c.Node,
		Status: bo.FContainerStatus(c.Status),
		Host:   s.core.hostname(),
	}
}

func sendClusterProgress(stream grpc.ServerStreamingServer[cdspb.BuildClusterEvent], phase, message string) error {
	if err := stream.Send(&cdspb.BuildClusterEvent{
		Payload: &cdspb.BuildClusterEvent_Progress{
			Progress: &cdspb.BuildClusterProgress{Phase: phase, Message: message},
		},
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send cluster progress: %v", err)
	}
	return nil
}

func sendClusterResponse(stream grpc.ServerStreamingServer[cdspb.BuildClusterEvent], name string, buildErr error) error {
	response := &cdspb.BuildClusterResponse{Name: name}
	if buildErr != nil {
		if _, isStatus := status.FromError(buildErr); isStatus {
			return buildErr
		}
		response.Error = boundedHead(buildErr.Error())
	}
	if err := stream.Send(&cdspb.BuildClusterEvent{
		Payload: &cdspb.BuildClusterEvent_Response{Response: response},
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send cluster response: %v", err)
	}
	return nil
}
