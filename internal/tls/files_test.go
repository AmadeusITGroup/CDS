package cdstls

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
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

func requireCfssl(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"cfssl", "cfssljson"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not installed", bin)
		}
	}
}

// verifyCert checks that certFile is signed by caFile and valid for usage and dnsName.
func verifyCert(t *testing.T, caFile, certFile string, usage x509.ExtKeyUsage, dnsName string) {
	t.Helper()
	pool := x509.NewCertPool()
	caPEM, err := os.ReadFile(caFile)
	require.NoError(t, err)
	require.True(t, pool.AppendCertsFromPEM(caPEM))
	certPEM, err := os.ReadFile(certFile)
	require.NoError(t, err)
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	_, err = cert.Verify(x509.VerifyOptions{Roots: pool, DNSName: dnsName, KeyUsages: []x509.ExtKeyUsage{usage}})
	require.NoError(t, err)
}

func TestEnsureCertsKeepsTheCA(t *testing.T) {
	requireCfssl(t)
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)
	clientDir := filepath.Join(root, ".xcds", "certs")
	agentDir := filepath.Join(root, ".xcds-agent", "certs")

	require.NoError(t, EnsureCerts())
	ca, err := os.ReadFile(filepath.Join(clientDir, "ca.pem"))
	require.NoError(t, err)
	verifyCert(t, filepath.Join(clientDir, "ca.pem"), filepath.Join(clientDir, "client.pem"), x509.ExtKeyUsageClientAuth, "")
	verifyCert(t, filepath.Join(agentDir, "ca.pem"), filepath.Join(agentDir, "agent-srv.pem"), x509.ExtKeyUsageServerAuth, "localhost")

	require.NoError(t, EnsureCerts())
	require.NoError(t, os.RemoveAll(agentDir))
	require.NoError(t, os.Remove(filepath.Join(clientDir, "client.pem")))
	require.NoError(t, EnsureCerts())

	again, err := os.ReadFile(filepath.Join(clientDir, "ca.pem"))
	require.NoError(t, err)
	assert.Equal(t, ca, again, "the CA must not be regenerated")
	verifyCert(t, filepath.Join(clientDir, "ca.pem"), filepath.Join(clientDir, "client.pem"), x509.ExtKeyUsageClientAuth, "")
	verifyCert(t, filepath.Join(clientDir, "ca.pem"), filepath.Join(agentDir, "agent-srv.pem"), x509.ExtKeyUsageServerAuth, "localhost")
}

func TestAgentCertsDir(t *testing.T) {
	requireCfssl(t)
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)

	dir, err := AgentCertsDir("remote.example.com")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".xcds", "certs", "agents", "remote.example.com"), dir)
	verifyCert(t, filepath.Join(root, ".xcds", "certs", "ca.pem"), filepath.Join(dir, "agent-srv.pem"), x509.ExtKeyUsageServerAuth, "remote.example.com")
	assert.FileExists(t, filepath.Join(dir, "ca.pem"))
	assert.FileExists(t, filepath.Join(dir, "agent-srv-key.pem"))
	assert.NoFileExists(t, filepath.Join(dir, "ca-key.pem"), "the CA key never leaves the client certs dir")

	issued, err := os.ReadFile(filepath.Join(dir, "agent-srv.pem"))
	require.NoError(t, err)
	_, err = AgentCertsDir("remote.example.com")
	require.NoError(t, err)
	again, err := os.ReadFile(filepath.Join(dir, "agent-srv.pem"))
	require.NoError(t, err)
	assert.Equal(t, issued, again, "certificates are issued once per host")

	for _, invalid := range []string{"", "../escape", "a/b", ".hidden"} {
		_, err := AgentCertsDir(invalid)
		assert.Error(t, err, invalid)
	}
}
