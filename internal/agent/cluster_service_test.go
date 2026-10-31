package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/orchestration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockClusterOps records calls and returns scripted results.
type mockClusterOps struct {
	buildErr         error
	buildProgress    []clusterProgress
	startErr         error
	stopErr          error
	deleteErr        error
	deleteForce      bool
	cluster          orchestration.Cluster
	clusterErr       error
	kubeconfig       string
	kubeconfigErr    error
	appliedMirrors   []orchestration.Mirror
	applyMirrorsErr  error
	ingressDeployed  bool
	deployIngressErr error

	registrySecretCreated bool
	registrySecretErr     error
	registryTrusted       bool
	registryTrustErr      error
}

func (m *mockClusterOps) BuildCluster(_, _ string, report clusterProgressReporter) error {
	for _, p := range m.buildProgress {
		if err := report(p); err != nil {
			return err
		}
	}
	return m.buildErr
}
func (m *mockClusterOps) StartCluster() error { return m.startErr }
func (m *mockClusterOps) StopCluster() error  { return m.stopErr }
func (m *mockClusterOps) DeleteCluster(force bool) error {
	m.deleteForce = force
	return m.deleteErr
}
func (m *mockClusterOps) GetCluster() (orchestration.Cluster, error) {
	return m.cluster, m.clusterErr
}
func (m *mockClusterOps) GetKubeconfig() (string, error) {
	return m.kubeconfig, m.kubeconfigErr
}
func (m *mockClusterOps) ApplyMirrors(mirrors []orchestration.Mirror) error {
	m.appliedMirrors = mirrors
	return m.applyMirrorsErr
}
func (m *mockClusterOps) DeployIngress() error {
	m.ingressDeployed = true
	return m.deployIngressErr
}
func (m *mockClusterOps) CreateRegistrySecret(host string, port int, user, pass string) error {
	m.registrySecretCreated = true
	return m.registrySecretErr
}
func (m *mockClusterOps) TrustRegistryOnNodes(host string, port int, caPEM string) error {
	m.registryTrusted = true
	return m.registryTrustErr
}

func newTestClusterService(t *testing.T, ops clusterOps) *clusterServiceServer {
	t.Helper()
	srv, err := newgrpcServer(NewConfig(
		withStagingDir(t.TempDir()),
		withStateDir(t.TempDir()),
		withClusterOps(ops),
	))
	require.NoError(t, err)
	return newClusterServiceServer(srv)
}

// mockBuildStream captures BuildCluster streamed events.
type mockBuildStream struct {
	events []*cdspb.BuildClusterEvent
	grpc.ServerStreamingServer[cdspb.BuildClusterEvent]
}

func (m *mockBuildStream) Context() context.Context { return context.Background() }
func (m *mockBuildStream) Send(e *cdspb.BuildClusterEvent) error {
	m.events = append(m.events, e)
	return nil
}

func TestBuildClusterStreamsProgressThenResponse(t *testing.T) {
	ops := &mockClusterOps{buildProgress: []clusterProgress{{phase: "network", message: "net"}, {phase: "cluster", message: "creating"}}}
	svc := newTestClusterService(t, ops)
	stream := &mockBuildStream{}

	err := svc.BuildCluster(&cdspb.BuildClusterRequest{Engine: "kind"}, stream)
	require.NoError(t, err)

	// init + 2 progress + final response
	require.Len(t, stream.events, 4)
	assert.Equal(t, "init", stream.events[0].GetProgress().GetPhase())
	assert.Equal(t, "network", stream.events[1].GetProgress().GetPhase())
	assert.Equal(t, "cluster", stream.events[2].GetProgress().GetPhase())
	resp := stream.events[3].GetResponse()
	require.NotNil(t, resp)
	assert.Equal(t, orchestration.ClusterName, resp.GetName())
	assert.Empty(t, resp.GetError())
}

func TestBuildClusterReportsErrorInResponse(t *testing.T) {
	ops := &mockClusterOps{buildErr: errors.New("boom")}
	svc := newTestClusterService(t, ops)
	stream := &mockBuildStream{}

	err := svc.BuildCluster(&cdspb.BuildClusterRequest{}, stream)
	require.NoError(t, err, "a build failure is reported in the response, not as an RPC error")
	resp := stream.events[len(stream.events)-1].GetResponse()
	require.NotNil(t, resp)
	assert.Contains(t, resp.GetError(), "boom")
}

func TestStartStopDeleteCluster(t *testing.T) {
	ops := &mockClusterOps{}
	svc := newTestClusterService(t, ops)

	_, err := svc.StartCluster(context.Background(), &cdspb.StartClusterRequest{})
	require.NoError(t, err)
	_, err = svc.StopCluster(context.Background(), &cdspb.StopClusterRequest{})
	require.NoError(t, err)
	_, err = svc.DeleteCluster(context.Background(), &cdspb.DeleteClusterRequest{Force: true})
	require.NoError(t, err)
	assert.True(t, ops.deleteForce, "force is threaded to the ops layer")
}

func TestStartClusterMapsErrorToInternal(t *testing.T) {
	ops := &mockClusterOps{startErr: errors.New("nope")}
	svc := newTestClusterService(t, ops)

	_, err := svc.StartCluster(context.Background(), &cdspb.StartClusterRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestGetClusterReturnsProtoWithHost(t *testing.T) {
	ops := &mockClusterOps{cluster: orchestration.Cluster{
		Name:   orchestration.ClusterName,
		Node:   orchestration.ControlPlaneNode,
		Status: bo.KContainerStatusRunning,
	}}
	svc := newTestClusterService(t, ops)

	resp, err := svc.GetCluster(context.Background(), &cdspb.GetClusterRequest{})
	require.NoError(t, err)
	assert.Equal(t, orchestration.ClusterName, resp.GetCluster().GetName())
	assert.Equal(t, "running", resp.GetCluster().GetStatus())
	assert.NotEmpty(t, resp.GetCluster().GetHost())
}

func TestGetKubeconfigReturnsConfig(t *testing.T) {
	ops := &mockClusterOps{kubeconfig: "apiVersion: v1\nkind: Config\n"}
	svc := newTestClusterService(t, ops)

	resp, err := svc.GetKubeconfig(context.Background(), &cdspb.GetKubeconfigRequest{})
	require.NoError(t, err)
	assert.Contains(t, resp.GetKubeconfig(), "kind: Config")
}
