package cenv

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCenvTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)
	cos.SetMockedFileSystem()
	t.Cleanup(func() {
		SetConfigDirForClient()
		cos.SetRealFileSystem()
	})
	return root
}

func TestConfigDirFollowsProcessMode(t *testing.T) {
	root := setupCenvTest(t)

	SetConfigDirForClient()
	assert.Equal(t, filepath.Join(root, ".cds", "certs"), ConfigDir("certs"))

	SetConfigDirForAgent()
	assert.Equal(t, filepath.Join(root, ".cds-agent", "certs"), ConfigDir("certs"))
}

func TestClientAndAgentConfigDirIgnoreProcessMode(t *testing.T) {
	root := setupCenvTest(t)

	for _, setMode := range []func(){SetConfigDirForClient, SetConfigDirForAgent} {
		setMode()
		assert.Equal(t, filepath.Join(root, ".cds", "certs"), ClientConfigDir("certs"))
		assert.Equal(t, filepath.Join(root, ".cds-agent", "certs"), AgentConfigDir("certs"))
	}
}

func TestCreateTempFileWithContentEnsuresTempDirectory(t *testing.T) {
	setupCenvTest(t)
	require.False(t, cos.Exists(ConfigDir(kTmp)))

	path, err := CreateTempFileWithContent(strings.NewReader("content"))

	require.NoError(t, err)
	assert.Equal(t, ConfigDir(kTmp), filepath.Dir(path))
	content, err := cos.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestCacheDirFollowsProcessMode(t *testing.T) {
	root := setupCenvTest(t)

	SetConfigDirForAgent()
	assert.Equal(t, filepath.Join(root, ".cds-agent", "cache"), CacheDir())
}

func TestRemoveTempFile(t *testing.T) {
	root := setupCenvTest(t)
	path, err := CreateTempFileWithContent(strings.NewReader("content"))
	require.NoError(t, err)

	require.NoError(t, RemoveTempFile(path))
	assert.False(t, cos.Exists(path))
	require.NoError(t, RemoveTempFile(path), "removing an already removed temp file is not an error")

	configFile := filepath.Join(root, ".cds", "cliconfig.yaml")
	require.NoError(t, cos.WriteFile(configFile, []byte("content"), 0600))
	for _, outside := range []string{configFile, ConfigDir(kTmp), filepath.Join(ConfigDir(kTmp), "..", "cliconfig.yaml")} {
		assert.Error(t, RemoveTempFile(outside), outside)
	}
	assert.True(t, cos.Exists(configFile))
}
