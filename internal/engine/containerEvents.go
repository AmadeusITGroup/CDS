package engine

import (
	"github.com/amadeusitgroup/cds/internal/shexec"
)

var _ shexec.ExecuteEvent = (*RunEvent)(nil)

type RunEvent struct {
	cmd              string
	rollback         string
	canRecover       bool
	continueProcess  bool
	eventInfo        string
	createsContainer bool
}

func (re *RunEvent) Cmd() string {
	return re.cmd
}

func (re *RunEvent) Description() string {
	return re.eventInfo
}

// CreatesContainer reports whether running this event brings the container into
// existence (the `run` step). Callers use it to know, on a later failure,
// whether a container they must clean up was created by this deployment.
func (re *RunEvent) CreatesContainer() bool {
	return re.createsContainer
}

func (re *RunEvent) Recover() string {
	return re.rollback
}

func (re *RunEvent) Recoverable() bool {
	return re.canRecover
}

func (re *RunEvent) CarryOn() bool {
	return re.continueProcess
}
