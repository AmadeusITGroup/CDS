package cenv

import (
	"strings"
	"testing"
	"time"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTempFileWithContentEnsuresTempDirectory(t *testing.T) {
	setupCenvTestFS(t)

	path, err := CreateTempFileWithContent(strings.NewReader("content"))

	require.NoError(t, err)
	assert.True(t, cos.Exists(path))
	content, err := cos.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestRemoveTempFileRefusesNonTempPath(t *testing.T) {
	setupCenvTestFS(t)

	require.NoError(t, cos.Fs.MkdirAll("/tmp/testconfig/.xcds", 0755))
	require.NoError(t, cos.WriteFile("/tmp/testconfig/.xcds/config.yaml", []byte("content"), 0600))

	err := RemoveTempFile("/tmp/testconfig/.xcds/config.yaml")

	require.Error(t, err)
	assert.True(t, cos.Exists("/tmp/testconfig/.xcds/config.yaml"))
}

func TestCleanupStaleTempFilesOnlyRemovesOldManagedFiles(t *testing.T) {
	setupCenvTestFS(t)

	tmpDir := ConfigDir(kTmp)
	require.NoError(t, EnsureDir(tmpDir, 0700))
	oldTime := time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 20, 8, 30, 0, 0, time.UTC)
	now := time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)
	staleManaged := tmpDir + "/tmp-file-stale"
	staleLegacyManaged := tmpDir + "/cds-tmp-stale"
	freshManaged := tmpDir + "/tmp-file-fresh"
	staleUnmanaged := tmpDir + "/other-stale"

	for _, path := range []string{staleManaged, staleLegacyManaged, freshManaged, staleUnmanaged} {
		require.NoError(t, cos.WriteFile(path, []byte("content"), 0600))
	}
	require.NoError(t, cos.Fs.Chtimes(staleManaged, oldTime, oldTime))
	require.NoError(t, cos.Fs.Chtimes(staleLegacyManaged, oldTime, oldTime))
	require.NoError(t, cos.Fs.Chtimes(staleUnmanaged, oldTime, oldTime))
	require.NoError(t, cos.Fs.Chtimes(freshManaged, newTime, newTime))

	require.NoError(t, cleanupStaleTempFiles(now, 24*time.Hour))

	assert.False(t, cos.Exists(staleManaged))
	assert.False(t, cos.Exists(staleLegacyManaged))
	assert.True(t, cos.Exists(freshManaged))
	assert.True(t, cos.Exists(staleUnmanaged))
}

func setupCenvTestFS(t *testing.T) {
	t.Helper()

	cos.Fs = afero.NewMemMapFs()
	SetConfigDirForClient()
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")
	t.Cleanup(func() {
		SetConfigDirForClient()
		cos.SetRealFileSystem()
	})
}
