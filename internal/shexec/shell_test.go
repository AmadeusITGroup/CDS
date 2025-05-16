package shexec

import (
	"strings"
	"testing"
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
