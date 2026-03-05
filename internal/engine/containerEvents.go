package engine

import (
	"github.com/amadeusitgroup/cds/internal/shexec"
)

var _ shexec.ExecuteEvent = (*RunEvent)(nil)

type RunEvent struct {
	cmd             string
	rollback        string
	canRecover      bool
	continueProcess bool
	eventInfo       string
}

func (re *RunEvent) Cmd() string {
	return re.cmd
}

func (re *RunEvent) Description() string {
	return re.eventInfo
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
