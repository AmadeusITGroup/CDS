package registry

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestEnsureMaterialGeneratesCertAndCredentials(t *testing.T) {
	root := t.TempDir()
	m, err := ensureMaterial(root)
	require.NoError(t, err)

	// certificate is valid and issued for Hostname
	certPEM, err := os.ReadFile(m.certFile())
	require.NoError(t, err)
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, Hostname, cert.Subject.CommonName)
	assert.Contains(t, cert.DNSNames, Hostname)

	// a plain self-signed server certificate, not a CA (CDS-038)
	assert.False(t, cert.IsCA, "registry certificate is not a CA")
	assert.Zero(t, cert.KeyUsage&x509.KeyUsageCertSign, "registry certificate cannot sign certificates")
	assert.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)

	// a client that trusts only this certificate verifies the server
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	_, err = cert.Verify(x509.VerifyOptions{DNSName: Hostname, Roots: pool})
	assert.NoError(t, err, "the leaf is accepted as its own trust anchor")

	// htpasswd holds a bcrypt hash of the generated password for the user
	assert.NotEmpty(t, m.username)
	assert.NotEmpty(t, m.password)
	line, err := os.ReadFile(m.htpasswd())
	require.NoError(t, err)
	parts := strings.SplitN(strings.TrimSpace(string(line)), ":", 2)
	require.Len(t, parts, 2)
	assert.Equal(t, m.username, parts[0])
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(parts[1]), []byte(m.password)))
}

func TestEnsureMaterialIsIdempotent(t *testing.T) {
	root := t.TempDir()
	first, err := ensureMaterial(root)
	require.NoError(t, err)
	firstCert, err := os.ReadFile(first.certFile())
	require.NoError(t, err)

	// second call must not rotate the certificate or the username
	second, err := ensureMaterial(root)
	require.NoError(t, err)
	secondCert, err := os.ReadFile(second.certFile())
	require.NoError(t, err)

	assert.Equal(t, firstCert, secondCert, "certificate is not regenerated")
	assert.Equal(t, first.username, second.username, "username is preserved")
}

func TestEnsureMaterialPersistsPlaintextCredentials(t *testing.T) {
	root := t.TempDir()
	first, err := ensureMaterial(root)
	require.NoError(t, err)
	require.NotEmpty(t, first.username)
	require.NotEmpty(t, first.password)

	// a second call reuses the SAME plaintext credentials (recovered from the
	// persisted creds file, not the unrecoverable bcrypt hash).
	second, err := ensureMaterial(root)
	require.NoError(t, err)
	assert.Equal(t, first.username, second.username)
	assert.Equal(t, first.password, second.password, "password survives reuse via the persisted creds file")
}

func TestEnsureMaterialReissuesCertForOldHostname(t *testing.T) {
	// a host provisioned before the rename (CDS-016) holds a certificate for
	// the old name; the next build must reissue it for Hostname and keep the
	// credentials clients already use.
	root := t.TempDir()
	first, err := ensureMaterial(root)
	require.NoError(t, err)
	require.NoError(t, first.generateCertificate("cds.sec.io"))

	second, err := ensureMaterial(root)
	require.NoError(t, err)
	certPEM, err := os.ReadFile(second.certFile())
	require.NoError(t, err)
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, []string{Hostname}, cert.DNSNames)
	assert.Equal(t, first.username, second.username)
	assert.Equal(t, first.password, second.password)
}
