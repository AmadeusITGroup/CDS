package bootstrap

import (
	"os/exec"
	"syscall"
)

func fire() error {
	return fireLocalAgent("Linux")
}

func prepareAgentCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// func onRemoteLinux(hostName string) ([]byte, error) {

// 	// 1. Get Distro from remote host. If not linux -> return error

// 	// 2. Download
// 	// MVP: download binary from artifactory if not installed. On devboxes: it should be already installed!
// 	// start cds-api-agent using systemd.
// 	// day1: sudo dnf install cds-package (for none 1A users or users not using devbox)

// 	// 2. CA creation
// 	// Build CA certs
// 	// MVP: using cfssl binary to create CA certs, push to remote host using scp
// 	// day1: using code - tls package

// 	// 3. Start server
// 	// MVP: Using CLI
// 	// day1: fork process in case of laptop

// 	// 4. get server address
// 	return nil, nil
// }
