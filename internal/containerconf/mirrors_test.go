package containerconf

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrationConfigCollectsMirrors(t *testing.T) {
	config, err := ParseBytes(strings.NewReader(`{
		"image": "alpine:3.19",
		"orchestration": {
			"engine": "kind",
			"mirrors": [
				{ "registry": "docker.io", "endpoint": "https://mirror.example.com/docker" },
				{ "registry": "quay.io", "endpoint": "https://mirror.example.com/quay" }
			]
		}
	}`))
	require.NoError(t, err)

	oc := config.OrchestrationConfig()
	require.Len(t, oc.Mirrors, 2)
	assert.Equal(t, "docker.io", oc.Mirrors[0].Registry)
	assert.Equal(t, "https://mirror.example.com/docker", oc.Mirrors[0].Endpoint)
	assert.Equal(t, "quay.io", oc.Mirrors[1].Registry)
}

func TestOrchestrationConfigSkipsMalformedMirrors(t *testing.T) {
	config, err := ParseBytes(strings.NewReader(`{
		"orchestration": {
			"mirrors": [
				{ "registry": "docker.io" },
				{ "endpoint": "https://x" },
				{ "registry": "quay.io", "endpoint": "https://ok" }
			]
		}
	}`))
	require.NoError(t, err)

	oc := config.OrchestrationConfig()
	require.Len(t, oc.Mirrors, 1, "entries missing registry or endpoint are skipped")
	assert.Equal(t, "quay.io", oc.Mirrors[0].Registry)
}

func TestOrchestrationConfigNoMirrors(t *testing.T) {
	config, err := ParseBytes(strings.NewReader(`{"orchestration": {"engine": "kind"}}`))
	require.NoError(t, err)
	assert.Empty(t, config.OrchestrationConfig().Mirrors)
}

func TestOrchestrationConfigCollectsIngress(t *testing.T) {
	with, err := ParseBytes(strings.NewReader(`{"orchestration": {"engine": "kind", "ingress": true}}`))
	require.NoError(t, err)
	assert.True(t, with.OrchestrationConfig().Ingress)

	without, err := ParseBytes(strings.NewReader(`{"orchestration": {"engine": "kind"}}`))
	require.NoError(t, err)
	assert.False(t, without.OrchestrationConfig().Ingress)
}
