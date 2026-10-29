package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/amadeusitgroup/cds/internal/engine"
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

	keyPair := projectSharedKeyPair(projectName)
	publicKey, err := readPublicKey(keyPair)
	if err != nil {
		return cerr.AppendErrorFmt("Project %s is not shared", err, projectName)
	}
	containerName, err := syncedProjectPrimaryContainer(projectName)
	if err != nil {
		return err
	}

	remoteUser := db.ProjectContainerRemoteUser(projectName, containerName)
	if err := onProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		_, err := executeInContainer(ctx, services.container, containerName, engine.AuthorizedKeysCommand(false, remoteUser, publicKey), engine.KRootUsr)
		return err
	}); err != nil {
		return cerr.AppendErrorFmt("Failed to remove the shared SSH key from container %s", err, containerName)
	}

	removeProjectSharedKeyPair(keyPair)
	clog.Info(fmt.Sprintf("Project '%s' is no longer shared through container '%s'.", projectName, containerName))
	return nil
}
