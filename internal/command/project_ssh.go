package command

import (
	"github.com/spf13/cobra"

	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

var _ baseCmd = (*projectSsh)(nil)

type projectSsh struct {
	defaultCmd
}

func (pssh *projectSsh) command() *cobra.Command {
	if pssh.cmd == nil {
		pssh.cmd = &cobra.Command{
			Use:   "ssh PROJECT-NAME",
			Short: "ssh into the currently deployed devcontainer",
			Long: `CDS will open an ssh session to the deployed container and attach the current terminal to it. ` +
				`It does not depend on the SSH configuration for the connection.`,
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              pssh.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
	}
	return pssh.cmd
}

func (pssh *projectSsh) subCommands() []baseCmd {
	return pssh.subCmds
}

func (pssh *projectSsh) execute(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)

	containerName, err := syncedProjectPrimaryContainer(projectName)
	if err != nil {
		return err
	}
	target, err := containerSSHTarget(projectName, containerName, cg.EmptyStr)
	if err != nil {
		return err
	}
	return shexec.AttachShellUsingKey(target)
}
