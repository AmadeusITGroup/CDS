package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

var _ baseCmd = (*projectStart)(nil)

type projectStart struct {
	defaultCmd
}

func (ps *projectStart) command() *cobra.Command {
	if ps.cmd == nil {
		ps.cmd = &cobra.Command{
			Use:               "start [PROJECT-NAME]",
			Short:             "Ensure all of the project's resources are running",
			Long:              `Ensure all of the project's resources are running.`,
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              ps.runE,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		ps.initFlags()
		ps.initSubCommands()
	}
	return ps.cmd
}

func (ps *projectStart) subCommands() []baseCmd {
	return ps.subCmds
}

func (ps *projectStart) initFlags() {
}

func (ps *projectStart) initSubCommands() {
	ps.subCmds = []baseCmd{}
}

func (ps *projectStart) runE(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)
	clog.Info(fmt.Sprintf("Using project '%s'.", projectName))

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError(fmt.Sprintf("Project '%s' has no containers to start.\n"+
			getTipRun(projectName), projectName))
	}

	// TODO: Leverage ContainerConf package once implemented — validate devcontainer configuration
	clog.Debug("Skipping devcontainer configuration validation — ContainerConf not yet implemented")

	// TODO: Agent Service interaction needed — full start sequence:
	// 1. Align container statuses via engine (engine.NewContainerEngine + alignContainerStatuses)
	// 2. Check if already running (warn and return early)
	// 3. Start stopped containers (engine start)
	// 4. Re-align container statuses
	// 5. Verify all containers are running
	// 6. Start orchestration engine (KinD) if used
	// 7. Start registry if used
	clog.Info(fmt.Sprintf("Project '%s' is ready for starting. Agent service required to proceed.", projectName))
	clog.Info(getTipSsh(projectName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'start' operation")
}
