package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/core"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	kStagingDirName     = "staging"
	kDeploymentsDirName = "deployments"
)

type bom struct {
	manager      commandManager
	logger       *slog.Logger
	stagingDir   string
	stateDir     string
	containerOps containerOps
	hostname     string
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

// withStagingDir overrides the directory where uploaded artifacts are staged.
func withStagingDir(dir string) func(*bom) {
	return func(b *bom) {
		b.stagingDir = dir
	}
}

// withStateDir overrides the directory where deployments are recorded for rebuilds.
func withStateDir(dir string) func(*bom) {
	return func(b *bom) {
		b.stateDir = dir
	}
}

// withContainerOps overrides the container engine operations.
func withContainerOps(ops containerOps) func(*bom) {
	return func(b *bom) {
		b.containerOps = ops
	}
}

type commandManager interface {
	Version() string
	Space() map[string]core.Cmd
	Project() map[string]core.Cmd
}

func defaultManager() commandManager {
	return core.New()
}

func NewAgent(config *bom, opts ...grpc.ServerOption) (*grpc.Server, error) {
	gsrv := grpc.NewServer(opts...)
	srv, err := newgrpcServer(config)
	if err != nil {
		return nil, err
	}
	cdspb.RegisterAgentInfoServiceServer(gsrv, newAgentInfoServiceServer(srv))
	cdspb.RegisterContainerServiceServer(gsrv, newContainerServiceServer(srv))
	cdspb.RegisterArtifactServiceServer(gsrv, newArtifactServiceServer(srv))
	return gsrv, nil
}

func (s *agentInfoServiceServer) GetVersion(context.Context, *cdspb.GetVersionRequest) (*cdspb.GetVersionResponse, error) {
	if 1 == 2 { // TODO: remove - it's just a showcase of how one can return error
		return nil, status.Error(codes.NotFound, "dummy error")
	}
	return &cdspb.GetVersionResponse{Current: s.core.manager().Version()}, nil
}

type grpcServer struct {
	b bom
}

// newgrpcServer completes the agent configuration with defaults for anything
// that was not explicitly configured.
func newgrpcServer(b *bom) (srv *grpcServer, err error) {
	srv = &grpcServer{
		b: *b,
	}
	if srv.b.manager == nil {
		srv.b.manager = defaultManager()
	}
	if srv.b.stagingDir == cg.EmptyStr {
		srv.b.stagingDir = filepath.Join(cenv.CacheDir(), kStagingDirName)
	}
	if srv.b.stateDir == cg.EmptyStr {
		srv.b.stateDir = filepath.Join(cenv.CacheDir(), kDeploymentsDirName)
	}
	if srv.b.containerOps == nil {
		srv.b.containerOps = defaultContainerOps{}
	}
	if srv.b.hostname == cg.EmptyStr {
		hostname, errHostname := os.Hostname()
		if errHostname != nil {
			clog.Warn(fmt.Sprintf("Failed to resolve agent hostname, using %s", cg.KLocalhost), errHostname)
			hostname = cg.KLocalhost
		}
		srv.b.hostname = hostname
	}
	return srv, nil
}

func (s *grpcServer) manager() commandManager {
	return s.b.manager
}

func (s *grpcServer) containerOps() containerOps {
	return s.b.containerOps
}

func (s *grpcServer) stagingDir() string {
	return s.b.stagingDir
}

func (s *grpcServer) deployments() deploymentStore {
	return deploymentStore{dir: s.b.stateDir}
}

func (s *grpcServer) hostname() string {
	return s.b.hostname
}

type agentInfoServiceServer struct {
	cdspb.UnimplementedAgentInfoServiceServer
	core *grpcServer
}

func newAgentInfoServiceServer(core *grpcServer) *agentInfoServiceServer {
	return &agentInfoServiceServer{core: core}
}

type containerServiceServer struct {
	cdspb.UnimplementedContainerServiceServer
	core *grpcServer
}

func newContainerServiceServer(core *grpcServer) *containerServiceServer {
	return &containerServiceServer{core: core}
}

type artifactServiceServer struct {
	cdspb.UnimplementedArtifactServiceServer
	core *grpcServer
}

func newArtifactServiceServer(core *grpcServer) *artifactServiceServer {
	return &artifactServiceServer{core: core}
}
