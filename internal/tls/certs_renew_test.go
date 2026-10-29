package cdstls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCertWithValidity writes a self-signed PEM certificate to path whose
// validity window is [notBefore, notAfter]. Content is irrelevant beyond its
// dates; certValidNow only parses and checks the window.
func writeCertWithValidity(t *testing.T, path string, notBefore, notAfter time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
}

func TestCertValidNow(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	valid := filepath.Join(dir, "valid.pem")
	writeCertWithValidity(t, valid, now.Add(-time.Hour), now.Add(time.Hour))
	assert.NoError(t, certValidNow(valid))

	expired := filepath.Join(dir, "expired.pem")
	writeCertWithValidity(t, expired, now.Add(-2*time.Hour), now.Add(-time.Hour))
	assert.Error(t, certValidNow(expired), "an expired certificate is not valid now")

	notYet := filepath.Join(dir, "notyet.pem")
	writeCertWithValidity(t, notYet, now.Add(time.Hour), now.Add(2*time.Hour))
	assert.Error(t, certValidNow(notYet), "a not-yet-valid certificate is not valid now")

	assert.Error(t, certValidNow(filepath.Join(dir, "missing.pem")))

	garbage := filepath.Join(dir, "garbage.pem")
	require.NoError(t, os.WriteFile(garbage, []byte("not a pem"), 0o600))
	assert.Error(t, certValidNow(garbage))
}

// TestEnsureCertsRenewsExpiredLeaf covers P2: an expired client leaf must be
// reissued from the existing CA, not reused because the file exists. The CA is
// preserved.
func TestEnsureCertsRenewsExpiredLeaf(t *testing.T) {
	requireCfssl(t)
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)
	clientDir := filepath.Join(root, ".xcds", "certs")

	require.NoError(t, EnsureCerts())
	caBefore, err := os.ReadFile(filepath.Join(clientDir, "ca.pem"))
	require.NoError(t, err)

	// replace the valid client leaf with an expired one
	clientCert := filepath.Join(clientDir, "client.pem")
	writeCertWithValidity(t, clientCert, time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))
	require.Error(t, certValidNow(clientCert), "the leaf is now expired")

	require.NoError(t, EnsureCerts())

	assert.NoError(t, certValidNow(clientCert), "the expired leaf was renewed")
	caAfter, err := os.ReadFile(filepath.Join(clientDir, "ca.pem"))
	require.NoError(t, err)
	assert.Equal(t, caBefore, caAfter, "the CA (trust root) is preserved across renewal")
}

// TestAgentCertsDirReissuesAfterCARegeneration covers P2: a cached host leaf
// signed by a previous CA must be reissued after the client PKI is rebuilt,
// not returned by date validity alone (it would fail against the new CA).
func TestAgentCertsDirReissuesAfterCARegeneration(t *testing.T) {
	requireCfssl(t)
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)

	dir, err := AgentCertsDir("remote.example.com")
	require.NoError(t, err)
	firstLeaf, err := os.ReadFile(filepath.Join(dir, "agent-srv.pem"))
	require.NoError(t, err)

	// regenerate the whole client PKI (new CA), then request the host certs again
	require.NoError(t, BuildCerts())
	dir2, err := AgentCertsDir("remote.example.com")
	require.NoError(t, err)
	assert.Equal(t, dir, dir2)
	secondLeaf, err := os.ReadFile(filepath.Join(dir, "agent-srv.pem"))
	require.NoError(t, err)

	assert.NotEqual(t, firstLeaf, secondLeaf, "the leaf was reissued under the new CA")
	// the reissued leaf chains to the current client CA and matches its key
	assert.NoError(t, certMatchesCAAndKey(
		filepath.Join(dir, "agent-srv.pem"),
		filepath.Join(dir, "agent-srv-key.pem"),
		filepath.Join(root, ".xcds", "certs", "ca.pem"),
	))
}

// TestEnsureCertsReissuesOnKeyMismatch covers P2: a date-valid leaf whose key
// was replaced must be reissued, because the pair no longer works for TLS.
func TestEnsureCertsReissuesOnKeyMismatch(t *testing.T) {
	requireCfssl(t)
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)
	clientDir := filepath.Join(root, ".xcds", "certs")

	require.NoError(t, EnsureCerts())
	clientKey := filepath.Join(clientDir, "client-key.pem")
	original, err := os.ReadFile(clientKey)
	require.NoError(t, err)

	// replace the client key with a different, unrelated key
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(other)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(clientKey, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600))

	require.NoError(t, EnsureCerts())

	renewed, err := os.ReadFile(clientKey)
	require.NoError(t, err)
	assert.NotEqual(t, other, renewed, "the mismatched key was replaced by a reissue")
	// the reissued pair is consistent and chains to the CA
	assert.NoError(t, certMatchesCAAndKey(
		filepath.Join(clientDir, "client.pem"), clientKey, filepath.Join(clientDir, "ca.pem"),
	))
	assert.NotEqual(t, original, renewed) // sanity: something changed
}
