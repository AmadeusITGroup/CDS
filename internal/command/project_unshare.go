package command

import (
	"context"
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

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to unshare.\n" +
			getTipRun(projectName))
	}

	if err := syncProjectContainers(projectName); err != nil {
		return err
	}

	publicKey, keyPair, err := existingProjectSharedPublicKey(projectName)
	if err != nil {
		return err
	}

	return withProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		containerName, remoteUser, err := projectPrimaryContainer(projectName)
		if err != nil {
			return err
		}
		if _, err := executeProjectContainerCommand(ctx, services.container, projectName, removeSharedKeyCommand(remoteUser, publicKey), projectSharedKeyUser); err != nil {
			return cerr.AppendErrorFmt("Failed to remove shared SSH key from container %s", err, containerName)
		}

		removeProjectSharedKeyPair(projectName)
		clog.Info(fmt.Sprintf("Project '%s' is no longer shared through container '%s'.", projectName, containerName))
		clog.Info(fmt.Sprintf("Removed shared key pair rooted at %s", keyPair.PathToPrv))
		return nil
	})
}
