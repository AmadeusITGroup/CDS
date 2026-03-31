package command

import "github.com/spf13/cobra"

type project struct {
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (p *project) initSubCommands() {
	p.subCmds = append(p.subCmds,
		/*&projectCopy{},*/
		&projectInit{},
		/*&projectRun{},*/
		&projectUse{},
		/*&projectList{},
		&projectStart{},
		&projectStop{},
		&projectClear{},
		&projectDrain{},
		&projectRebuild{},
		&projectSync{},
		&projectDelete{},
		&projectShow{},
		&projectSsh{},
		&projectRsh{},
		&projectExpose{},
		&projectRename{},
		&projectShare{},
		&projectUnshare{},*/
	)
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/
var _ baseCmd = (*project)(nil)

func (p *project) command() *cobra.Command {
	if p.cmd == nil {
		p.cmd = &cobra.Command{
			Use:     "project",
			Aliases: []string{"p", "pr"},
			Short:   "Use projects",
			Long:    `Use .devcontainer configuration to manage dev environment as projects.`,
			Args:    cobra.NoArgs,
		}
		p.initSubCommands()
	}
	return p.cmd
}

func (p *project) subCommands() []baseCmd {
	return p.subCmds
}
