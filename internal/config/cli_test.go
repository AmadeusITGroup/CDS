package config

import (
	"fmt"
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

func TestAgentAddressUsesConfiguredLocalhostTargetServer(t *testing.T) {
	tests := []struct {
		name         string
		targetServer string
	}{
		{
			name:         "port only",
			targetServer: ":9091",
		},
		{
			name:         "host and port",
			targetServer: "localhost:9092",
		},
		{
			name:         "url target",
			targetServer: "https://localhost:9093",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupConfigTestFS(t)

			require.NoError(t, cos.Fs.MkdirAll("/tmp/testconfig/.xcds", 0755))
			require.NoError(t, cos.WriteFile("/tmp/testconfig/.xcds/cliconfig.yaml", []byte(fmt.Sprintf(`apiVersion: v1
agents:
  - targetServer: %s
`, tt.targetServer)), 0600))

			address, err := AgentAddress("localhost")
			require.NoError(t, err)
			assert.Equal(t, tt.targetServer, address)
		})
	}
}

func TestAgentAddressDoesNotFallbackToHardcodedLocalhostPort(t *testing.T) {
	setupConfigTestFS(t)

	address, err := AgentAddress("localhost")
	assert.Empty(t, address)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `No agent found with hostname "localhost"`)
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

func TestAgentCRUDHelpersWithoutInit(t *testing.T) {
	setupConfigTestFS(t)

	require.NoError(t, CreateAgentInConfig(NewAgent(
		WithTargetAddress("https://agent.example:8443"),
		WithSSHTunnel(true),
		WithAgentTLS(NewTlssecret(
			WithCA("/tmp/ca.pem"),
			WithCert("/tmp/client.pem"),
			WithKey("/tmp/client-key.pem"),
		)),
	)))

	agents, err := RegisteredAgents()
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "https://agent.example:8443", agents[0].TargetSrv)

	agent, err := RegisteredAgent("https://agent.example:8443")
	require.NoError(t, err)
	assert.True(t, agent.SshTunnel)

	require.NoError(t, UpdateAgentInConfig("https://agent.example:8443", NewAgent(
		WithTargetAddress("https://agent-updated.example:9443"),
		WithSSHTunnel(false),
		WithAgentTLS(NewTlssecret()),
	)))

	agent, err = RegisteredAgent("https://agent-updated.example:9443")
	require.NoError(t, err)
	assert.Equal(t, "https://agent-updated.example:9443", agent.TargetSrv)
	assert.False(t, agent.SshTunnel)
	assert.Empty(t, agent.Certs.CA)

	require.NoError(t, DeleteAgentFromConfig("https://agent-updated.example:9443"))

	agents, err = RegisteredAgents()
	require.NoError(t, err)
	assert.Empty(t, agents)
}
