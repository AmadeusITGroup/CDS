package cdstls

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertFilePathsFollowActiveConfigDirectory(t *testing.T) {
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")
	t.Cleanup(cenv.SetConfigDirForClient)

	cenv.SetConfigDirForClient()
	assert.Equal(t, filepath.Join("/tmp/testconfig", ".xcds", "certs", "ca.pem"), CAFilePath())
	assert.Equal(t, filepath.Join("/tmp/testconfig", ".xcds", "certs", "client.pem"), ClientCertFilePath())

	cenv.SetConfigDirForAgent()
	assert.Equal(t, filepath.Join("/tmp/testconfig", ".xcds-agent", "certs", "ca.pem"), CAFilePath())
	assert.Equal(t, filepath.Join("/tmp/testconfig", ".xcds-agent", "certs", "agent-srv.pem"), AgentServerCertFilePath())
}

func TestInstallRuntimeCertsSplitsClientAndAgentFiles(t *testing.T) {
	configRoot := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", configRoot)
	cenv.SetConfigDirForClient()
	t.Cleanup(cenv.SetConfigDirForClient)

	for _, name := range []string{"ca.pem", "client.pem", "client-key.pem", "agent-srv.pem", "agent-srv-key.pem", "ca-key.pem", "client.csr"} {
		require.NoError(t, os.WriteFile(filepath.Join(workDir, name), []byte(name), 0600))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(configRoot, ".xcds", "certsjson"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(configRoot, ".xcds-agent", "certsjson"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(configRoot, ".xcds", "certs"), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(configRoot, ".xcds-agent", "certs"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(configRoot, ".xcds", "certs", "agent-srv.pem"), []byte("stale"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(configRoot, ".xcds", "certs", "ca-key.pem"), []byte("stale"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(configRoot, ".xcds-agent", "certs", "client.pem"), []byte("stale"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(configRoot, ".xcds-agent", "certs", "client.csr"), []byte("stale"), 0600))

	require.NoError(t, installRuntimeCerts(workDir))

	clientCertsDir := filepath.Join(configRoot, ".xcds", "certs")
	agentCertsDir := filepath.Join(configRoot, ".xcds-agent", "certs")
	assert.FileExists(t, filepath.Join(clientCertsDir, "ca.pem"))
	assert.FileExists(t, filepath.Join(clientCertsDir, "client.pem"))
	assert.FileExists(t, filepath.Join(clientCertsDir, "client-key.pem"))
	assert.NoFileExists(t, filepath.Join(clientCertsDir, "agent-srv.pem"))
	assert.NoFileExists(t, filepath.Join(clientCertsDir, "agent-srv-key.pem"))
	assert.NoFileExists(t, filepath.Join(clientCertsDir, "ca-key.pem"))

	assert.FileExists(t, filepath.Join(agentCertsDir, "ca.pem"))
	assert.FileExists(t, filepath.Join(agentCertsDir, "agent-srv.pem"))
	assert.FileExists(t, filepath.Join(agentCertsDir, "agent-srv-key.pem"))
	assert.NoFileExists(t, filepath.Join(agentCertsDir, "client.pem"))
	assert.NoFileExists(t, filepath.Join(agentCertsDir, "client-key.pem"))
	assert.NoFileExists(t, filepath.Join(agentCertsDir, "ca-key.pem"))
	assert.NoFileExists(t, filepath.Join(agentCertsDir, "client.csr"))
	assert.NoDirExists(t, filepath.Join(configRoot, ".xcds", "certsjson"))
	assert.NoDirExists(t, filepath.Join(configRoot, ".xcds-agent", "certsjson"))
}
