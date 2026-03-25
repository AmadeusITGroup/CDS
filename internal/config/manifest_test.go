package config

import (
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/amadeusitgroup/cds/internal/source"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDefaultManifest(t *testing.T) {
	m := defaultManifest()
	assert.Equal(t, "v1", m.APIVersion)
	assert.Len(t, m.Sources, 3)

	cli, ok := m.Sources[SourceKeyCLIAgentConfig]
	assert.True(t, ok)
	assert.Equal(t, "localfs", cli.Type)
	assert.NotEmpty(t, cli.Ref)

	prof, ok := m.Sources[SourceKeyProfile]
	assert.True(t, ok)
	assert.Equal(t, "localfs", prof.Type)

	db, ok := m.Sources[SourceKeyDB]
	assert.True(t, ok)
	assert.Equal(t, "localfs", db.Type)
}

func TestManifestYAMLRoundTrip(t *testing.T) {
	m := defaultManifest()
	data, err := yaml.Marshal(m)
	require.NoError(t, err)

	var m2 Manifest
	err = yaml.Unmarshal(data, &m2)
	require.NoError(t, err)

	assert.Equal(t, m.APIVersion, m2.APIVersion)
	assert.Len(t, m2.Sources, 3)
	for key, ref := range m.Sources {
		ref2, ok := m2.Sources[key]
		assert.True(t, ok, "missing key: %s", key)
		assert.Equal(t, ref.Type, ref2.Type)
		assert.Equal(t, ref.Ref, ref2.Ref)
	}
}

func TestLoadManifest_Bootstrap(t *testing.T) {
	cos.Fs = afero.NewMemMapFs()
	defer cos.SetRealFileSystem()

	// Set HOME so cenv resolves to a known path in the mock FS
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")

	m, err := LoadManifest()
	require.NoError(t, err)
	assert.Equal(t, "v1", m.APIVersion)
	assert.Len(t, m.Sources, 3)

	// File should have been created
	assert.True(t, cos.Exists("/tmp/testconfig/.xcds/"+kManifestFileName))
}

func TestLoadManifest_ExistingFile(t *testing.T) {
	cos.Fs = afero.NewMemMapFs()
	defer cos.SetRealFileSystem()

	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")

	content := `apiVersion: v1
sources:
  cliagentconfig:
    type: localfs
    ref: /custom/path/cli.yaml
  profile:
    type: scm
    ref: https://github.com/example/configs
  db:
    type: localfs
    ref: /custom/path/db.json
`
	manifestPath := "/tmp/testconfig/.xcds/" + kManifestFileName
	err := cos.Fs.MkdirAll("/tmp/testconfig/.xcds", 0755)
	require.NoError(t, err)
	err = cos.WriteFile(manifestPath, []byte(content), 0600)
	require.NoError(t, err)

	m, err := LoadManifest()
	require.NoError(t, err)
	assert.Equal(t, "v1", m.APIVersion)

	cli := m.Sources[SourceKeyCLIAgentConfig]
	assert.Equal(t, "localfs", cli.Type)
	assert.Equal(t, "/custom/path/cli.yaml", cli.Ref)

	prof := m.Sources[SourceKeyProfile]
	assert.Equal(t, "scm", prof.Type)
	assert.Equal(t, "https://github.com/example/configs", prof.Ref)
}

func TestManifest_Resolve_LocalFS(t *testing.T) {
	m := &Manifest{
		APIVersion: "v1",
		Sources: map[string]SourceRef{
			"test": {Type: "localfs", Ref: "/absolute/path/config.yaml"},
		},
	}
	src, err := m.Resolve("test")
	require.NoError(t, err)
	assert.NotNil(t, src)
	assert.Equal(t, source.LocalFS, src.Type())
	assert.Equal(t, "/absolute/path/config.yaml", src.Information())
}

func TestManifest_Resolve_LocalFS_RelativePath(t *testing.T) {
	m := &Manifest{
		APIVersion: "v1",
		Sources: map[string]SourceRef{
			"test": {Type: "localfs", Ref: "relative/path.yaml"},
		},
	}
	_, err := m.Resolve("test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute path")
}

func TestManifest_Resolve_UnknownKey(t *testing.T) {
	m := &Manifest{
		APIVersion: "v1",
		Sources:    map[string]SourceRef{},
	}
	_, err := m.Resolve("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown source key")
}

func TestManifest_Resolve_SCM(t *testing.T) {
	m := &Manifest{
		APIVersion: "v1",
		Sources: map[string]SourceRef{
			"test": {Type: "scm", Ref: "https://github.com/example/repo"},
		},
	}
	src, err := m.Resolve("test")
	require.NoError(t, err)
	assert.NotNil(t, src)
	assert.Equal(t, source.SCM, src.Type())
}

func TestManifest_Resolve_UnsupportedType(t *testing.T) {
	m := &Manifest{
		APIVersion: "v1",
		Sources: map[string]SourceRef{
			"test": {Type: "artifactory", Ref: "some-ref"},
		},
	}
	_, err := m.Resolve("test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not yet supported")
}

func TestEnsureSourceWithDefault_Creates(t *testing.T) {
	cos.Fs = afero.NewMemMapFs()
	defer cos.SetRealFileSystem()

	src, err := source.NewLocalSource("/tmp/testfile.yaml")
	require.NoError(t, err)

	err = EnsureSourceWithDefault(src, strings.NewReader("default content"), 0600)
	require.NoError(t, err)

	data, err := cos.ReadFile("/tmp/testfile.yaml")
	require.NoError(t, err)
	assert.Equal(t, "default content", string(data))
}

func TestEnsureSourceWithDefault_Exists(t *testing.T) {
	cos.Fs = afero.NewMemMapFs()
	defer cos.SetRealFileSystem()

	err := cos.WriteFile("/tmp/existing.yaml", []byte("existing"), 0600)
	require.NoError(t, err)

	src, err := source.NewLocalSource("/tmp/existing.yaml")
	require.NoError(t, err)

	err = EnsureSourceWithDefault(src, strings.NewReader("should not overwrite"), 0600)
	require.NoError(t, err)

	data, err := cos.ReadFile("/tmp/existing.yaml")
	require.NoError(t, err)
	assert.Equal(t, "existing", string(data))
}
