package agent

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterDependentsEmptyWhenNoContainers(t *testing.T) {
	// no containers listed -> no dependents, no network inspection happens
	deps, err := clusterDependents(func() ([]string, error) { return nil, nil })
	require.NoError(t, err)
	assert.Empty(t, deps)
}

func TestClusterDependentsPropagatesListError(t *testing.T) {
	_, err := clusterDependents(func() ([]string, error) { return nil, errors.New("list boom") })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list boom")
}

func TestErrInUseMessage(t *testing.T) {
	err := errInUse("cluster", []string{"dev-a", "dev-b"})
	assert.Contains(t, err.Error(), "cluster is in use by 2 container(s)")
	assert.Contains(t, err.Error(), "dev-a")
	assert.Contains(t, err.Error(), "--force")
}
