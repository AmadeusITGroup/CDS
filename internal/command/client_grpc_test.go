package command

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/cos"
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
