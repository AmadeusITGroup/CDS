package command

import (
	"context"
	"net"
	"testing"

	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteOnReachesTheRegisteredAgent(t *testing.T) {
	setupTestAgent(t)

	version, err := agentVersionOn(cg.KLocalhost, withoutAutostart())

	require.NoError(t, err)
	assert.Equal(t, cg.Version, version)
}

func TestExecuteOnVerifiesTheAgentHostName(t *testing.T) {
	// the agent certificate is only issued for localhost
	setupTestAgent(t, cg.KLocalhost)
	address, err := config.AgentAddress(cg.KLocalhost)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(address)
	require.NoError(t, err)
	require.NoError(t, config.UpsertAgentForHostInConfig("127.0.0.1", config.NewAgent(
		config.WithTargetAddress(net.JoinHostPort("127.0.0.1", port)),
	)))

	_, err = agentVersionOn(cg.KLocalhost, withoutAutostart())
	require.NoError(t, err)
	_, err = agentVersionOn("127.0.0.1", withoutAutostart())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate")
}

func TestExecuteOnFailsForUnregisteredAgent(t *testing.T) {
	setupTestAgent(t)

	_, err := agentVersionOn("unknown.example.com", withoutAutostart())

	require.Error(t, err)
	assert.Contains(t, err.Error(), `No agent found with hostname "unknown.example.com"`)
}

func TestExecuteOnFailsOnInvalidAgentAddress(t *testing.T) {
	setupTestAgent(t)
	require.NoError(t, config.UpsertAgentForHostInConfig(cg.KLocalhost, config.NewAgent(
		config.WithTargetAddress("localhost:%zz"),
	)))

	called := false
	err := stubCallback(func(agentServices, context.Context) error {
		called = true
		return nil
	}).executeOn(cg.KLocalhost, withoutAutostart())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to create agent client")
	assert.False(t, called)
}
