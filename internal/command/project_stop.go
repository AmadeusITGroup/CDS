package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

var _ baseCmd = (*projectStop)(nil)

type projectStop struct {
	defaultCmd
}

func (ps *projectStop) command() *cobra.Command {
	if ps.cmd == nil {
		ps.cmd = &cobra.Command{
			Use:               "stop [PROJECT-NAME]",
			Short:             "Ensure all of the project's resources are stopped",
			Long:              `Ensure all of the project's resources are stopped`,
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

func (ps *projectStop) subCommands() []baseCmd {
	return ps.subCmds
}

func (ps *projectStop) initFlags() {
}

func (ps *projectStop) initSubCommands() {
	ps.subCmds = []baseCmd{}
}

func (ps *projectStop) runE(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)
	clog.Info(fmt.Sprintf("Using project '%s'.", projectName))

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		clog.Warn(fmt.Sprintf("Project '%s' is not active! Nothing to do.", projectName))
		return nil
	}

	if err := withProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		return stopProjectContainersOnAgent(ctx, services.container, projectName)
	}); err != nil {
		return err
	}

	clog.Info(fmt.Sprintf("Project '%s' stopped.", projectName))
	clog.Info(getTipStartAndSsh(projectName))
	return nil
}
