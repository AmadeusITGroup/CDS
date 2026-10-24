package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/cos"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"github.com/amadeusitgroup/cds/internal/tls/tlstest"
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

func setupCommandConfigTestFS(t *testing.T) {
	t.Helper()

	cos.SetMockedFileSystem()
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")
	t.Cleanup(func() {
		cos.SetRealFileSystem()
	})
}

func TestStubCallbackExecuteFailsOnInvalidAgentAddress(t *testing.T) {
	setupCommandConfigTestFS(t)
	// certificates are read from the real filesystem, the CLI config from the mocked one
	t.Setenv("CDS_CONFIG_PATH", t.TempDir())
	tlstest.WriteCerts(t, filepath.Dir(cdstls.CAFilePath()))
	require.NoError(t, config.CreateAgentInConfig(config.NewAgent(
		config.WithTargetAddress("localhost:%zz"),
	)))

	called := false
	err := stubCallback(func(agentServices, context.Context) error {
		called = true
		return nil
	}).execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to create agent client")
	assert.False(t, called)
}
