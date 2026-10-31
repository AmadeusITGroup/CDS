package engine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateNetworkWithArgsPlacesFlagsBeforeName proves the network create
// command carries the extra flags (e.g. --ipv6=false) before the network name,
// which is what EnsureKindNetwork relies on to force an IPv4-only kind network.
func TestCreateNetworkWithArgsPlacesFlagsBeforeName(t *testing.T) {
	ce := NewContainerEngine()
	SetEngineName(ce, "podman")
	ce.SetAction(K_ACTION_NETWORK)
	ce.SetNetworkCmd(K_NETWORK_CREATE)
	ce.SetNetworkName("kind")
	ce.SetNetworkCreateArgs("--ipv6=false")

	cmds, err := ce.BuildCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmd := cmds[0].Cmd()
	assert.Contains(t, cmd, "podman network create")
	assert.Contains(t, cmd, "--ipv6=false")
	// the flag must come before the trailing network name
	flagIdx := strings.Index(cmd, "--ipv6=false")
	nameIdx := strings.LastIndex(cmd, "kind")
	assert.Less(t, flagIdx, nameIdx, "flags must precede the network name")
}

// TestCreateNetworkWithoutArgsHasNoFlags proves the plain create path is
// unchanged (no stray empty args) when no flags are set.
func TestCreateNetworkWithoutArgsHasNoFlags(t *testing.T) {
	ce := NewContainerEngine()
	SetEngineName(ce, "podman")
	ce.SetAction(K_ACTION_NETWORK)
	ce.SetNetworkCmd(K_NETWORK_CREATE)
	ce.SetNetworkName("kind")

	cmds, err := ce.BuildCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	assert.Equal(t, "podman network create kind", strings.TrimSpace(cmds[0].Cmd()))
}
