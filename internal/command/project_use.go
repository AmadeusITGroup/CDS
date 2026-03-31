package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

type projectUse struct {
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (pu *projectUse) initSubCommands() {
}

func (pu *projectUse) execute(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)

	if err := db.SetProject(projectName); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to set project (%s) for container project", projectName), err)
	}

	clog.Info(fmt.Sprintf("Project switched to '%s'", projectName))

	return nil
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*projectUse)(nil)

func (pu *projectUse) command() *cobra.Command {
	if pu.cmd == nil {
		pu.cmd = &cobra.Command{
			Use:   "use PROJECT-NAME",
			Short: "Change current project",
			Long: `Make cds configuration point to the specified project so that ` +
				`subsequent commands implicitly use this configuration.`,
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              pu.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		pu.initSubCommands()
	}
	return pu.cmd
}

func (pu *projectUse) subCommands() []baseCmd {
	return pu.subCmds
}
