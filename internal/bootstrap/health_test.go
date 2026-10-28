package bootstrap

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"github.com/amadeusitgroup/cds/internal/tls/tlstest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type fakeAgentInfo struct {
	cdspb.UnimplementedAgentInfoServiceServer
	version string
}

func (f fakeAgentInfo) GetVersion(context.Context, *cdspb.GetVersionRequest) (*cdspb.GetVersionResponse, error) {
	return &cdspb.GetVersionResponse{Current: f.version}, nil
}

// startFakeAgent serves the agent info service over mutual TLS with the test
// certificates (valid for localhost and 127.0.0.1) and returns its address.
func startFakeAgent(t *testing.T, version string) string {
	t.Helper()
	t.Setenv("CDS_CONFIG_PATH", t.TempDir())
	certsDir := filepath.Dir(cdstls.CAFilePath())
	tlstest.WriteCerts(t, certsDir)

	tlsConfig, err := cdstls.SetupTLSConfig(cdstls.TLSConfig{
		CAFile:   cdstls.CAFilePath(),
		CertFile: filepath.Join(certsDir, "agent-srv.pem"),
		KeyFile:  filepath.Join(certsDir, "agent-srv-key.pem"),
		Server:   true,
	})
	require.NoError(t, err)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConfig)))
	cdspb.RegisterAgentInfoServiceServer(server, fakeAgentInfo{version: version})
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)
	return lis.Addr().String()
}

func TestIsAgentRunning(t *testing.T) {
	t.Run("not registered", func(t *testing.T) {
		startFakeAgent(t, cg.Version)
		running, err := isAgentRunning("127.0.0.1")
		require.NoError(t, err)
		assert.False(t, running)
	})

	t.Run("healthy agent", func(t *testing.T) {
		address := startFakeAgent(t, cg.Version)
		require.NoError(t, registerAgent("127.0.0.1", address))

		running, err := isAgentRunning("127.0.0.1")
		require.NoError(t, err)
		assert.True(t, running)
		require.NoError(t, waitForHealthyAgent(address, "127.0.0.1"))
	})

	t.Run("certificate not issued for the host", func(t *testing.T) {
		address := startFakeAgent(t, cg.Version)
		require.NoError(t, config.UpsertAgentForHostInConfig("other.example.com", config.NewAgent(config.WithTargetAddress(address))))

		running, err := isAgentRunning("other.example.com")
		require.NoError(t, err)
		assert.False(t, running, "the agent certificate must be valid for the host")
	})

	t.Run("remote agent with another version is updated", func(t *testing.T) {
		address := startFakeAgent(t, "other-version")
		require.NoError(t, registerAgent("127.0.0.1", address))

		running, err := isAgentRunning("127.0.0.1")
		require.NoError(t, err)
		assert.False(t, running)
	})

	t.Run("local agent with another version is kept", func(t *testing.T) {
		address := startFakeAgent(t, "other-version")
		_, port, err := net.SplitHostPort(address)
		require.NoError(t, err)
		require.NoError(t, registerAgent(cg.KLocalhost, net.JoinHostPort(cg.KLocalhost, port)))

		running, err := isAgentRunning(cg.KLocalhost)
		require.NoError(t, err)
		assert.True(t, running)
	})

	t.Run("registered agent not listening", func(t *testing.T) {
		startFakeAgent(t, cg.Version)
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		address := lis.Addr().String()
		require.NoError(t, lis.Close())
		require.NoError(t, registerAgent("127.0.0.1", address))

		running, err := isAgentRunning("127.0.0.1")
		require.NoError(t, err)
		assert.False(t, running)
	})
}

func TestWaitForHealthyAgentUsesProvisioningIdentityNotStaleRegistration(t *testing.T) {
	// P2: readiness after provisioning must use the provisioning identity, so a
	// stale/obsolete registration does not prevent reaching the fresh agent.
	address := startFakeAgent(t, cg.Version)
	// an obsolete registration pointing at non-existent credentials
	require.NoError(t, config.UpsertAgentForHostInConfig("127.0.0.1", config.NewAgent(
		config.WithTargetAddress(address),
		config.WithAgentTLS(config.NewTlssecret(
			config.WithCA("/nonexistent/ca.pem"),
			config.WithCert("/nonexistent/client.pem"),
			config.WithKey("/nonexistent/client-key.pem"),
		)),
	)))

	// readiness still succeeds because it dials with the provisioning credentials
	require.NoError(t, waitForHealthyAgent(address, "127.0.0.1"))
}

func TestRegisteredAgentTLSUsesRegistrationThenDefaults(t *testing.T) {
	t.Setenv("CDS_CONFIG_PATH", t.TempDir())

	// not registered -> default client provisioning certificates
	defaults := registeredAgentTLS("unknown.example.com")
	assert.Equal(t, cdstls.CAFilePath(), defaults.CAFile)
	assert.Equal(t, cdstls.ClientCertFilePath(), defaults.CertFile)
	assert.Equal(t, cdstls.ClientKeyFilePath(), defaults.KeyFile)

	// registered with its own TLS material -> that material is used
	require.NoError(t, config.UpsertAgentForHostInConfig("host.example.com", config.NewAgent(
		config.WithTargetAddress("host.example.com:8087"),
		config.WithAgentTLS(config.NewTlssecret(
			config.WithCA("/custom/ca.pem"),
			config.WithCert("/custom/client.pem"),
			config.WithKey("/custom/client-key.pem"),
		)),
	)))
	registered := registeredAgentTLS("host.example.com")
	assert.Equal(t, "/custom/ca.pem", registered.CAFile)
	assert.Equal(t, "/custom/client.pem", registered.CertFile)
	assert.Equal(t, "/custom/client-key.pem", registered.KeyFile)

	// registered without TLS material -> fall back to defaults
	require.NoError(t, config.UpsertAgentForHostInConfig("plain.example.com", config.NewAgent(
		config.WithTargetAddress("plain.example.com:8087"),
	)))
	fallback := registeredAgentTLS("plain.example.com")
	assert.Equal(t, cdstls.CAFilePath(), fallback.CAFile)
}
