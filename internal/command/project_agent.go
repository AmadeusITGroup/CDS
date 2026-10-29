package command

import (
	"strings"

	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// projectAgentHost returns the host running the agent of the project.
func projectAgentHost(projectName string) string {
	if hostName := strings.TrimSpace(db.ProjectHostName(projectName)); hostName != cg.EmptyStr {
		return hostName
	}
	return cg.KLocalhost
}

// onProjectAgent calls the agent of the project host, see stubCallback.executeOn.
func onProjectAgent(projectName string, callback stubCallback, options ...func(*callOptions)) error {
	return callback.executeOn(projectAgentHost(projectName), options...)
}
