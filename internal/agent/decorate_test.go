package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureOrchestrationNoopWhenNotRequested(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}
	var built int32
	cluster.buildProgress = nil

	err := ensureOrchestration(context.Background(), cluster, reg,
		containerconf.KindOrchestrationConfig{Requested: false},
		func(deployProgress) error { atomic.AddInt32(&built, 1); return nil })

	require.NoError(t, err)
	assert.Zero(t, built, "nothing is ensured when orchestration is not requested")
}

func TestEnsureOrchestrationBuildsClusterAndRegistry(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{
		Requested: true,
		Engine:    "kind",
		Registry:  containerconf.RegistryConfig{Requested: true, Secure: true},
	}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	assert.True(t, reg.secure, "registry built when requested")
}

func TestEnsureOrchestrationSkipsRegistryWhenNotRequested(t *testing.T) {
	cluster := &mockClusterOps{}
	// a registry that would error if built, to prove it is not built
	reg := &mockRegistryOps{buildErr: errors.New("registry should not be built")}

	oc := containerconf.KindOrchestrationConfig{
		Requested: true,
		Registry:  containerconf.RegistryConfig{Requested: false},
	}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err, "registry is not built when not requested, so its error never surfaces")
}

func TestEnsureOrchestrationPropagatesClusterError(t *testing.T) {
	cluster := &mockClusterOps{buildErr: errors.New("cluster boom")}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{Requested: true}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster boom")
}

func TestContainerDecorationCommandsShape(t *testing.T) {
	cmds := containerDecorationCommands("", "dev-1", "KUBECONFIG_CONTENT", "CA_PEM")
	require.Len(t, cmds, 2)
	assert.Contains(t, cmds[0].script, "/root/.kube/config")
	assert.Contains(t, cmds[0].script, "KUBECONFIG_CONTENT")
	assert.Contains(t, cmds[1].script, "/etc/containers/certs.d/"+registry.Hostname)
	assert.Contains(t, cmds[1].script, "CA_PEM")
}

func TestContainerDecorationCommandsSkipEmpty(t *testing.T) {
	assert.Empty(t, containerDecorationCommands("", "dev-1", "", ""))

	onlyKube := containerDecorationCommands("", "dev-1", "KC", "")
	require.Len(t, onlyKube, 1)
	assert.Contains(t, onlyKube[0].script, "/root/.kube/config")
}

func TestEnsureOrchestrationAppliesMirrors(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{
		Requested: true,
		Mirrors: []containerconf.Mirror{
			{Registry: "docker.io", Endpoint: "https://mirror/docker"},
		},
	}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	require.Len(t, cluster.appliedMirrors, 1)
	assert.Equal(t, "docker.io", cluster.appliedMirrors[0].Registry)
	assert.Equal(t, "https://mirror/docker", cluster.appliedMirrors[0].Endpoint)
}

func TestEnsureOrchestrationDeploysIngressWhenRequested(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{Requested: true, Ingress: true}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	assert.True(t, cluster.ingressDeployed, "ingress is deployed when requested")
}

func TestEnsureOrchestrationSkipsIngressWhenNotRequested(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{Requested: true, Ingress: false}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	assert.False(t, cluster.ingressDeployed)
}
