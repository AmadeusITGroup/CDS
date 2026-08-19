package bootstrap

import (
	"os/exec"
	"syscall"
)

func fire() error {
	return fireLocalAgent("Darwin")
}

func prepareAgentCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
