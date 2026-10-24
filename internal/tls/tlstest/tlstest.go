// Package tlstest provides helpers to generate the mutual TLS material used by
// cds in tests, without relying on cfssl or on files outside of the test sandbox.
package tlstest

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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	kPermDir  = 0700
	kPermFile = 0600
)

// WriteCerts generates a CA, an agent server certificate (valid for localhost
// and 127.0.0.1) and a client certificate, and writes them into dir using the
// same file names as the cdstls package (ca.pem, agent-srv.pem, agent-srv-key.pem,
// client.pem, client-key.pem).
func WriteCerts(t testing.TB, dir string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, kPermDir))

	caKey := newKey(t)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "cds-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	writeCert(t, filepath.Join(dir, "ca.pem"), caDER)

	issue := func(serial int64, cn string, usage x509.ExtKeyUsage, dnsNames []string, ips []net.IP, certFile, keyFile string) {
		key := newKey(t)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
			DNSNames:     dnsNames,
			IPAddresses:  ips,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		require.NoError(t, err)
		writeCert(t, filepath.Join(dir, certFile), der)
		writeKey(t, filepath.Join(dir, keyFile), key)
	}

	issue(2, "127.0.0.1", x509.ExtKeyUsageServerAuth, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, "agent-srv.pem", "agent-srv-key.pem")
	issue(3, "client", x509.ExtKeyUsageClientAuth, nil, nil, "client.pem", "client-key.pem")
}

func newKey(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func writeCert(t testing.TB, path string, der []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), kPermFile))
}

func writeKey(t testing.TB, path string, key *ecdsa.PrivateKey) {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), kPermFile))
}
