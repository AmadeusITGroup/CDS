package command

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func TestExecuteOnHintsWhenTheAgentIsUnavailable(t *testing.T) {
	setupTestAgent(t)

	err := stubCallback(func(agentServices, context.Context) error {
		return status.Error(codes.Unavailable, "connection refused")
	}).executeOn(cg.KLocalhost, withoutAutostart())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not running")
	assert.Contains(t, err.Error(), "cds space host add "+cg.KLocalhost)
}

func TestExecuteOnHintsWhenTheClientCertificateExpired(t *testing.T) {
	setupTestAgent(t)

	err := stubCallback(func(agentServices, context.Context) error {
		return status.Error(codes.Unavailable, "tls: expired certificate")
	}).executeOn(cg.KLocalhost, withoutAutostart())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate for "+cg.KLocalhost+" expired")
	assert.NotContains(t, err.Error(), "is not running")
}

func TestExecuteOnShowsAnAgentRejectionInPlainWords(t *testing.T) {
	setupTestAgent(t)

	for _, code := range []codes.Code{codes.InvalidArgument, codes.FailedPrecondition, codes.NotFound, codes.Unimplemented, codes.Internal} {
		err := stubCallback(func(agentServices, context.Context) error {
			rejection := status.Error(code, "orchestration must be an object, got bool (false); remove it to turn it off.")
			return cerr.AppendError("Failed to receive deploy progress", rejection)
		}).executeOn(cg.KLocalhost, withoutAutostart())

		require.Error(t, err, code.String())
		assert.True(t, strings.HasPrefix(err.Error(), "orchestration must be an object, got bool (false); remove it to turn it off, at ["), err.Error())
		assert.NotContains(t, err.Error(), "rpc error", code.String())
		assert.NotContains(t, err.Error(), "Failed to receive", code.String())
		assert.NotContains(t, err.Error(), "..", code.String())
	}
}

func TestExecuteOnKeepsTheContextOfATransportFailure(t *testing.T) {
	setupTestAgent(t)

	for _, code := range []codes.Code{codes.DeadlineExceeded, codes.Canceled} {
		err := stubCallback(func(agentServices, context.Context) error {
			return cerr.AppendError("Failed to receive deploy progress", status.Error(code, "stream broken"))
		}).executeOn(cg.KLocalhost, withoutAutostart())

		require.Error(t, err, code.String())
		assert.True(t, strings.HasPrefix(err.Error(), "Failed to receive deploy progress"), err.Error())
	}

	err := stubCallback(func(agentServices, context.Context) error {
		return cerr.AppendError("Failed to receive deploy progress", status.Error(codes.Unavailable, "connection refused"))
	}).executeOn(cg.KLocalhost, withoutAutostart())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not running")
}
