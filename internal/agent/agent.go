package agent

import (
	"context"
	"log/slog"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"google.golang.org/grpc"
)

const defaultAgentVersion = "9.9.9"

type bom struct {
	logger *slog.Logger
}

func NewConfig(options ...func(*bom)) *bom {
	// TODO: handle the case where it's not the first start of the server: load the config from the file on disk
	b := &bom{}
	for _, option := range options {
		option(b)
	}
	return b
}

func WithLogger(logger *slog.Logger) func(*bom) {
	return func(b *bom) {
		b.logger = logger
	}
}

func NewAgent(config *bom, opts ...grpc.ServerOption) (*grpc.Server, error) {
	gsrv := grpc.NewServer(opts...)
	stagingDir := defaultArtifactStagingDir()
	cdspb.RegisterAgentInfoServiceServer(gsrv, newAgentInfoServiceServer())
	cdspb.RegisterContainerServiceServer(gsrv, newContainerServiceServer(stagingDir))
	cdspb.RegisterArtifactServiceServer(gsrv, newArtifactServiceServer(stagingDir))
	return gsrv, nil
}

func (s *agentInfoServiceServer) GetVersion(context.Context, *cdspb.GetVersionRequest) (*cdspb.GetVersionResponse, error) {
	return &cdspb.GetVersionResponse{Current: defaultAgentVersion}, nil
}

type agentInfoServiceServer struct {
	cdspb.UnimplementedAgentInfoServiceServer
}

func newAgentInfoServiceServer() *agentInfoServiceServer {
	return &agentInfoServiceServer{}
}

type containerServiceServer struct {
	cdspb.UnimplementedContainerServiceServer
	engineOps  ContainerOps
	stagingDir string
}

// NewContainerServiceWithOps creates a container gRPC service server that
// delegates container engine calls to the provided ContainerOps implementation.
// This is the primary entry point for tests that need to inject a fake engine.
func NewContainerServiceWithOps(ops ContainerOps, stagingDir string) cdspb.ContainerServiceServer {
	return &containerServiceServer{
		engineOps:  ops,
		stagingDir: stagingDir,
	}
}

func newContainerServiceServer(stagingDir string) *containerServiceServer {
	if stagingDir == "" {
		stagingDir = defaultArtifactStagingDir()
	}
	return &containerServiceServer{
		engineOps:  newDefaultContainerOps(),
		stagingDir: stagingDir,
	}
}
