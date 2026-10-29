package shexec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLocalCmdWithOutput(t *testing.T) {
	stdout, err := RunLocalCmdWithOutput([]ExecuteEvent{&DefaultShEvent{ExeCmd: "echo hello"}})

	if err != nil && stdout != "hello" {
		t.Errorf("com.RunLocalCmdWithOutput: Failed to run hello cmd: stdout %s, err: %s", stdout, err)
	}

	stdout, err = RunLocalCmdWithOutput([]ExecuteEvent{&DefaultShEvent{ExeCmd: "echoerror"}})
	commandNotFound := strings.Contains(stdout, "echoerror: command not found") || strings.Contains(stdout, "'echoerror' is not recognized as an internal or external command")
	if err == nil || (!commandNotFound) {
		t.Errorf("com.RunLocalCmdWithOutput: Failed crash on wrong cmd: stdout %s, err: %s", stdout, err)
	}
}

func TestRunLocalCmdWithExitCode(t *testing.T) {
	for name, tc := range map[string]struct {
		cmd        string
		wantOutput string
		wantCode   int
	}{
		"success":                       {cmd: "echo hello", wantOutput: "hello\n", wantCode: 0},
		"non-zero exit is not an error": {cmd: "echo out; echo err >&2; exit 3", wantOutput: "out\nerr\n", wantCode: 3},
	} {
		t.Run(name, func(t *testing.T) {
			output, code, err := RunLocalCmdWithExitCode(&DefaultShEvent{ExeCmd: tc.cmd})

			require.NoError(t, err)
			assert.Equal(t, tc.wantOutput, output)
			assert.Equal(t, tc.wantCode, code)
		})
	}
}
