package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, ops containerOps) *grpcServer {
	t.Helper()
	srv, err := newgrpcServer(NewConfig(
		withStagingDir(t.TempDir()),
		withStateDir(t.TempDir()),
		withContainerOps(ops),
	))
	require.NoError(t, err)
	return srv
}

func newTestContainerService(t *testing.T, ops containerOps) *containerServiceServer {
	t.Helper()
	return newContainerServiceServer(newTestServer(t, ops))
}

func newTestArtifactService(t *testing.T) *artifactServiceServer {
	t.Helper()
	return newArtifactServiceServer(newTestServer(t, defaultContainerOps{}))
}
