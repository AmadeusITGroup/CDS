package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDisplayAgentTargetShowsPortOnlyLocalhostWithHost(t *testing.T) {
	assert.Equal(t, "localhost:8087", displayAgentTarget(":8087"))
	assert.Equal(t, "agent.example:8087", displayAgentTarget("agent.example:8087"))
}
