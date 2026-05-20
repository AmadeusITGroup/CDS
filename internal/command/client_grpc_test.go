package command

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAgentServerAddressUsesConfiguredLocalhostTargetServer(t *testing.T) {
	setupCommandConfigTestFS(t)

	require.NoError(t, config.CreateAgentInConfig(config.NewAgent(
		config.WithTargetAddress(":9091"),
	)))

	addr, err := getAgentServerAddress()
	require.NoError(t, err)
	assert.Equal(t, ":9091", addr)
}

func TestGetAgentServerAddressErrorsWhenLocalhostAgentIsMissing(t *testing.T) {
	setupCommandConfigTestFS(t)

	addr, err := getAgentServerAddress()
	require.Error(t, err)
	assert.Empty(t, addr)
	assert.Contains(t, err.Error(), `No agent found with hostname "localhost"`)
}

func TestGetAgentTargetUsesConfiguredRemoteHostAndCerts(t *testing.T) {
	setupCommandConfigTestFS(t)

	require.NoError(t, config.CreateAgentInConfig(config.NewAgent(
		config.WithTargetAddress("my-remote-host.example.com:8087"),
		config.WithAgentTLS(config.NewTlssecret(
			config.WithCA("/tmp/remote-ca.pem"),
			config.WithCert("/tmp/remote-client.pem"),
			config.WithKey("/tmp/remote-client-key.pem"),
		)),
	)))

	target, err := getAgentTarget("my-remote-host.example.com")
	require.NoError(t, err)
	assert.Equal(t, "my-remote-host.example.com", target.hostName)
	assert.Equal(t, "my-remote-host.example.com:8087", target.address)
	assert.Equal(t, "/tmp/remote-ca.pem", target.caFile)
	assert.Equal(t, "/tmp/remote-client.pem", target.certFile)
	assert.Equal(t, "/tmp/remote-client-key.pem", target.keyFile)
	assert.Equal(t, cg.KLocalhost, target.serverName)
}

func TestGetAgentTargetFallsBackToDefaultCertPaths(t *testing.T) {
	setupCommandConfigTestFS(t)

	require.NoError(t, config.CreateAgentInConfig(config.NewAgent(
		config.WithTargetAddress(":8087"),
	)))

	target, err := getAgentTarget(cg.KLocalhost)
	require.NoError(t, err)
	assert.Equal(t, cdstls.CAFilePath(), target.caFile)
	assert.Equal(t, cdstls.ClientCertFilePath(), target.certFile)
	assert.Equal(t, cdstls.ClientKeyFilePath(), target.keyFile)
	assert.Equal(t, cg.KLocalhost, target.serverName)
}

func TestAddressHostNormalizesAgentAddresses(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{name: "blank", address: "", want: cg.KLocalhost},
		{name: "port only", address: ":8087", want: cg.KLocalhost},
		{name: "host port", address: "remote.example.com:8087", want: "remote.example.com"},
		{name: "url", address: "https://remote.example.com:8087", want: "remote.example.com"},
		{name: "bare host", address: "remote.example.com", want: "remote.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, addressHost(tt.address))
		})
	}
}

func setupCommandConfigTestFS(t *testing.T) {
	t.Helper()

	cos.SetMockedFileSystem()
	cenv.SetConfigDirForClient()
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")
	t.Cleanup(func() {
		cenv.SetConfigDirForClient()
		cos.SetRealFileSystem()
	})
}
