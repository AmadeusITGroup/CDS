package containerconf

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubCollector struct {
	kind      string
	artifacts []RequiredArtifact
	err       error
}

func (s stubCollector) Kind() string { return s.kind }
func (s stubCollector) Collect(collectContext) ([]RequiredArtifact, error) {
	return s.artifacts, s.err
}

func dockerfileTestCollector() artifactCollector {
	return newConfigFileCollector(KindDockerfile, []string{KBuild, KBuildDockerfile}, newDockerfileReference)
}

func TestRunCollectorsAggregatesAndPreservesOrder(t *testing.T) {
	collectors := []artifactCollector{
		stubCollector{kind: "first", artifacts: []RequiredArtifact{{Identifier: "a"}}},
		stubCollector{kind: "second", artifacts: []RequiredArtifact{{Identifier: "b"}, {Identifier: "c"}}},
	}

	got, err := runCollectors(collectContext{}, collectors)
	require.NoError(t, err)
	assert.Equal(t, []RequiredArtifact{{Identifier: "a"}, {Identifier: "b"}, {Identifier: "c"}}, got)
}

func TestRunCollectorsWrapsErrorWithKind(t *testing.T) {
	sentinel := errors.New("boom")
	collectors := []artifactCollector{
		stubCollector{kind: "broken", err: sentinel},
	}

	_, err := runCollectors(collectContext{}, collectors)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "broken:")
}

func TestRunCollectorsRejectsDuplicateIdentifier(t *testing.T) {
	collectors := []artifactCollector{
		stubCollector{kind: "one", artifacts: []RequiredArtifact{{Identifier: "dup"}}},
		stubCollector{kind: "two", artifacts: []RequiredArtifact{{Identifier: "dup"}}},
	}

	_, err := runCollectors(collectContext{}, collectors)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"dup"`)
	assert.Contains(t, err.Error(), `"one"`)
	assert.Contains(t, err.Error(), `"two"`)
}

func TestDockerfileCollectorEmitsNothingWhenUnset(t *testing.T) {
	cfg := NewConfig()
	got, err := dockerfileTestCollector().Collect(collectContext{Config: cfg, ConfigDir: t.TempDir()})
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDockerfileCollectorRejectsNonStringValue(t *testing.T) {
	cfg := NewConfig()
	cfg.Set(KBuild+"."+KBuildDockerfile, 42)

	_, err := dockerfileTestCollector().Collect(collectContext{Config: cfg, ConfigDir: t.TempDir()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestConfigFileCollectorUsesRegisteredConfigPathAndReference(t *testing.T) {
	configDir := t.TempDir()
	cfg := NewConfig()
	cfg.Set("custom.file", "service.conf")
	collector := newConfigFileCollector("custom", []string{"custom", "file"}, func(sourcePath string) (localArtifactReference, error) {
		assert.Equal(t, "service.conf", sourcePath)
		return localArtifactReference{sourcePath: sourcePath, identifier: "resource/custom/service.conf"}, nil
	})

	got, err := collector.Collect(collectContext{Config: cfg, ConfigDir: configDir})
	require.NoError(t, err)
	assert.Equal(t, []RequiredArtifact{{
		Identifier: "resource/custom/service.conf",
		Source: SourceRef{
			Type: SourceTypeLocalFS,
			Ref:  filepath.Join(configDir, "service.conf"),
		},
	}}, got)
}

func TestCollectArtifactsWrapsCollectorError(t *testing.T) {
	isolateDefaultArtifactHome(t)
	cfg := NewConfig()
	cfg.Set(KBuild+"."+KBuildDockerfile, "   ")

	_, err := CollectArtifacts(cfg, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), KindDockerfile+":")
}
