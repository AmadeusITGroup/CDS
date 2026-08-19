package agent

import (
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultArtifactStagingDirUsesAgentCache(t *testing.T) {
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")
	cenv.SetConfigDirForAgent()
	t.Cleanup(cenv.SetConfigDirForClient)

	assert.Equal(t, filepath.Join("/tmp/testconfig", ".xcds-agent", "cache", "staging"), defaultArtifactStagingDir())
}

func TestResolveProjectArtifact(t *testing.T) {
	identifier, err := containerconf.ResourceIdentifier(containerconf.KindDockerfile, "Dockerfile")
	require.NoError(t, err)
	artifact, err := resolveProjectArtifact("/tmp/staging", "project-a", identifier)
	require.NoError(t, err)

	assert.Equal(t, identifier, artifact.identifier)
	assert.Equal(t, filepath.Join("/tmp/staging", "project-a", "resource", "dockerfile", "Dockerfile"), artifact.path)
}

func TestResolveProjectArtifactEscapesLogicalName(t *testing.T) {
	identifier, err := containerconf.ResourceIdentifier(containerconf.KindDockerfile, "../Dockerfile")
	require.NoError(t, err)

	artifact, err := resolveProjectArtifact("/tmp/staging", "project-a", identifier)
	require.NoError(t, err)

	assert.Equal(t, identifier, artifact.identifier)
	assert.Equal(t, filepath.Join("/tmp/staging", "project-a", "resource", "dockerfile", "..%2FDockerfile"), artifact.path)
}

func TestResolveProjectArtifactRejectsInvalidProjectName(t *testing.T) {
	identifier, err := containerconf.ResourceIdentifier(containerconf.KindDockerfile, "Dockerfile")
	require.NoError(t, err)

	_, err = resolveProjectArtifact("/tmp/staging", "../project-a", identifier)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project_name")
}
