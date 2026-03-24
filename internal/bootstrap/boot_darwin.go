package bootstrap

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
)

var (
	binaries = []binary{cfsslbin{n: "cfssl"}, cfssljsonbin{n: "cfssljson"}, cdsagentbin{n: "cds-api-agent"}}
)

func fire() error {
	clog.Debug("Starting agent on Darwin")
	defer clog.Debug("Agent started on Darwin")

	for _, bin := range binaries {
		if err := ensureBinary(bin); err != nil {
			return cerr.AppendErrorFmt("Failed to install binary %s", err, bin.name())
		}
	}

	if err := cdstls.BuildCerts(); err != nil {
		return cerr.AppendError("Failed to build certs", err)
	}

	var (
		server   string
		startErr error
	)

	if server, startErr = startAgent(); startErr != nil {
		return cerr.AppendError("Failed to start agent", startErr)
	}

	addr, parseErr := net.ResolveTCPAddr("tcp", server)
	if parseErr != nil {
		return cerr.AppendError("Failed to parse agent address", parseErr)
	}

	if err := config.AddClientToConfig(config.NewClient(
		config.WithTargetAddress(addr.String()),
		config.WithClientTLS(
			config.NewTlssecret(
				config.WithCA(cdstls.CAFilePath),
				config.WithCert(cdstls.ClientCertFilePath),
				config.WithKey(cdstls.ClientKeyFilePath)),
		),
	)); err != nil {
		return cerr.AppendError("failed to add client to CLI config", err)
	}
	return nil
}

func startAgent() (string, error) {
	cmd := exec.Command("cds-api-agent")
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
