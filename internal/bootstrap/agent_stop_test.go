package bootstrap

import (
	"errors"
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubAgentProcessOps(t *testing.T, name string) *[]int {
	t.Helper()
	originalProcessName := processName
	originalSignalProcess := signalProcess
	signaled := []int{}
	processName = func(int) (string, error) { return name, nil }
	signalProcess = func(pid int) error {
		signaled = append(signaled, pid)
		return nil
	}
	t.Cleanup(func() {
		processName = originalProcessName
		signalProcess = originalSignalProcess
	})
	return &signaled
}

func loadOwnedLocalAgentForStopTest(t *testing.T, ownership bo.AgentOwnership) {
	t.Helper()
	db.ResetForTest()
	t.Cleanup(db.ResetForTest)
	db.AddHost(cg.KLocalhost, "tester")
	require.NoError(t, db.SetHostAgentOwnership(cg.KLocalhost, ownership))
}

func TestStopAgentSignalsOwnedLocalProcess(t *testing.T) {
	loadOwnedLocalAgentForStopTest(t, bo.AgentOwnership{PID: 1234, Binary: "cds-api-agent", Manager: "process"})
	signaled := stubAgentProcessOps(t, "/usr/local/bin/cds-api-agent")

	require.NoError(t, StopAgent(cg.KLocalhost))

	assert.Equal(t, []int{1234}, *signaled)
}

func TestStopAgentSkipsProcessThatDoesNotMatchOwnership(t *testing.T) {
	loadOwnedLocalAgentForStopTest(t, bo.AgentOwnership{PID: 1234, Binary: "cds-api-agent", Manager: "process"})
	signaled := stubAgentProcessOps(t, "python")

	require.NoError(t, StopAgent(cg.KLocalhost))

	assert.Empty(t, *signaled)
}

func TestStopAgentIgnoresStaleOwnership(t *testing.T) {
	loadOwnedLocalAgentForStopTest(t, bo.AgentOwnership{PID: 1234, Binary: "cds-api-agent", Manager: "process"})
	originalProcessName := processName
	originalSignalProcess := signalProcess
	signaled := []int{}
	processName = func(int) (string, error) { return "", errors.New("missing process") }
	signalProcess = func(pid int) error {
		signaled = append(signaled, pid)
		return nil
	}
	t.Cleanup(func() {
		processName = originalProcessName
		signalProcess = originalSignalProcess
	})

	require.NoError(t, StopAgent(cg.KLocalhost))

	assert.Empty(t, signaled)
}

func TestMatchesOwnedAgentProcess(t *testing.T) {
	assert.True(t, matchesOwnedAgentProcess("/usr/local/bin/cds-api-agent", bo.AgentOwnership{Binary: "cds-api-agent"}))
	assert.True(t, matchesOwnedAgentProcess("cdssrv", bo.AgentOwnership{Binary: "/opt/cds/cdssrv"}))
	assert.False(t, matchesOwnedAgentProcess("python", bo.AgentOwnership{Binary: "cds-api-agent"}))
}

func TestIsProcessManagedAgent(t *testing.T) {
	assert.True(t, isProcessManagedAgent(bo.AgentOwnership{PID: 1234, Binary: "cds-api-agent", Manager: "process"}))
	assert.False(t, isProcessManagedAgent(bo.AgentOwnership{PID: 0, Binary: "cds-api-agent", Manager: "process"}))
	assert.False(t, isProcessManagedAgent(bo.AgentOwnership{PID: 1234, Binary: "cds-api-agent", Manager: "systemd"}))
}
