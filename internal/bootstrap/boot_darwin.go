package bootstrap

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
)

var (
	binaries = []binary{cfsslbin{n: "cfssl"}, cfssljsonbin{n: "cfssljson"}, cdsagentbin{n: "cds-api-agent"}}
)

func fire() (bo.AgentOwnership, error) {
	clog.Debug("Starting agent on Darwin")
	defer clog.Debug("Agent started on Darwin")

	for _, bin := range binaries {
		if err := ensureBinary(bin); err != nil {
			return bo.AgentOwnership{}, cerr.AppendErrorFmt("Failed to install binary %s", err, bin.name())
		}
	}

	if err := cdstls.BuildCerts(); err != nil {
		return bo.AgentOwnership{}, cerr.AppendError("Failed to build certs", err)
	}

	var (
		server    string
		ownership bo.AgentOwnership
		startErr  error
	)

	if server, ownership, startErr = startAgent(); startErr != nil {
		return bo.AgentOwnership{}, cerr.AppendError("Failed to start agent", startErr)
	}

	addr, parseErr := net.ResolveTCPAddr("tcp", server)
	if parseErr != nil {
		if stopErr := StopManagedAgent(ownership); stopErr != nil {
			clog.Warn("Failed to stop local agent after address parsing failure", stopErr)
		}
		return bo.AgentOwnership{}, cerr.AppendError("Failed to parse agent address", parseErr)
	}
	ownership.Address = addr.String()

	if err := config.AddAgentToConfig(config.NewAgent(
		config.WithTargetAddress(addr.String()),
		config.WithAgentTLS(
			config.NewTlssecret(
				config.WithCA(cdstls.CAFilePath),
				config.WithCert(cdstls.ClientCertFilePath),
				config.WithKey(cdstls.ClientKeyFilePath)),
		),
	)); err != nil {
		if stopErr := StopManagedAgent(ownership); stopErr != nil {
			clog.Warn("Failed to stop local agent after config registration failure", stopErr)
		}
		return bo.AgentOwnership{}, cerr.AppendError("failed to add agent to CLI config", err)
	}
	return ownership, nil
}

func startAgent() (string, bo.AgentOwnership, error) {
	cmd := exec.Command("cds-api-agent")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	clog.Debug("Start agent...")
	if err := cmd.Start(); err != nil {
		return cg.EmptyStr, bo.AgentOwnership{}, cerr.AppendError("Failed to start agent", err)
	}

	go func() {
		clog.Debug("Waiting agent to start...")
		if err := cmd.Wait(); err != nil {
			clog.Error("Command finished with error:", err)
		}
	}()

	clog.Debug(fmt.Sprintf("Agent started with pid: %d", cmd.Process.Pid))
	return ":8087", bo.AgentOwnership{PID: cmd.Process.Pid, Binary: "cds-api-agent", Manager: "process"}, nil
}
