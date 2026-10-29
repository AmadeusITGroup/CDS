package core

import cg "github.com/amadeusitgroup/cds/internal/global"

var (
	spaceCommands = map[string]Cmd{
		"init": func() error { return nil },
	}
	projectCommands = map[string]Cmd{
		"init": func() error { return nil },
	}
)

type Cmd func() error

func New() Manager {
	// add initialization here
	return Manager{}
}

type Manager struct {
}

func (m Manager) Version() string {
	return cg.Version
}

func (m Manager) Space() map[string]Cmd {
	return spaceCommands
}

func (m Manager) Project() map[string]Cmd {
	return projectCommands
}
