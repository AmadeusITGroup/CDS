package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockRegistryOps struct {
	buildErr      error
	buildProgress []registryProgress
	startErr      error
	stopErr       error
	deleteErr     error
	deleteForce   bool
	secure        bool
	reg           registry.Registry
	regErr        error
	ca            string
	caErr         error
	user          string
	pass          string
	credErr       error
}

func (m *mockRegistryOps) BuildRegistry(secure bool, report registryProgressReporter) error {
	m.secure = secure
	for _, p := range m.buildProgress {
		if err := report(p); err != nil {
			return err
		}
	}
	return m.buildErr
}
func (m *mockRegistryOps) StartRegistry() error { return m.startErr }
func (m *mockRegistryOps) StopRegistry() error  { return m.stopErr }
func (m *mockRegistryOps) DeleteRegistry(force bool) error {
	m.deleteForce = force
	return m.deleteErr
}
func (m *mockRegistryOps) GetRegistry() (registry.Registry, error) { return m.reg, m.regErr }
func (m *mockRegistryOps) RegistryCA() (string, error)             { return m.ca, m.caErr }
func (m *mockRegistryOps) RegistryCredentials() (string, string, error) {
	return m.user, m.pass, m.credErr
}

func newTestRegistryService(t *testing.T, ops registryOps) *registryServiceServer {
	t.Helper()
	srv, err := newgrpcServer(NewConfig(
		withStagingDir(t.TempDir()),
		withStateDir(t.TempDir()),
		withRegistryOps(ops),
	))
	require.NoError(t, err)
	return newRegistryServiceServer(srv)
}

type mockRegistryBuildStream struct {
	events []*cdspb.BuildRegistryEvent
	grpc.ServerStreamingServer[cdspb.BuildRegistryEvent]
}

func (m *mockRegistryBuildStream) Context() context.Context { return context.Background() }
func (m *mockRegistryBuildStream) Send(e *cdspb.BuildRegistryEvent) error {
	m.events = append(m.events, e)
	return nil
}

func TestBuildRegistryStreamsProgressThenResponse(t *testing.T) {
	ops := &mockRegistryOps{buildProgress: []registryProgress{{phase: "material", message: "m"}, {phase: "registry", message: "r"}}}
	svc := newTestRegistryService(t, ops)
	stream := &mockRegistryBuildStream{}

	err := svc.BuildRegistry(&cdspb.BuildRegistryRequest{Secure: true}, stream)
	require.NoError(t, err)
	require.Len(t, stream.events, 4) // init + 2 progress + response
	assert.True(t, ops.secure, "secure flag is threaded to the ops layer")
	resp := stream.events[3].GetResponse()
	require.NotNil(t, resp)
	assert.Equal(t, registry.Name, resp.GetName())
	assert.Empty(t, resp.GetError())
}

func TestBuildRegistryReportsErrorInResponse(t *testing.T) {
	ops := &mockRegistryOps{buildErr: errors.New("boom")}
	svc := newTestRegistryService(t, ops)
	stream := &mockRegistryBuildStream{}

	require.NoError(t, svc.BuildRegistry(&cdspb.BuildRegistryRequest{Secure: true}, stream))
	resp := stream.events[len(stream.events)-1].GetResponse()
	require.NotNil(t, resp)
	assert.Contains(t, resp.GetError(), "boom")
}

func TestRegistryStartStopDelete(t *testing.T) {
	ops := &mockRegistryOps{}
	svc := newTestRegistryService(t, ops)

	_, err := svc.StartRegistry(context.Background(), &cdspb.StartRegistryRequest{})
	require.NoError(t, err)
	_, err = svc.StopRegistry(context.Background(), &cdspb.StopRegistryRequest{})
	require.NoError(t, err)
	_, err = svc.DeleteRegistry(context.Background(), &cdspb.DeleteRegistryRequest{Force: true})
	require.NoError(t, err)
	assert.True(t, ops.deleteForce)
}

func TestRegistryStartMapsErrorToInternal(t *testing.T) {
	ops := &mockRegistryOps{startErr: errors.New("nope")}
	svc := newTestRegistryService(t, ops)
	_, err := svc.StartRegistry(context.Background(), &cdspb.StartRegistryRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestGetRegistryReturnsProtoWithHost(t *testing.T) {
	ops := &mockRegistryOps{reg: registry.Registry{
		Name:     registry.Name,
		Status:   "running",
		Port:     registry.DefaultPort,
		Hostname: registry.Hostname,
	}}
	svc := newTestRegistryService(t, ops)

	resp, err := svc.GetRegistry(context.Background(), &cdspb.GetRegistryRequest{})
	require.NoError(t, err)
	assert.Equal(t, registry.Name, resp.GetRegistry().GetName())
	assert.Equal(t, int32(registry.DefaultPort), resp.GetRegistry().GetPort())
	assert.Equal(t, registry.Hostname, resp.GetRegistry().GetHostname())
	assert.NotEmpty(t, resp.GetRegistry().GetHost())
}
