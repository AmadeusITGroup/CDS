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

	containerName, err := syncedProjectPrimaryContainer(projectName)
	if err != nil {
		return err
	}
	keyPair, err := ensureProjectSharedKeyPair(projectName)
	if err != nil {
		return err
	}
	publicKey, err := readPublicKey(keyPair)
	if err != nil {
		return err
	}

	remoteUser := db.ProjectContainerRemoteUser(projectName, containerName)
	if err := onProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		_, err := executeInContainer(ctx, services.container, containerName, engine.AuthorizedKeysCommand(true, remoteUser, publicKey), engine.KRootUsr)
		return err
	}); err != nil {
		return cerr.AppendErrorFmt("Failed to install the shared SSH key in container %s", err, containerName)
	}

	clog.Info(fmt.Sprintf("Project '%s' is shared through container '%s'.", projectName, containerName))
	clog.Info(fmt.Sprintf("Share the private key %s, to be used with:", keyPair.PathToPrv))
	clog.Info(fmt.Sprintf("  ssh -i <private key> -p %d %s@%s", db.ContainerSSHPort(projectName, containerName), remoteUser, projectAgentHost(projectName)))
	return nil
}
