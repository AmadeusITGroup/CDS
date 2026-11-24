package agent

import (
	"errors"
	"testing"

	"github.com/amadeusitgroup/cds/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterDependentsEmptyWhenNoContainers(t *testing.T) {
	// no containers listed -> no dependents, no network inspection happens
	deps, err := clusterDependents(func() ([]string, error) { return nil, nil }, noInspect(t))
	require.NoError(t, err)
	assert.Empty(t, deps)
}

func TestClusterDependentsPropagatesListError(t *testing.T) {
	_, err := clusterDependents(func() ([]string, error) { return nil, errors.New("list boom") }, noInspect(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list boom")
}

func TestErrInUseMessage(t *testing.T) {
	err := errInUse("cluster", []string{"dev-a", "dev-b"})
	assert.Contains(t, err.Error(), "cluster is in use by 2 container(s)")
	assert.Contains(t, err.Error(), "dev-a")
	assert.Contains(t, err.Error(), "--force")
}

func noInspect(t *testing.T) func(string) ([]string, error) {
	return func(name string) ([]string, error) {
		t.Fatalf("unexpected inspect of %s", name)
		return nil, nil
	}
}

func TestClusterDependentsPropagatesInspectError(t *testing.T) {
	list := func() ([]string, error) { return []string{"dev-a", "dev-b"}, nil }
	networksOf := func(name string) ([]string, error) {
		if name == "dev-b" {
			return nil, errors.New("inspect boom")
		}
		return []string{"podman"}, nil
	}
	_, err := clusterDependents(list, networksOf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dev-b")
	assert.Contains(t, err.Error(), "inspect boom")
}

func TestClusterDependentsFindsKindNetwork(t *testing.T) {
	list := func() ([]string, error) { return []string{"dev-a", "dev-b"}, nil }
	networksOf := func(name string) ([]string, error) {
		if name == "dev-a" {
			return []string{"podman", engine.KindNetworkName}, nil
		}
		return []string{"podman"}, nil
	}
	deps, err := clusterDependents(list, networksOf)
	require.NoError(t, err)
	assert.Equal(t, []string{"dev-a"}, deps)
}

func TestGuardDeleteRefusesWhenUsageUnknown(t *testing.T) {
	failing := func() ([]string, error) { return nil, errors.New("list boom") }

	err := guardDelete("registry", false, failing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not deleted")
	assert.Contains(t, err.Error(), "--force")
	assert.Contains(t, err.Error(), "list boom")

	// force skips the check entirely
	require.NoError(t, guardDelete("registry", true, failing))
}

func TestGuardDeleteAllowsWhenNoDependents(t *testing.T) {
	require.NoError(t, guardDelete("cluster", false, func() ([]string, error) { return nil, nil }))
	err := guardDelete("cluster", false, func() ([]string, error) { return []string{"dev-a"}, nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "in use by 1 container(s)")
}
