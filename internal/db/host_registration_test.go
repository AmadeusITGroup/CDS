package db

import (
	"encoding/json"
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/stretchr/testify/assert"
)

func TestSetAndGetHostRuntime(t *testing.T) {
	tests := []struct {
		name         string
		initialData  data
		hostToUpdate string
		info         bo.RuntimeInfo
		expectError  bool
	}{
		{
			name:         "Set runtime info for existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "localhost"}, {Name: "host2"}}}},
			hostToUpdate: "localhost",
			info:         bo.RuntimeInfo{Engine: "podman", Version: "5.1.1"},
			expectError:  false,
		},
		{
			name:         "Set runtime info for non-existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "localhost"}}}},
			hostToUpdate: "missing",
			info:         bo.RuntimeInfo{Engine: "podman", Version: "5.1.1"},
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()

			err := SetHostRuntime(tt.hostToUpdate, tt.info)
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.info, GetHostRuntime(tt.hostToUpdate))
		})
	}
}

func TestSetGetAndClearHostAgentOwnership(t *testing.T) {
	tearDown := setupTest(t, data{hosts: hosts{Hosts: []*host{{Name: "localhost"}}}})
	defer tearDown()

	ownership := bo.AgentOwnership{PID: 1234, Address: ":8087", Binary: "cds-api-agent", Manager: "process"}

	assert.NoError(t, SetHostAgentOwnership("localhost", ownership))
	assert.Equal(t, ownership, GetHostAgentOwnership("localhost"))

	assert.NoError(t, ClearHostAgentOwnership("localhost"))
	assert.Equal(t, bo.AgentOwnership{}, GetHostAgentOwnership("localhost"))
}

// TestHostRuntimeBackwardCompatible ensures a db.json written before the
// runtimeInfo and agentOwnership fields existed still loads, with the new fields
// zero-valued.
func TestHostRuntimeBackwardCompatible(t *testing.T) {
	legacy := `{"hosts":[{"name":"localhost","username":"dev"}]}`

	var d data
	err := json.Unmarshal([]byte(legacy), &d)
	assert.NoError(t, err)

	tearDown := setupTest(t, d)
	defer tearDown()

	assert.True(t, HasHost("localhost"))
	assert.Equal(t, bo.RuntimeInfo{}, GetHostRuntime("localhost"))
	assert.Equal(t, bo.AgentOwnership{}, GetHostAgentOwnership("localhost"))
}
