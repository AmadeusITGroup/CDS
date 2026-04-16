package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
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

	// TODO: Leverage ContainerConf package once implemented — validate devcontainer configuration
	clog.Debug("Skipping devcontainer configuration validation — ContainerConf not yet implemented")

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to ssh into.\n" +
			getTipRun(projectName) + "\n" +
			getTipSsh(projectName))
	}

	// TODO: Agent Service interaction needed — SSH into container:
	// 1. Align container statuses via engine (alignContainerStatuses)
	// 2. Verify running containers exist
	// 3. Get first running container info (including SSH port mapping)
	// 4. Build target host with SSH key paths (db.GetHostKey, db.GetHostPubKey)
	// 5. Resolve remote user (db.ProjectContainerRemoteUser)
	// 6. Attach shell using key (shexec.AttachShellUsingKey)
	clog.Info(fmt.Sprintf("Project '%s' has containers configured. Agent service required for SSH access.", projectName))
	clog.Info(getTipStartAndSsh(projectName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'ssh' operation")
}
