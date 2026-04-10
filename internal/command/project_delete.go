package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

type projectDelete struct {
	forceDelete bool
	deleteAll   bool
	defaultCmd
}

type projectDeletor func(string) error

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (pDlt *projectDelete) initFlags() {
	pDlt.cmd.Flags().BoolVarP(&pDlt.forceDelete, "force", "f", false, `Delete the project in CDS configuration even if remote cleanup fails`)
	pDlt.cmd.Flags().BoolVarP(&pDlt.deleteAll, "all", "a", false, `Delete all the projects`)
}

func (pDlt *projectDelete) initSubCommands() {
	pDlt.subCmds = []baseCmd{}
}

func (pDlt *projectDelete) execute(cmd *cobra.Command, args []string) error {
	currentContextProject := db.GetCurrentProject()
	deletor := pDlt.getDeletor()

	toDeleteProjectName := getProjectNameFromArgsOrContext(args)
	if err := deletor(toDeleteProjectName); err != nil {
		return err
	}

	projectNames := db.ListProjects()
	if len(projectNames) > 0 && toDeleteProjectName == currentContextProject {
		if err := db.SetProject(projectNames[0]); err != nil {
			return cerr.AppendError("Failed to switch to remaining project", err)
		}
		clog.Info(fmt.Sprintf("Project selection was empty -> selecting project '%s'.", projectNames[0]))
		return nil
	}

	if err := db.FlushContext(); err != nil {
		return cerr.AppendError("Failed to unselect last project", err)
	}

	clog.Info("Last project has been deleted. Project selection is now empty")
	return nil
}

func (pDlt *projectDelete) getDeletor() projectDeletor {
	if pDlt.deleteAll {
		if pDlt.forceDelete {
			return handleForceDeleteAll
		}
		return handleDeleteAll
	}
	if pDlt.forceDelete {
		return handleForceDelete
	}
	return deleteProjectFromConfig
}

func handleDeleteAll(_ string) error {
	return handleDeleteAllOrForceAll(deleteProjectFromConfig, "Error report after project delete all:")
}

func handleForceDeleteAll(_ string) error {
	return handleDeleteAllOrForceAll(handleForceDelete, "Error report after force project delete all:")
}

func handleDeleteAllOrForceAll(deleteOption func(string) error, errorMessage string) error {
	var projectDeleteErrors []error
	projectNames := db.ListProjects()
	for _, name := range projectNames {
		if deleteErr := deleteOption(name); deleteErr != nil {
			projectDeleteErrors = append(projectDeleteErrors, deleteErr)
		}
	}

	if len(projectDeleteErrors) > 0 {
		return cerr.AppendMultipleErrors(errorMessage, projectDeleteErrors)
	}

	clog.Info("All projects have been deleted.")
	return nil
}

func handleForceDelete(projectName string) error {
	if errorDel := deleteProjectFromConfig(projectName); errorDel != nil {
		clog.Warn("Failed to delete project normally, forcing config deletion")
		if _, deleteErr := db.DeleteProject(projectName); deleteErr != nil {
			clog.Warn("Failed to delete project in config file", deleteErr)
			return deleteErr
		}
		clog.Warn("Project deleted in CDS configuration. This action could lead to inconsistencies between CDS local state and the impacted target(s).")
	}
	return nil
}

func deleteProjectFromConfig(projectName string) error {
	clog.Info(fmt.Sprintf("Using project '%s'.", projectName))

	if _, err := db.DeleteProject(projectName); err != nil {
		clog.Warn("Failed to delete project in config file", err)
		return err
	}

	clog.Info(fmt.Sprintf("Project '%s' deleted", projectName))
	return nil
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*projectDelete)(nil)

func (pDlt *projectDelete) command() *cobra.Command {
	if pDlt.cmd == nil {
		pDlt.cmd = &cobra.Command{
			Use:               "delete",
			Aliases:           []string{"del", "rm", "remove"},
			Short:             "Removing any resources deployed and erase project's configuration",
			Long:              `Delete will completely wipe a project from CDS, including deployed resources & local configuration.`,
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              pDlt.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		pDlt.initFlags()
		pDlt.initSubCommands()
	}
	return pDlt.cmd
}

func (pDlt *projectDelete) subCommands() []baseCmd {
	return pDlt.subCmds
}
