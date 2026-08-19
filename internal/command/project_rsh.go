package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/amadeusitgroup/cds/internal/host"
	"github.com/amadeusitgroup/cds/internal/shexec"
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

	if err := withProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		return syncProjectContainersFromAgent(ctx, services.container, projectName, containers, false)
	}); err != nil {
		return err
	}

	containers = db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to open remote shell into.\n" +
			getTipRun(projectName))
	}
	containerName := containers[0]
	port := db.ContainerSSHPort(projectName, containerName)
	if port <= 0 {
		return cerr.NewError(fmt.Sprintf("Container %q does not expose an SSH port", containerName))
	}

	remoteUser := prsh.user
	if remoteUser == "" {
		remoteUser = db.ProjectContainerRemoteUser(projectName, containerName)
	}
	if remoteUser == "" {
		return cerr.NewError(fmt.Sprintf("Container %q does not have a configured remote user", containerName))
	}

	privateKeyPath, publicKeyPath, err := projectSSHKeyPair(projectName)
	if err != nil {
		return err
	}
	target := host.New(
		host.WithName(projectAgentHost(projectName)),
		host.WithUsername(remoteUser),
		host.WithPort(port),
		host.WithKeyPair(host.NewKeyPair(
			host.WithPathToPrv(privateKeyPath),
			host.WithPathToPub(publicKeyPath),
		)),
	)
	return shexec.AttachShellUsingKey(target)
}
