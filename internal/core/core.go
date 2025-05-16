package core

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
	return "9.9.9"
}

func (m Manager) Space() map[string]Cmd {
	return spaceCommands
}

func (m Manager) Project() map[string]Cmd {
	return projectCommands
}
