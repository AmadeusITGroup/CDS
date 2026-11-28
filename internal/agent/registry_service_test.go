package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/orchestration"
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
	built         bool
	reg           registry.Registry
	regErr        error
	ca            string
	caErr         error
	user          string
	pass          string
	credErr       error
}

func (m *mockRegistryOps) BuildRegistry(_ context.Context, report registryProgressReporter) error {
	m.built = true
	for _, p := range m.buildProgress {
		if err := report(p); err != nil {
			return err
		}
	}
	return m.buildErr
}
func (m *mockRegistryOps) StartRegistry(context.Context) error { return m.startErr }
func (m *mockRegistryOps) StopRegistry(context.Context) error  { return m.stopErr }
func (m *mockRegistryOps) DeleteRegistry(_ context.Context, force bool) error {
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

	err := svc.BuildRegistry(&cdspb.BuildRegistryRequest{}, stream)
	require.NoError(t, err)
	require.Len(t, stream.events, 4) // init + 2 progress + response
	assert.True(t, ops.built, "an empty request builds the (always secured) registry")
	resp := stream.events[3].GetResponse()
	require.NotNil(t, resp)
	assert.Equal(t, registry.Name, resp.GetName())
	assert.Empty(t, resp.GetError())
}

func TestBuildRegistryReportsErrorInResponse(t *testing.T) {
	ops := &mockRegistryOps{buildErr: errors.New("boom")}
	svc := newTestRegistryService(t, ops)
	stream := &mockRegistryBuildStream{}

	require.NoError(t, svc.BuildRegistry(&cdspb.BuildRegistryRequest{}, stream))
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

func TestDeleteRegistryMapsInUseToFailedPrecondition(t *testing.T) {
	inUse := guardDelete("registry", false, func() ([]string, error) { return []string{"dev-a"}, nil })
	svc := newTestRegistryService(t, &mockRegistryOps{deleteErr: inUse})
	_, err := svc.DeleteRegistry(context.Background(), &cdspb.DeleteRegistryRequest{})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	// usage unknown is not a refusal of the request: it stays Internal
	unknown := guardDelete("registry", false, func() ([]string, error) { return nil, errors.New("list boom") })
	svc = newTestRegistryService(t, &mockRegistryOps{deleteErr: unknown})
	_, err = svc.DeleteRegistry(context.Background(), &cdspb.DeleteRegistryRequest{})
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

// TestBuildRegistryRefreshesClusterTrust: a rebuilt registry
// re-installs its CA on the cluster nodes and recreates the pull secret, and
// leaves mirrors and ingress alone.
func TestBuildRegistryRefreshesClusterTrust(t *testing.T) {
	cluster := &mockClusterOps{cluster: orchestration.Cluster{Status: bo.KContainerStatusRunning}}
	srv := newTestOrchestrationServer(t, cluster, &mockRegistryOps{ca: "CA", user: "u", pass: "p"}, kOrchestratedConfig)

	require.NoError(t, newRegistryServiceServer(srv).BuildRegistry(&cdspb.BuildRegistryRequest{}, &mockRegistryBuildStream{}))

	assert.True(t, cluster.registryTrusted)
	assert.True(t, cluster.registrySecretCreated)
	assert.Nil(t, cluster.appliedMirrors, "mirrors are a cluster concern")
	assert.False(t, cluster.ingressDeployed, "ingress is a cluster concern")
}

func TestBuildRegistrySkipsClusterTrustWithoutRunningCluster(t *testing.T) {
	cluster := &mockClusterOps{clusterErr: errors.New("no cluster")}
	srv := newTestOrchestrationServer(t, cluster, &mockRegistryOps{ca: "CA"}, kOrchestratedConfig)

	require.NoError(t, newRegistryServiceServer(srv).BuildRegistry(&cdspb.BuildRegistryRequest{}, &mockRegistryBuildStream{}))

	assert.False(t, cluster.registryTrusted)
	assert.False(t, cluster.registrySecretCreated)
}

// a rebuilt registry has new credentials: re-decoration must refresh the
// CDS_REGISTRY_* env in running containers, not only the CA.
func TestBuildRegistryRedecorationRefreshesRegistryEnv(t *testing.T) {
	containers := &mockContainerOps{containers: bo.Containers{{Name: "dev-0", Status: bo.KContainerStatusRunning}}}
	srv, err := newgrpcServer(NewConfig(
		withStagingDir(t.TempDir()),
		withStateDir(t.TempDir()),
		withContainerOps(containers),
		withClusterOps(&mockClusterOps{kubeconfig: "KC"}),
		withRegistryOps(&mockRegistryOps{ca: "CA", user: "newuser", pass: "newpass"}),
	))
	require.NoError(t, err)
	require.NoError(t, srv.deployments().save(&cdspb.DeployContainerRequest{
		ContainerName:      "dev-0",
		ProjectName:        "p",
		DevcontainerConfig: []byte(kOrchestratedConfig),
	}))

	require.NoError(t, newRegistryServiceServer(srv).BuildRegistry(&cdspb.BuildRegistryRequest{}, &mockRegistryBuildStream{}))

	// the registry env step is the last decoration step
	assert.Contains(t, containers.lastExecCmd, "CDS_REGISTRY_USR=newuser")
	assert.Contains(t, containers.lastExecCmd, "CDS_REGISTRY_PWD=newpass")
	assert.Contains(t, containers.lastExecCmd, fmt.Sprintf("CDS_REGISTRY_PORT=%d", registry.DefaultPort))
}
