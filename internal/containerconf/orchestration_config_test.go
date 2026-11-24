package containerconf

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrationConfigAbsentSection(t *testing.T) {
	config, err := ParseBytes(strings.NewReader(`{"image":"alpine:3.19"}`))
	require.NoError(t, err)

	oc := config.OrchestrationConfig()

	assert.False(t, oc.Requested, "no orchestration section means not requested")
	assert.Empty(t, oc.Engine)
	assert.Empty(t, oc.ConfigFile)
	assert.False(t, oc.PersistentVolume)
	assert.False(t, oc.Registry.Requested)
	assert.False(t, oc.Registry.Secure)
	assert.False(t, config.RegistryPortIgnored())
}

func TestOrchestrationConfigKindWithSecureRegistry(t *testing.T) {
	config, err := ParseBytes(strings.NewReader(`{
		"image": "alpine:3.19",
		"orchestration": {
			"engine": "kind",
			"configFile": "/etc/cds/kind.yaml",
			"pvc": true,
			"registry": {
				"secure": true,
				"port": 6000
			}
		}
	}`))
	require.NoError(t, err)

	oc := config.OrchestrationConfig()

	assert.True(t, oc.Requested)
	assert.Equal(t, "kind", oc.Engine)
	assert.Equal(t, "/etc/cds/kind.yaml", oc.ConfigFile)
	assert.True(t, oc.PersistentVolume)
	assert.True(t, oc.Registry.Requested)
	assert.True(t, oc.Registry.Secure)
	assert.True(t, config.RegistryPortIgnored(), "the removed port key is detected so the client can warn (CDS-033)")
}

func TestOrchestrationConfigEngineWithoutRegistry(t *testing.T) {
	config, err := ParseBytes(strings.NewReader(`{
		"image": "alpine:3.19",
		"orchestration": {"engine": "kind"}
	}`))
	require.NoError(t, err)

	oc := config.OrchestrationConfig()

	assert.True(t, oc.Requested)
	assert.Equal(t, "kind", oc.Engine)
	assert.False(t, oc.Registry.Requested, "no registry sub-section means no registry")
	assert.False(t, oc.Registry.Secure)
}

func TestOrchestrationConfigInsecureRegistry(t *testing.T) {
	config, err := ParseBytes(strings.NewReader(`{
		"image": "alpine:3.19",
		"orchestration": {"engine": "kind", "registry": {}}
	}`))
	require.NoError(t, err)

	oc := config.OrchestrationConfig()

	assert.True(t, oc.Registry.Requested, "an empty registry object still requests a registry")
	assert.False(t, oc.Registry.Secure)
	assert.False(t, config.RegistryPortIgnored())
}
