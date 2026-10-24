package cdstls

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertFilePathsFollowConfigPath(t *testing.T) {
	first := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", first)
	assert.Equal(t, filepath.Join(first, ".xcds", "certs", "ca.pem"), CAFilePath())

	second := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", second)
	certsDir := filepath.Join(second, ".xcds", "certs")
	assert.Equal(t, filepath.Join(certsDir, "ca.pem"), CAFilePath())
	assert.Equal(t, filepath.Join(certsDir, "agent-srv.pem"), AgentServerCertFilePath())
	assert.Equal(t, filepath.Join(certsDir, "agent-srv-key.pem"), AgentServerKeyFilePath())
	assert.Equal(t, filepath.Join(certsDir, "client.pem"), ClientCertFilePath())
	assert.Equal(t, filepath.Join(certsDir, "client-key.pem"), ClientKeyFilePath())
}

func TestCertFilePathsFollowAgentMode(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)
	cenv.SetConfigDirForAgent()
	t.Cleanup(cenv.SetConfigDirForClient)

	assert.Equal(t, filepath.Join(root, ".xcds-agent", "certs", "ca.pem"), CAFilePath())
	assert.Equal(t, filepath.Join(root, ".xcds-agent", "certs", "agent-srv.pem"), AgentServerCertFilePath())
}

func TestInstallCertsSplitsClientAndAgentMaterial(t *testing.T) {
	root := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)

	generated := []string{"ca.pem", "ca-key.pem", "ca.csr", "client.pem", "client-key.pem", "client.csr", "agent-srv.pem", "agent-srv-key.pem", "agent-srv.csr"}
	for _, name := range generated {
		require.NoError(t, os.WriteFile(filepath.Join(workDir, name), []byte(name), 0600))
	}

	require.NoError(t, installCerts(workDir))

	assertDirContent := func(dir string, want ...string) {
		t.Helper()
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
			info, err := e.Info()
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), e.Name())
			content, err := os.ReadFile(filepath.Join(dir, e.Name()))
			require.NoError(t, err)
			assert.Equal(t, e.Name(), string(content))
		}
		assert.ElementsMatch(t, want, got)
	}
	assertDirContent(filepath.Join(root, ".xcds", "certs"), "ca.pem", "ca-key.pem", "client.pem", "client-key.pem")
	assertDirContent(filepath.Join(root, ".xcds-agent", "certs"), "ca.pem", "agent-srv.pem", "agent-srv-key.pem")
}

func TestInstallCertsFailsOnMissingMaterial(t *testing.T) {
	t.Setenv("CDS_CONFIG_PATH", t.TempDir())

	assert.Error(t, installCerts(t.TempDir()))
}
