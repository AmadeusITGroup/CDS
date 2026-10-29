package bootstrap

import "github.com/amadeusitgroup/cds/internal/clog"

func fire() error {
	clog.Debug("Starting agent on Darwin")
	return fireLocal(execAgent)
}
