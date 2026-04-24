package containerconf

import (
	"errors"
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
func (s stubCollector) Collect(CollectContext) ([]RequiredArtifact, error) {
	return s.artifacts, s.err
}

func TestRunCollectorsAggregatesAndPreservesOrder(t *testing.T) {
	collectors := []ArtifactCollector{
		stubCollector{kind: "first", artifacts: []RequiredArtifact{{Identifier: "a"}}},
		stubCollector{kind: "second", artifacts: []RequiredArtifact{{Identifier: "b"}, {Identifier: "c"}}},
	}

	got, err := runCollectors(CollectContext{}, collectors)
	require.NoError(t, err)
	assert.Equal(t, []RequiredArtifact{{Identifier: "a"}, {Identifier: "b"}, {Identifier: "c"}}, got)
}

func TestRunCollectorsWrapsErrorWithKind(t *testing.T) {
	sentinel := errors.New("boom")
	collectors := []ArtifactCollector{
		stubCollector{kind: "broken", err: sentinel},
	}

	_, err := runCollectors(CollectContext{}, collectors)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.Contains(t, err.Error(), "broken:")
}

func TestRunCollectorsRejectsDuplicateIdentifier(t *testing.T) {
	collectors := []ArtifactCollector{
		stubCollector{kind: "one", artifacts: []RequiredArtifact{{Identifier: "dup"}}},
		stubCollector{kind: "two", artifacts: []RequiredArtifact{{Identifier: "dup"}}},
	}

	_, err := runCollectors(CollectContext{}, collectors)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"dup"`)
	assert.Contains(t, err.Error(), `"one"`)
	assert.Contains(t, err.Error(), `"two"`)
}

func TestDockerfileCollectorEmitsNothingWhenUnset(t *testing.T) {
	cfg := NewConfig()
	got, err := dockerfileCollector{}.Collect(CollectContext{Config: cfg, ConfigDir: t.TempDir()})
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDockerfileCollectorRejectsNonStringValue(t *testing.T) {
	cfg := NewConfig()
	cfg.Set(KBuild+"."+KBuildDockerfile, 42)

	_, err := dockerfileCollector{}.Collect(CollectContext{Config: cfg, ConfigDir: t.TempDir()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestCollectArtifactsWrapsCollectorError(t *testing.T) {
	cfg := NewConfig()
	cfg.Set(KBuild+"."+KBuildDockerfile, "   ")

	_, err := CollectArtifacts(cfg, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), KindDockerfile+":")
}
