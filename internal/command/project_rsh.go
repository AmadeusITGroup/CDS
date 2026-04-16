package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
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

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to rsh into.\n" +
			getTipRun(projectName) + "\n" +
			getTipSsh(projectName))
	}

	// TODO: Agent Service interaction needed — remote shell into container:
	// 1. Align container statuses via engine (alignContainerStatuses)
	// 2. Verify running containers exist
	// 3. Get first running container info
	// 4. Build target host with SSH key paths
	// 5. Resolve session user (--user flag or db.ProjectContainerRemoteUser)
	// 6. Build remote execution engine with K_ACTION_EXE and K_EXEC_CMD_RSH
	// 7. Attach process using key (shexec.AttachProcessUsingKey)
	clog.Info(fmt.Sprintf("Project '%s' has containers configured. Agent service required for remote shell access.", projectName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'rsh' operation")
}
