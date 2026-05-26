package bootstrap

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/host"
	"github.com/amadeusitgroup/cds/internal/systemd"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
)

var (
	binaries = []binary{cfsslbin{n: "cfssl"}, cfssljsonbin{n: "cfssljson"}, cdsagentbin{n: "cdssrv"}}
)

func fire() error {
	clog.Debug("Starting agent on Linux")
	defer clog.Debug("Agent started on Linux")

	for _, bin := range binaries {
		if err := ensureBinary(bin); err != nil {
			return cerr.AppendErrorFmt("Failed to install binary %s", err, bin.name())
		}
	}

	if err := cdstls.BuildCerts(); err != nil {
		return cerr.AppendError("Failed to build certs", err)
	}

	port := agentPort()
	addr := fmt.Sprintf(":%d", port)

	agentPath, err := exec.LookPath("cdssrv")
	if err != nil {
		return cerr.AppendError("cdssrv not found on PATH", err)
	}

	sysd := systemd.New(
		systemd.WithTarget(host.New(host.WithName(cg.KLocalhost))),
		systemd.WithServicePort(port),
		systemd.WithServiceBinary(agentPath),
	)
	if sysd.In() {
		if err := sysd.StartService(); err != nil {
			clog.Warn("Systemd start failed, falling back to exec:", err)
		} else {
			return registerAgentInConfig(addr)
		}
	}

	if _, err := startAgent(); err != nil {
		return cerr.AppendError("Failed to start agent", err)
	}
	return registerAgentInConfig(addr)
}

func agentPort() int {
	if p := os.Getenv("CDS_AGENT_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil && port > 0 {
			return port
		}
	}
	return 8087
}

func registerAgentInConfig(server string) error {
	addr, parseErr := net.ResolveTCPAddr("tcp", server)
	if parseErr != nil {
		return cerr.AppendError("Failed to parse agent address", parseErr)
	}

	if err := config.AddAgentToConfig(config.NewAgent(
		config.WithTargetAddress(addr.String()),
		config.WithAgentTLS(
			config.NewTlssecret(
				config.WithCA(cdstls.CAFilePath),
				config.WithCert(cdstls.ClientCertFilePath),
				config.WithKey(cdstls.ClientKeyFilePath)),
		),
	)); err != nil {
		return cerr.AppendError("failed to add agent to CLI config", err)
	}
	return nil
}

func startAgent() (string, error) {
	cmd := exec.Command("cdssrv")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	clog.Debug("Start agent...")
	if err := cmd.Start(); err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to start agent", err)
	}

	go func() {
		clog.Debug("Waiting agent to start...")
		if err := cmd.Wait(); err != nil {
			clog.Error("Command finished with error:", err)
		}
	}()

	clog.Debug(fmt.Sprintf("Agent started with pid: %d", cmd.Process.Pid))
	return ":8087", nil
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
