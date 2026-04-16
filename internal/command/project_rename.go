package command

import (
	"fmt"

	"github.com/spf13/cobra"

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

	// TODO: Agent Service interaction needed — rename container:
	// 1. Get first running container info
	// 2. Execute engine rename operation (engine.K_ACTION_RENAME via shexec.RunCmds)
	// 3. Update container name in config (db equivalent of space.RenameContainer)
	// 4. Clear old SSH config entry (com.ClearSSHConfig)
	// 5. Add new SSH config entry if port 22 is exposed (configureSSHForContainer)
	clog.Info(fmt.Sprintf("Project '%s' rename to '%s' requires agent service to proceed.", pr.projectName, pr.newContainerName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'rename' operation")
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
