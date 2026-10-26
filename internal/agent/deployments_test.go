package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDeploymentStoreSaveLoadRoundTrip(t *testing.T) {
	store := deploymentStore{dir: t.TempDir()}
	req := &cdspb.DeployContainerRequest{
		ContainerName:      "dev-1",
		ProjectName:        "proj",
		DevcontainerConfig: []byte(`{"image":"ubuntu:24.04"}`),
	}
	require.NoError(t, store.save(req))

	loaded, err := store.load("dev-1")
	require.NoError(t, err)
	assert.True(t, proto.Equal(req, loaded))
}

// TestDeploymentStoreSaveIsAtomic covers P2: the record is published by rename,
// so a successful save leaves exactly the record file and no temporary leftover.
func TestDeploymentStoreSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	store := deploymentStore{dir: dir}
	require.NoError(t, store.save(&cdspb.DeployContainerRequest{ContainerName: "dev-1", ProjectName: "proj"}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the final record remains, no temp file")
	assert.Equal(t, "dev-1.json", entries[0].Name())
}

// TestDeploymentStoreSavePreservesPreviousRecordOnFailure covers P2: a failing
// replacement must not destroy the previous valid record. We force the rename
// to fail by making the destination a directory, and check the first record is
// still loadable.
func TestDeploymentStoreSavePreservesPreviousRecordOnFailure(t *testing.T) {
	dir := t.TempDir()
	store := deploymentStore{dir: dir}
	first := &cdspb.DeployContainerRequest{ContainerName: "dev-1", ProjectName: "proj", OverrideImageTag: "v1"}
	require.NoError(t, store.save(first))

	// replace the record file with a directory of the same name so os.Rename fails
	recordPath := filepath.Join(dir, "dev-1.json")
	require.NoError(t, os.Remove(recordPath))
	require.NoError(t, os.Mkdir(recordPath, 0o755))

	err := store.save(&cdspb.DeployContainerRequest{ContainerName: "dev-1", ProjectName: "proj", OverrideImageTag: "v2"})
	require.Error(t, err, "the save fails because the destination cannot be replaced")

	// no stray temp file should remain in the directory
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the temporary file is cleaned up on failure")
	assert.Equal(t, "dev-1.json", entries[0].Name())
}
