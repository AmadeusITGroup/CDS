package config

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentAddressLoadsRegisteredAgentsWithoutInit(t *testing.T) {
	setupConfigTestFS(t)

	require.NoError(t, cos.Fs.MkdirAll("/tmp/testconfig/.xcds", 0755))
	require.NoError(t, cos.WriteFile("/tmp/testconfig/.xcds/cliconfig.yaml", []byte(`apiVersion: v1
agents:
  - targetServer: https://agent.example:8443
`), 0600))

	address, err := AgentAddress("agent.example")
	require.NoError(t, err)
	assert.Equal(t, "https://agent.example:8443", address)
}

func TestAddAgentToConfigPersistsAgentsWithoutInit(t *testing.T) {
	setupConfigTestFS(t)

	require.NoError(t, AddAgentToConfig(NewAgent(
		WithTargetAddress("https://agent.example:8443"),
		WithSSHTunnel(true),
		WithAgentTLS(NewTlssecret(
			WithCA("/tmp/ca.pem"),
			WithCert("/tmp/client.pem"),
			WithKey("/tmp/client-key.pem"),
		)),
	)))

	data, err := readCLIAgentData()
	require.NoError(t, err)
	require.Len(t, data.Agents, 1)
	assert.Equal(t, "https://agent.example:8443", data.Agents[0].TargetSrv)
	assert.True(t, data.Agents[0].SshTunnel)
	assert.Equal(t, "/tmp/ca.pem", data.Agents[0].Certs.CA)
	address, err := AgentAddress("agent.example")
	require.NoError(t, err)
	assert.Equal(t, "https://agent.example:8443", address)
}
