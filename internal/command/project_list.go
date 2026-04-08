package command

import (
	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/db"
)

type projectList struct {
	output string
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (pl *projectList) initFlags() {
	pl.cmd.Flags().StringVarP(&pl.output, "output", "o", "", "Dump the CDS output into the defined format.")
}

func (pl *projectList) initSubCommands() {
	pl.subCmds = []baseCmd{}
}

func (pl *projectList) execute(cmd *cobra.Command, args []string) error {
	projects := db.ListProjects()
	currentProject := db.GetCurrentProject()
	projectsInfo := make([]bo.ProjectInfo, 0, len(projects))

	for _, projectName := range projects {
		projectsInfo = append(projectsInfo, db.GetProjectInfo(projectName))
	}

	formatProjectListInOutput(projectsInfo, currentProject)
	return nil
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*projectList)(nil)

func (pl *projectList) command() *cobra.Command {
	if pl.cmd == nil {
		pl.cmd = &cobra.Command{
			Use:           "list",
			Aliases:       []string{"ls"},
			Short:         "List configured CDS projects",
			Long:          `List configured CDS projects`,
			Args:          cobra.NoArgs,
			RunE:          pl.execute,
			SilenceUsage:  true,
			SilenceErrors: true,
			PreRunE: func(cmd *cobra.Command, args []string) error {
				return setCDSCommandOutputFormat(pl.output)
			},
		}
		pl.initFlags()
		pl.initSubCommands()
	}
	return pl.cmd
}

func (pl *projectList) subCommands() []baseCmd {
	return pl.subCmds
}
