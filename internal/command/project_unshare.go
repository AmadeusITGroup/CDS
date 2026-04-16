package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

var _ baseCmd = (*projectUnshare)(nil)

type projectUnshare struct {
	defaultCmd
}

func (punshare *projectUnshare) command() *cobra.Command {
	if punshare.cmd == nil {
		punshare.cmd = &cobra.Command{
			Use:   "unshare PROJECT-NAME",
			Short: "Deactivate the sharing of your devcontainer",
			Long: `CDS will remove the temporary ssh keypair from authorised_keys so that the keypair is no longer valid. ` +
				`Existing connections won't be closed.`,
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              punshare.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
	}
	return punshare.cmd
}

func (punshare *projectUnshare) subCommands() []baseCmd {
	return punshare.subCmds
}

func (punshare *projectUnshare) execute(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)

	// TODO: Leverage ContainerConf package once implemented — validate devcontainer configuration
	clog.Debug("Skipping devcontainer configuration validation — ContainerConf not yet implemented")

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to unshare.\n" +
			getTipRun(projectName))
	}

	// TODO: Agent Service interaction needed — unshare container access:
	// 1. Align container statuses via engine (alignContainerStatuses)
	// 2. Verify running containers exist
	// 3. Get first running container info
	// 4. Delete shared keys from container authorized_keys (engine.DeleteSharedKeys)
	// 5. Remove temporary keypair directory (os.RemoveAll on shared keys path)
	clog.Info(fmt.Sprintf("Project '%s' unshare requires agent service to proceed.", projectName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'unshare' operation")
}
