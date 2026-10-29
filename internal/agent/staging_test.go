package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDirsUseAgentCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)
	cenv.SetConfigDirForAgent()
	t.Cleanup(cenv.SetConfigDirForClient)

	srv, err := newgrpcServer(NewConfig())
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(root, ".xcds-agent", "cache", "staging"), srv.stagingDir())
	assert.Equal(t, filepath.Join(root, ".xcds-agent", "cache", "deployments"), srv.deployments().dir)
	assert.NotEmpty(t, srv.hostname())
}

func TestStagedArtifactPath(t *testing.T) {
	for name, tc := range map[string]struct {
		project    string
		identifier string
		wantPath   string
		wantErr    bool
	}{
		"namespaced identifier": {
			project:    "project-a",
			identifier: "resource/dockerfile/Dockerfile",
			wantPath:   filepath.Join("/staging", "project-a", "resource", "dockerfile", "Dockerfile"),
		},
		"escaped logical name stays in its segment": {
			project:    "project-a",
			identifier: "resource/dockerfile/..%2FDockerfile",
			wantPath:   filepath.Join("/staging", "project-a", "resource", "dockerfile", "..%2FDockerfile"),
		},
		"invalid project":      {project: "../project-a", identifier: "resource/dockerfile/Dockerfile", wantErr: true},
		"empty project":        {project: "", identifier: "resource/dockerfile/Dockerfile", wantErr: true},
		"traversal identifier": {project: "project-a", identifier: "../../etc/passwd", wantErr: true},
		"absolute identifier":  {project: "project-a", identifier: "/etc/passwd", wantErr: true},
		"empty identifier":     {project: "project-a", identifier: "", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			_, path, err := stagedArtifactPath("/staging", tc.project, tc.identifier)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantPath, path)
		})
	}
}

func TestStagingResourceProvider(t *testing.T) {
	dir := t.TempDir()
	for project, content := range map[string]string{"project-a": "FROM a", "project-b": "FROM b"} {
		subDir := filepath.Join(dir, project, "resource", "dockerfile")
		require.NoError(t, os.MkdirAll(subDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(subDir, "Dockerfile"), []byte(content), 0600))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "project-b", "resource", "dockerfile", "dir"), 0700))
	provider := stagingResourceProvider{stagingDir: dir, projectName: "project-b"}
	const identifier = "resource/dockerfile/Dockerfile"

	data, err := provider.FetchFile(containerconf.ResourceTypeFile, identifier)
	require.NoError(t, err)
	assert.Equal(t, "FROM b", string(data), "provider is project scoped")
	assert.True(t, provider.FileExists(containerconf.ResourceTypeFile, identifier))

	_, err = provider.FetchFile(containerconf.ResourceTypeFile, "resource/dockerfile/missing")
	assert.Error(t, err)
	assert.False(t, provider.FileExists(containerconf.ResourceTypeFile, "resource/dockerfile/missing"))
	assert.False(t, provider.FileExists(containerconf.ResourceTypeFile, "resource/dockerfile/dir"), "directories are not files")

	_, err = provider.FetchFile(containerconf.ResourceTypeFeature, identifier)
	assert.Error(t, err)
	assert.False(t, provider.FileExists(containerconf.ResourceTypeFeature, identifier))
}
