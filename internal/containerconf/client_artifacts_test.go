package containerconf

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectArtifactsReturnsDockerfileSourceAndIdentifier(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), ".devcontainer")
	config := NewConfig()
	config.Set(KBuild+"."+KBuildDockerfile, "Dockerfile")

	artifacts, err := CollectArtifacts(config, configDir)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	identifier, err := DockerfileIdentifier(config)
	require.NoError(t, err)

	assert.Equal(t, RequiredArtifact{
		Identifier: identifier,
		Source: SourceRef{
			Type: SourceTypeLocalFS,
			Ref:  filepath.Join(configDir, "Dockerfile"),
		},
	}, artifacts[0])
}

func TestCollectArtifactsResolvesPathOutsideConfigDir(t *testing.T) {
	projectDir := t.TempDir()
	configDir := filepath.Join(projectDir, ".devcontainer")
	config := NewConfig()
	config.Set(KBuild+"."+KBuildDockerfile, "../Dockerfile")

	artifacts, err := CollectArtifacts(config, configDir)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	identifier, err := DockerfileIdentifier(config)
	require.NoError(t, err)

	assert.Equal(t, identifier, artifacts[0].Identifier)
	assert.Equal(t, "resource/dockerfile/Dockerfile", identifier)
	assert.Equal(t, SourceRef{
		Type: SourceTypeLocalFS,
		Ref:  filepath.Join(projectDir, "Dockerfile"),
	}, artifacts[0].Source)
}

func TestCollectArtifactsReturnsEmptyWithoutConfigDerivedFiles(t *testing.T) {
	config := NewConfig()
	config.Set(KImage, "dockerhub.rnd.fix.me/dummy/image:0.1.0")

	artifacts, err := CollectArtifacts(config, t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, artifacts)
}

func TestCollectArtifactsRejectsRelativePathWithoutConfigDir(t *testing.T) {
	config := NewConfig()
	config.Set(KBuild+"."+KBuildDockerfile, "Dockerfile")

	_, err := CollectArtifacts(config, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config directory is required")
}
