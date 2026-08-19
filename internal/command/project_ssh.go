package command

import (
	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/db"
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

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to ssh into.\n" +
			getTipRun(projectName) + "\n" +
			getTipSsh(projectName))
	}

	if err := syncProjectContainers(projectName); err != nil {
		return err
	}

	containers = db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to ssh into.\n" +
			getTipRun(projectName) + "\n" +
			getTipSsh(projectName))
	}
	target, _, err := projectContainerSSHTarget(projectName)
	if err != nil {
		return err
	}
	return shexec.AttachShellUsingKey(target)
}
