package bootstrap

import "github.com/amadeusitgroup/cds/internal/clog"

func fire() error {
	clog.Debug("Starting agent on Windows")
	return fireLocal(execAgent)
}
