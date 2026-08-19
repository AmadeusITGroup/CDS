package agent

import (
	"context"
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

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cenv"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func TestAgent(t *testing.T) {
	for usecase, fn := range map[string]func(t *testing.T, client cdspb.AgentInfoServiceClient, config *bom){
		"get server version succeeds": testServerVersion,
	} {
		t.Run(usecase, func(t *testing.T) {
			client, config, teardown := setupTest(t, nil)
			defer teardown()
			fn(t, client, config)
		})
	}
}

func setupTest(t *testing.T, fn func(*bom)) (client cdspb.AgentInfoServiceClient, cfg *bom, teardown func()) {
	t.Helper()
	t.Setenv("CDS_CONFIG_PATH", t.TempDir())
	t.Cleanup(cenv.SetConfigDirForClient)
	writeAgentTestCerts(t)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)

	cenv.SetConfigDirForClient()
	// Configure the client’s TLS credentials to use our self-signed CA as the client’s Root CA (the CA it will use to verify the server).
	// Set the client to use those credentials for its connection.
	clientTLSConfig, err := cdstls.SetupTLSConfig(cdstls.TLSConfig{CAFile: cdstls.CAFilePath(),
		// Settings the following two attributes is needed for mutual TLS authentication (server authenticates the client,
		// on top of the default authentication where the client authenticates the server).
		CertFile: cdstls.ClientCertFilePath(),
		KeyFile:  cdstls.ClientKeyFilePath(),
	})
	assert.NoError(t, err)

	clientCreds := credentials.NewTLS(clientTLSConfig)
	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(clientCreds /*insecure.NewCredentials()*/),
	)
	assert.NoError(t, err)

	cenv.SetConfigDirForAgent()
	// Configure the Agent TLS and enable it to handle TLS connections.
	agentTLSConfig, err := cdstls.SetupTLSConfig(cdstls.TLSConfig{
		CertFile:      cdstls.AgentServerCertFilePath(),
		KeyFile:       cdstls.AgentServerKeyFilePath(),
		CAFile:        cdstls.CAFilePath(),
		ServerAddress: lis.Addr().String(),
		Server:        true, // Setting Server attribute to true enable authentication of clients at server side. Mutual TLS authentication use case
	})
	agentCreds := credentials.NewTLS(agentTLSConfig)
	require.NoError(t, err)

	cfg = &bom{}
	if fn != nil {
		fn(cfg)
	}
	server, err := NewAgent(cfg, grpc.Creds(agentCreds))
	assert.NoError(t, err)

	// Serve blocks, needs to be run in its own goroutine
	go func() {
		err := server.Serve(lis)
		require.NoError(t, err)
	}()

	client = cdspb.NewAgentInfoServiceClient(conn)

	return client, cfg, func() {
		server.Stop()
		_ = conn.Close()
		_ = lis.Close()
	}
}

func testServerVersion(t *testing.T, client cdspb.AgentInfoServiceClient, config *bom) {
	ctx := context.Background()
	reply, err := client.GetVersion(ctx, &cdspb.GetVersionRequest{})
	assert.NoError(t, err)
	want := "9.9.9"
	assert.Equal(t, want, reply.GetCurrent(), "they should be equal")

}

func writeAgentTestCerts(t *testing.T) {
	t.Helper()

	caKey, caCert, caPEM := createTestCA(t)
	serverCertPEM, serverKeyPEM := createTestLeafCert(t, caKey, caCert, x509.ExtKeyUsageServerAuth)
	clientCertPEM, clientKeyPEM := createTestLeafCert(t, caKey, caCert, x509.ExtKeyUsageClientAuth)

	cenv.SetConfigDirForClient()
	writeTestCertFile(t, cdstls.CAFilePath(), caPEM)
	writeTestCertFile(t, cdstls.ClientCertFilePath(), clientCertPEM)
	writeTestCertFile(t, cdstls.ClientKeyFilePath(), clientKeyPEM)

	cenv.SetConfigDirForAgent()
	writeTestCertFile(t, cdstls.CAFilePath(), caPEM)
	writeTestCertFile(t, cdstls.AgentServerCertFilePath(), serverCertPEM)
	writeTestCertFile(t, cdstls.AgentServerKeyFilePath(), serverKeyPEM)
}

func createTestCA(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CDS test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return key, cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func createTestLeafCert(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, usage x509.ExtKeyUsage) ([]byte, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func writeTestCertFile(t *testing.T, path string, content []byte) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, content, 0600))
}
