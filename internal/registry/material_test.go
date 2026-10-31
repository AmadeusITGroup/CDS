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
