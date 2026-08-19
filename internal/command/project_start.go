package command

import (
	"context"
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

	if err := withProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		return startProjectContainersOnAgent(ctx, services.container, projectName)
	}); err != nil {
		return err
	}

	clog.Info(fmt.Sprintf("Project '%s' started.", projectName))
	clog.Info(getTipSsh(projectName))
	return nil
}
