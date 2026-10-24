package cdstls

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
