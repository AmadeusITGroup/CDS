package registry

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestEnsureMaterialGeneratesCertAndCredentials(t *testing.T) {
	root := t.TempDir()
	m, _, err := ensureMaterial(root)
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

	// a plain self-signed server certificate, not a CA
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
	first, _, err := ensureMaterial(root)
	require.NoError(t, err)
	firstCert, err := os.ReadFile(first.certFile())
	require.NoError(t, err)

	// second call must not rotate the certificate or the username
	second, _, err := ensureMaterial(root)
	require.NoError(t, err)
	secondCert, err := os.ReadFile(second.certFile())
	require.NoError(t, err)

	assert.Equal(t, firstCert, secondCert, "certificate is not regenerated")
	assert.Equal(t, first.username, second.username, "username is preserved")
}

func TestEnsureMaterialPersistsPlaintextCredentials(t *testing.T) {
	root := t.TempDir()
	first, _, err := ensureMaterial(root)
	require.NoError(t, err)
	require.NotEmpty(t, first.username)
	require.NotEmpty(t, first.password)

	// a second call reuses the SAME plaintext credentials (recovered from the
	// persisted creds file, not the unrecoverable bcrypt hash).
	second, _, err := ensureMaterial(root)
	require.NoError(t, err)
	assert.Equal(t, first.username, second.username)
	assert.Equal(t, first.password, second.password, "password survives reuse via the persisted creds file")
}

func TestEnsureMaterialReissuesCertForOldHostname(t *testing.T) {
	// a host provisioned before the rename to registry.cds.internal holds a certificate for
	// the old name; the next build must reissue it for Hostname and keep the
	// credentials clients already use.
	root := t.TempDir()
	first, _, err := ensureMaterial(root)
	require.NoError(t, err)
	require.NoError(t, first.generateCertificate("cds.sec.io"))

	second, _, err := ensureMaterial(root)
	require.NoError(t, err)
	certPEM, err := os.ReadFile(second.certFile())
	require.NoError(t, err)
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, []string{Hostname, "localhost"}, cert.DNSNames)
	require.Len(t, cert.IPAddresses, 1)
	assert.Equal(t, "127.0.0.1", cert.IPAddresses[0].String())
	assert.Equal(t, first.username, second.username)
	assert.Equal(t, first.password, second.password)
}

// writeTestCert overwrites the stored certificate with a self-signed one for
// Hostname, with the given expiry and CA flag, and the loopback SANs unless
// noLoopback.
func writeTestCert(t *testing.T, m material, notAfter time.Time, isCA, noLoopback bool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: Hostname},
		DNSNames:              []string{Hostname},
		NotBefore:             notAfter.Add(-2 * certValidity),
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if !noLoopback {
		tmpl.DNSNames = append(tmpl.DNSNames, "localhost")
		tmpl.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1)}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(m.certFile(), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644))
}

// TestEnsureMaterialReissuesUnusableCert: an expired, soon-expiring or
// CA-flagged certificate is reissued as a plain leaf, the credentials are kept,
// and the change is reported so the registry restarts.
func TestEnsureMaterialReissuesUnusableCert(t *testing.T) {
	for name, tc := range map[string]struct {
		notAfter   time.Time
		isCA       bool
		noLoopback bool
	}{
		"expired":                           {notAfter: time.Now().Add(-time.Hour)},
		"expires soon":                      {notAfter: time.Now().Add(certRenewBefore / 2)},
		"CA (older versions)":               {notAfter: time.Now().Add(certValidity), isCA: true},
		"no loopback SANs (before CDS-075)": {notAfter: time.Now().Add(certValidity), noLoopback: true},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			first, changed, err := ensureMaterial(root)
			require.NoError(t, err)
			require.True(t, changed, "new material is a change")
			writeTestCert(t, first, tc.notAfter, tc.isCA, tc.noLoopback)

			second, changed, err := ensureMaterial(root)
			require.NoError(t, err)
			assert.True(t, changed, "reissue is reported")
			cert := readTestCert(t, second)
			assert.False(t, cert.IsCA)
			assert.NoError(t, cert.VerifyHostname("127.0.0.1"))
			assert.NoError(t, cert.VerifyHostname("localhost"))
			assert.True(t, cert.NotAfter.After(time.Now().Add(certRenewBefore)))
			assert.Equal(t, first.username, second.username)
			assert.Equal(t, first.password, second.password)
		})
	}
}

// TestEnsureMaterialReusesAGoodCert: a valid leaf with every SAN is kept.
func TestEnsureMaterialReusesAGoodCert(t *testing.T) {
	root := t.TempDir()
	first, _, err := ensureMaterial(root)
	require.NoError(t, err)
	writeTestCert(t, first, time.Now().Add(certValidity), false, false)
	_, changed, err := ensureMaterial(root)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestEnsureMaterialReportsNoChangeOnReuse(t *testing.T) {
	root := t.TempDir()
	_, _, err := ensureMaterial(root)
	require.NoError(t, err)
	_, changed, err := ensureMaterial(root)
	require.NoError(t, err)
	assert.False(t, changed)
}

func readTestCert(t *testing.T, m material) *x509.Certificate {
	t.Helper()
	certPEM, err := os.ReadFile(m.certFile())
	require.NoError(t, err)
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}
