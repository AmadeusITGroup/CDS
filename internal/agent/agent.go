package agent

import (
	"context"
	"log/slog"

	cdspb "github.com/amadeusitgroup/cds/internal/api/v1"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/core"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type bom struct {
	manager commandManager
	logger  *slog.Logger
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
	config.manager = defaultManager()
	srv, err := newgrpcServer(config)
	if err != nil {
		return nil, err
	}
	cdspb.RegisterAgentServer(gsrv, srv)
	return gsrv, nil
}

func (s *grpcServer) GetVersion(context.Context, *emptypb.Empty) (*cdspb.Version, error) {

	if 1 == 2 { // TODO: remove - it's just a showcase of how one can return error
		return nil, status.Error(codes.NotFound, "dummy error")
	}
	return &cdspb.Version{Current: s.manager().Version()}, nil
}

type grpcServer struct {
	cdspb.UnimplementedAgentServer
	b bom
}

func newgrpcServer(b *bom) (srv *grpcServer, err error) {
	srv = &grpcServer{
		b: *b,
	}
	return srv, nil
}

func (s *grpcServer) manager() commandManager {
	if s.b.manager == nil {
		clog.Warn("Agent server config was not initialized properly!")
		s.b.manager = defaultManager()
	}
	return s.b.manager
}
