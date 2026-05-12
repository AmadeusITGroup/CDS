package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

var _ baseCmd = (*projectSync)(nil)

type projectSync struct {
	defaultCmd
	all bool
}

func (ps *projectSync) command() *cobra.Command {
	if ps.cmd == nil {
		ps.cmd = &cobra.Command{
			Use:               "sync [PROJECT-NAME]",
			Short:             "Align containers statuses with current configuration",
			Long:              "Align containers statuses with current configuration",
			RunE:              ps.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		ps.initFlags()
		ps.initSubCommands()
	}
	return ps.cmd
}

func (ps *projectSync) subCommands() []baseCmd {
	return ps.subCmds
}

func (ps *projectSync) initFlags() {
	ps.cmd.Flags().BoolVarP(&ps.all, "all", "a", false, `sync container status for all projects (project name given ignored)`)
}

func (ps *projectSync) initSubCommands() {
	ps.subCmds = []baseCmd{}
}

func (ps *projectSync) execute(cmd *cobra.Command, args []string) error {
	if ps.all {
		return syncAllProjects()
	}

	if err := validateProjectNameFromArgsOrContext(cmd, args); err != nil {
		return err
	}
	projectName := getProjectNameFromArgsOrContext(args)
	clog.Info(fmt.Sprintf("Using project '%s' from current context.", projectName))

	if err := syncProject(projectName); err != nil {
		return err
	}

	return nil
}

func syncProject(projectName string) error {
	// TODO: Leverage ContainerConf package once implemented — validate devcontainer configuration
	clog.Debug("Skipping devcontainer configuration validation — ContainerConf not yet implemented")

	// TODO: Clean up SSH config entries for deleted containers once SSH config management is available.
	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		clog.Info("No containers found in configuration, nothing to sync.")
		return nil
	}

	return withProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		return syncProjectContainersFromAgent(ctx, services.container, projectName, containers, false)
	})
}

func syncAllProjects() error {
	clog.Info("Syncing all projects")
	projects := db.ListProjects()
	for _, projectName := range projects {
		clog.Info(fmt.Sprintf("Syncing container infos for project %s", projectName))
		if err := syncProject(projectName); err != nil {
			return err
		}
	}
	return nil
}
