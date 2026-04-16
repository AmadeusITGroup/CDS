package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

var _ baseCmd = (*projectShare)(nil)

type projectShare struct {
	defaultCmd
}

func (pshare *projectShare) command() *cobra.Command {
	if pshare.cmd == nil {
		pshare.cmd = &cobra.Command{
			Use:   "share PROJECT-NAME",
			Short: "share your devcontainer with other colleagues",
			Long: `CDS will create an ssh keypair which you can share with your colleagues so that they can access your devcontainer and help you with any investigation. ` +
				`You have to unshare the project in order to make the ssh keypair no longer valid.`,
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              pshare.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
	}
	return pshare.cmd
}

func (pshare *projectShare) subCommands() []baseCmd {
	return pshare.subCmds
}

func (pshare *projectShare) execute(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)

	// TODO: Leverage ContainerConf package once implemented — validate devcontainer configuration
	clog.Debug("Skipping devcontainer configuration validation — ContainerConf not yet implemented")

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to share.\n" +
			getTipRun(projectName))
	}

	// TODO: Agent Service interaction needed — share container access:
	// 1. Align container statuses via engine (alignContainerStatuses)
	// 2. Verify running containers exist
	// 3. Get first running container info
	// 4. Generate temporary SSH keypair (shexec.GenerateSharedKeys)
	// 5. Add temporary shared keys to container authorized_keys (engine.AddTempSharedKeys)
	// 6. Display private key content and connection instructions to user
	clog.Info(fmt.Sprintf("Project '%s' share requires agent service to proceed.", projectName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'share' operation")
}
