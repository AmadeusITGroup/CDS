package command

import (
	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/shexec"
)

var _ baseCmd = (*projectRsh)(nil)

type projectRsh struct {
	defaultCmd
	user string
}

func (prsh *projectRsh) command() *cobra.Command {
	if prsh.cmd == nil {
		prsh.cmd = &cobra.Command{
			Use:   "rsh PROJECT-NAME",
			Short: "open a remote shell into the currently deployed devcontainer or host",
			Long: `CDS will open a remote session to the deployed container and attach the current terminal to it. ` +
				`It does not depend on the SSH configuration for the connection.`,
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              prsh.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		prsh.initSubCommands()
		prsh.initFlags()
	}
	return prsh.cmd
}

func (prsh *projectRsh) initFlags() {
	prsh.cmd.Flags().StringVarP(&prsh.user, "user", "u", "", `User as which the remote session will be run`)
}

func (prsh *projectRsh) subCommands() []baseCmd {
	return prsh.subCmds
}

func (prsh *projectRsh) initSubCommands() {
}

func (prsh *projectRsh) execute(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)

	containerName, err := syncedProjectPrimaryContainer(projectName)
	if err != nil {
		return err
	}
	target, err := containerSSHTarget(projectName, containerName, prsh.user)
	if err != nil {
		return err
	}
	return shexec.AttachShellUsingKey(target)
}
