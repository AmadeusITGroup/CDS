package command

import (
	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/db"
)

type projectShow struct {
	all    bool
	output string
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (ps *projectShow) initFlags() {
	ps.cmd.Flags().BoolVarP(&ps.all, "all", "a", false, `Show information for all projects (project name given ignored)`)
	ps.cmd.Flags().StringVarP(&ps.output, "output", "o", "", `Dump the current CDS project information into the defined format: json or yaml.
To show the information about all existing projects, add the option --all.
e.g.: cds project show -o json --all`)
}

func (ps *projectShow) initSubCommands() {
	ps.subCmds = []baseCmd{}
}

func (ps *projectShow) execute(cmd *cobra.Command, args []string) error {
	if !ps.all {
		if err := validateProjectNameFromArgsOrContext(cmd, args); err != nil {
			return err
		}
		projectName := getProjectNameFromArgsOrContext(args)
		projectInfo := db.GetProjectInfo(projectName)
		formatProjectInfoInOutput(projectInfo)
		return nil
	}

	projects := db.ListProjects()
	if len(ps.output) > 0 {
		currentProject := db.GetCurrentProject()
		projectsInfo := make([]bo.ProjectInfo, 0, len(projects))
		for _, projectName := range projects {
			projectsInfo = append(projectsInfo, db.GetProjectInfo(projectName))
		}
		formatProjectListInOutput(projectsInfo, currentProject)
		return nil
	}

	for _, projectName := range projects {
		projectInfo := db.GetProjectInfo(projectName)
		printProjectInfo(projectInfo)
	}
	return nil
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*projectShow)(nil)

func (ps *projectShow) command() *cobra.Command {
	if ps.cmd == nil {
		ps.cmd = &cobra.Command{
			Use:           "show [PROJECT-NAME]",
			Short:         "Show CDS project configuration and state",
			Long:          `Show CDS project configuration and state`,
			RunE:          ps.execute,
			SilenceUsage:  true,
			SilenceErrors: true,
			PreRunE: func(cmd *cobra.Command, args []string) error {
				return setCDSCommandOutputFormat(ps.output)
			},
			ValidArgsFunction: completionProject,
		}
		ps.initFlags()
		ps.initSubCommands()
	}
	return ps.cmd
}

func (ps *projectShow) subCommands() []baseCmd {
	return ps.subCmds
}
