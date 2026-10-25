package bootstrap

import (
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/host"
	"github.com/amadeusitgroup/cds/internal/systemd"
)

func fire() error {
	clog.Debug("Starting agent on Linux")
	return fireLocal(startLinuxAgent)
}

// startLinuxAgent runs the agent as a systemd user service when possible,
// otherwise as a detached process.
func startLinuxAgent(binary string, port int) error {
	sysd := systemd.New(
		systemd.WithTarget(host.New(host.WithName(cg.KLocalhost))),
		systemd.WithServicePort(port),
		systemd.WithServiceBinary(binary),
	)
	if sysd.In() {
		err := sysd.StartService()
		if err == nil {
			return nil
		}
		clog.Warn("Systemd start failed, falling back to exec:", err)
	}
	return execAgent(binary, port)
}
