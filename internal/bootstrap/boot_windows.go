package bootstrap

import "os/exec"

func fire() error {
	return fireLocalAgent("Windows")
}

func prepareAgentCommand(cmd *exec.Cmd) {
}
