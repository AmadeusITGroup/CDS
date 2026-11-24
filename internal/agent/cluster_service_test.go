package agent

import (
	"context"
	"errors"
	"fmt"
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
	// registryPorts are the ports passed to the trust and pull-secret calls.
	registryPorts []int
}

func (m *mockClusterOps) BuildCluster(_ context.Context, _, _ string, report clusterProgressReporter) error {
	for _, p := range m.buildProgress {
		if err := report(p); err != nil {
			return err
		}
	}
	return m.buildErr
}
func (m *mockClusterOps) StartCluster(context.Context) error { return m.startErr }
func (m *mockClusterOps) StopCluster(context.Context) error  { return m.stopErr }
func (m *mockClusterOps) DeleteCluster(_ context.Context, force bool) error {
	m.deleteForce = force
	return m.deleteErr
}
func (m *mockClusterOps) GetCluster() (orchestration.Cluster, error) {
	return m.cluster, m.clusterErr
}
func (m *mockClusterOps) GetKubeconfig() (string, error) {
	return m.kubeconfig, m.kubeconfigErr
}
func (m *mockClusterOps) ApplyMirrors(_ context.Context, mirrors []orchestration.Mirror) error {
	m.appliedMirrors = mirrors
	return m.applyMirrorsErr
}
func (m *mockClusterOps) DeployIngress(context.Context) error {
	m.ingressDeployed = true
	return m.deployIngressErr
}
func (m *mockClusterOps) CreateRegistrySecret(_ context.Context, host string, port int, user, pass string) error {
	m.registrySecretCreated = true
	m.registryPorts = append(m.registryPorts, port)
	return m.registrySecretErr
}
func (m *mockClusterOps) TrustRegistryOnNodes(_ context.Context, host string, port int, caPEM string) error {
	m.registryTrusted = true
	m.registryPorts = append(m.registryPorts, port)
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

// a delete refused because containers use the cluster is the caller's
// precondition, not an agent fault (CDS-032); other failures stay Internal.
func TestDeleteClusterMapsInUseToFailedPrecondition(t *testing.T) {
	inUse := guardDelete("cluster", false, func() ([]string, error) { return []string{"dev-a"}, nil })
	svc := newTestClusterService(t, &mockClusterOps{deleteErr: inUse})
	_, err := svc.DeleteCluster(context.Background(), &cdspb.DeleteClusterRequest{})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "in use by 1 container(s)")

	svc = newTestClusterService(t, &mockClusterOps{deleteErr: errors.New("kind boom")})
	_, err = svc.DeleteCluster(context.Background(), &cdspb.DeleteClusterRequest{})
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

// newTestOrchestrationServer wires mock cluster, registry and container ops and
// records one deployment per config, as a deploy would have.
func newTestOrchestrationServer(t *testing.T, cluster clusterOps, reg registryOps, configs ...string) *grpcServer {
	t.Helper()
	srv, err := newgrpcServer(NewConfig(
		withStagingDir(t.TempDir()),
		withStateDir(t.TempDir()),
		withContainerOps(&mockContainerOps{}),
		withClusterOps(cluster),
		withRegistryOps(reg),
	))
	require.NoError(t, err)
	for i, cfg := range configs {
		require.NoError(t, srv.deployments().save(&cdspb.DeployContainerRequest{
			ContainerName:      fmt.Sprintf("dev-%d", i),
			ProjectName:        "p",
			DevcontainerConfig: []byte(cfg),
		}))
	}
	return srv
}

const kOrchestratedConfig = `{"image":"ubuntu:24.04","orchestration":{"engine":"kind","ingress":true,
	"mirrors":[{"registry":"docker.io","endpoint":"https://registry-1.docker.io"}],
	"registry":{"secure":true}}}`

// TestBuildClusterReappliesRecordedOrchestration covers CDS-031: a rebuilt
// cluster gets back the mirrors, ingress, registry CA and pull secret the
// recorded deployments asked for, not only the kubeconfig.
func TestBuildClusterReappliesRecordedOrchestration(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{ca: "CA", user: "u", pass: "p"}
	srv := newTestOrchestrationServer(t, cluster, reg, kOrchestratedConfig, `{"image":"ubuntu:24.04"}`)

	require.NoError(t, newClusterServiceServer(srv).BuildCluster(&cdspb.BuildClusterRequest{}, &mockBuildStream{}))

	assert.Equal(t, []orchestration.Mirror{{Registry: "docker.io", Endpoint: "https://registry-1.docker.io"}}, cluster.appliedMirrors)
	assert.True(t, cluster.ingressDeployed, "ingress is redeployed")
	assert.True(t, cluster.registryTrusted, "registry CA is trusted on the new nodes")
	assert.True(t, cluster.registrySecretCreated, "pull secret is recreated")
}

func TestBuildClusterReappliesNothingWithoutOrchestratedDeployments(t *testing.T) {
	cluster := &mockClusterOps{}
	srv := newTestOrchestrationServer(t, cluster, &mockRegistryOps{ca: "CA"}, `{"image":"ubuntu:24.04"}`)

	require.NoError(t, newClusterServiceServer(srv).BuildCluster(&cdspb.BuildClusterRequest{}, &mockBuildStream{}))

	assert.Nil(t, cluster.appliedMirrors)
	assert.False(t, cluster.ingressDeployed)
	assert.False(t, cluster.registryTrusted)
	assert.False(t, cluster.registrySecretCreated)
}

// a failing re-apply step is reported, not fatal: the cluster itself was built.
func TestBuildClusterReapplyFailureDoesNotFailBuild(t *testing.T) {
	cluster := &mockClusterOps{deployIngressErr: errors.New("ingress boom")}
	srv := newTestOrchestrationServer(t, cluster, &mockRegistryOps{ca: "CA"}, kOrchestratedConfig)
	stream := &mockBuildStream{}

	require.NoError(t, newClusterServiceServer(srv).BuildCluster(&cdspb.BuildClusterRequest{}, stream))

	resp := stream.events[len(stream.events)-1].GetResponse()
	require.NotNil(t, resp)
	assert.Empty(t, resp.GetError())
	assert.True(t, cluster.registrySecretCreated, "later steps still run")
}
