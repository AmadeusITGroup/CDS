package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
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

	// TODO: start orchestration engine (KinD) and registry when used
	if err := onProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		return changeProjectContainersState(ctx, services.container, projectName, bo.KContainerStatusRunning, func(containerName string) error {
			_, err := services.container.StartContainer(ctx, &cdspb.StartContainerRequest{ContainerName: containerName})
			return err
		})
	}); err != nil {
		return err
	}

	clog.Info(fmt.Sprintf("Project '%s' started.", projectName))
	clog.Info(getTipSsh(projectName))
	return nil
}
