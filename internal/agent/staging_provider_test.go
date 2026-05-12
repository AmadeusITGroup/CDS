package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStagingResourceProvider_FetchFile(t *testing.T) {
	dir := t.TempDir()
	provider := newStagingResourceProvider(dir, "test-project")
	dockerfileIdentifier, err := containerconf.ResourceIdentifier(containerconf.KindDockerfile, "Dockerfile")
	require.NoError(t, err)

	subDir := filepath.Join(dir, "test-project", "resource", "dockerfile")
	require.NoError(t, os.MkdirAll(subDir, 0700))
	content := []byte("FROM ubuntu:22.04\n")
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "Dockerfile"), content, 0600))

	t.Run("existing file returns content", func(t *testing.T) {
		data, err := provider.FetchFile(containerconf.ResourceTypeFile, dockerfileIdentifier)
		require.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		_, err := provider.FetchFile(containerconf.ResourceTypeFile, "nonexistent/file.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "staging resource")
	})

	t.Run("unsupported resource type returns error", func(t *testing.T) {
		_, err := provider.FetchFile(containerconf.ResourceTypeFeature, dockerfileIdentifier)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not supported")
	})
}

func TestStagingResourceProvider_FileExists(t *testing.T) {
	dir := t.TempDir()
	provider := newStagingResourceProvider(dir, "test-project")

	projectDir := filepath.Join(dir, "test-project")
	require.NoError(t, os.MkdirAll(projectDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "exists.txt"), []byte("data"), 0600))

	assert.True(t, provider.FileExists(containerconf.ResourceTypeFile, "exists.txt"))
	assert.False(t, provider.FileExists(containerconf.ResourceTypeFile, "missing.txt"))
}

func TestStagingResourceProvider_IsProjectScoped(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "project-a"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "project-b"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "project-a", "shared.txt"), []byte("project-a"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "project-b", "shared.txt"), []byte("project-b"), 0600))

	provider := newStagingResourceProvider(dir, "project-b")

	data, err := provider.FetchFile(containerconf.ResourceTypeFile, "shared.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("project-b"), data)
}
