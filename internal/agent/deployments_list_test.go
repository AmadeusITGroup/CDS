package agent

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentStoreListEmptyWhenNoDir(t *testing.T) {
	store := deploymentStore{dir: t.TempDir() + "/missing"}
	reqs, err := store.list()
	require.NoError(t, err)
	assert.Empty(t, reqs)
}

func TestDeploymentStoreListReturnsRecorded(t *testing.T) {
	store := deploymentStore{dir: t.TempDir()}
	require.NoError(t, store.save(&cdspb.DeployContainerRequest{ContainerName: "dev-a", ProjectName: "p"}))
	require.NoError(t, store.save(&cdspb.DeployContainerRequest{ContainerName: "dev-b", ProjectName: "p"}))

	reqs, err := store.list()
	require.NoError(t, err)
	names := []string{}
	for _, r := range reqs {
		names = append(names, r.GetContainerName())
	}
	assert.ElementsMatch(t, []string{"dev-a", "dev-b"}, names)
}
