package command

import (
	"context"
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

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to share.\n" +
			getTipRun(projectName))
	}

	if err := syncProjectContainers(projectName); err != nil {
		return err
	}

	publicKey, keyPair, err := projectSharedPublicKey(projectName)
	if err != nil {
		return err
	}

	return withProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		containerName, remoteUser, err := projectPrimaryContainer(projectName)
		if err != nil {
			return err
		}
		if _, err := executeProjectContainerCommand(ctx, services.container, projectName, installSharedKeyCommand(remoteUser, publicKey), projectSharedKeyUser); err != nil {
			return cerr.AppendErrorFmt("Failed to install shared SSH key in container %s", err, containerName)
		}

		sshPort := db.ContainerSSHPort(projectName, containerName)
		clog.Info(fmt.Sprintf("Project '%s' is shared through container '%s'.", projectName, containerName))
		clog.Info(fmt.Sprintf("Share private key: %s", keyPair.PathToPrv))
		clog.Info(fmt.Sprintf("Connection command: ssh -i %s -p %d %s@%s", keyPair.PathToPrv, sshPort, remoteUser, projectAgentHost(projectName)))
		return nil
	})
}
