package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

var _ baseCmd = (*projectRename)(nil)

type projectRename struct {
	defaultCmd
	projectName      string
	newContainerName string
}

func (pr *projectRename) command() *cobra.Command {
	if pr.cmd == nil {
		pr.cmd = &cobra.Command{
			Use:     "rename [PROJECT-NAME] NEW-NAME",
			Aliases: []string{"ren"},
			Short:   "Rename current container name",
			Long: `Rename currently used container name at runtime. 
This can be used to give a simpler and/or shorter names to your devcontainers.`,
			Args:              cobra.MaximumNArgs(2),
			PreRunE:           pr.check,
			RunE:              pr.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		pr.initSubCommands()
	}
	return pr.cmd
}

func (pr *projectRename) subCommands() []baseCmd {
	return pr.subCmds
}

func (pr *projectRename) check(cmd *cobra.Command, args []string) error {
	switch len(args) {
	case 0:
		return cerr.NewError("No arguments given. Rename needs at least the new container name")
	case 1:
		pr.projectName = db.GetCurrentProject()
		pr.newContainerName = args[0]
	case 2:
		pr.projectName = args[0]
		pr.newContainerName = args[1]
	}
	if err := validateCurrentProjectName(pr.projectName); err != nil {
		return err
	}
	if _, err := isValidProjectName(pr.newContainerName); err != nil {
		return err
	}

	containers := db.ProjectContainersName(pr.projectName)
	if len(containers) == 0 {
		return cerr.NewError("A project has to have containers in order for it to be renamed")
	}

	if len(pr.newContainerName) == 0 {
		return cerr.NewError("New container name cannot be empty")
	}
	return nil
}

func (pr *projectRename) initSubCommands() {
	pr.subCmds = []baseCmd{}
}

func (pr *projectRename) execute(cmd *cobra.Command, args []string) error {
	clog.Info(fmt.Sprintf("Using project '%s'.", pr.projectName))

	return withProjectAgent(pr.projectName, func(services agentServices, ctx context.Context) error {
		containers := db.ProjectContainersName(pr.projectName)
		if err := syncProjectContainersFromAgent(ctx, services.container, pr.projectName, containers, false); err != nil {
			return err
		}

		oldContainerName, remoteUser, err := projectPrimaryContainer(pr.projectName)
		if err != nil {
			return err
		}
		if oldContainerName == pr.newContainerName {
			clog.Warn(fmt.Sprintf("Container is already named %q. Skipping.", pr.newContainerName))
			return nil
		}

		if _, err := services.container.RenameContainer(ctx, &cdspb.RenameContainerRequest{
			ContainerName:    oldContainerName,
			NewContainerName: pr.newContainerName,
		}); err != nil {
			return cerr.AppendErrorFmt("Failed to rename container %s to %s", err, oldContainerName, pr.newContainerName)
		}
		if err := setProjectContainerFromAgentCurrentStatus(ctx, services.container, pr.projectName, pr.newContainerName, remoteUser); err != nil {
			return cerr.AppendError("Failed to synchronize renamed container state", err)
		}

		clog.Info(fmt.Sprintf("Container '%s' renamed to '%s'.", oldContainerName, pr.newContainerName))
		return nil
	})
}

// validateCurrentProjectName ensures the project exists in the configuration.
func validateCurrentProjectName(projectName string) error {
	if len(projectName) == 0 {
		return cerr.NewError("CDS is not set on a project yet, cannot deploy nothing!\n" +
			"Consider using 'cds project list' and 'cds project use <project-name>' before running this command!")
	}

	if !db.HasProject(projectName) {
		return cerr.NewError(fmt.Sprintf("Project '%s' is not defined in CDS configuration!\n"+
			"Consider using 'cds project list' and 'cds project use <project-name>' to switch project!", projectName))
	}

	return nil
}
