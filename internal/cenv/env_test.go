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
	assert.Equal(t, filepath.Join(root, ".xcds", "certs"), ConfigDir("certs"))

	SetConfigDirForAgent()
	assert.Equal(t, filepath.Join(root, ".xcds-agent", "certs"), ConfigDir("certs"))
}

func TestClientAndAgentConfigDirIgnoreProcessMode(t *testing.T) {
	root := setupCenvTest(t)

	for _, setMode := range []func(){SetConfigDirForClient, SetConfigDirForAgent} {
		setMode()
		assert.Equal(t, filepath.Join(root, ".xcds", "certs"), ClientConfigDir("certs"))
		assert.Equal(t, filepath.Join(root, ".xcds-agent", "certs"), AgentConfigDir("certs"))
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
