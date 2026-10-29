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
// otherwise as a detached process. Like the remote path, it restarts the
// service when the units changed or the service was already running: bootstrap
// only runs when the local agent is unhealthy, so an already-active service is
// stale (e.g. still serving a certificate EnsureCerts has just reissued) and
// must be restarted to reload it.
func startLinuxAgent(binary string, port int) error {
	sysd := systemd.New(
		systemd.WithTarget(host.New(host.WithName(cg.KLocalhost))),
		systemd.WithServicePort(port),
		systemd.WithServiceBinary(binary),
	)
	if sysd.In() {
		wasUp := sysd.IsServiceUp()
		changed, err := sysd.StartService()
		if err == nil {
			if changed || wasUp {
				if err := sysd.RestartService(); err != nil {
					clog.Warn("Systemd restart failed, falling back to exec:", err)
					return execAgent(binary, port)
				}
			}
			return nil
		}
		clog.Warn("Systemd start failed, falling back to exec:", err)
	}
	return execAgent(binary, port)
}
