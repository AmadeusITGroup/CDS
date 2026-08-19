package command

import (
	"fmt"
	"net"
	"time"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

var _ baseCmd = (*projectExpose)(nil)

type serviceInfo struct {
	local       string
	remote      string
	serviceName string
}

type projectExpose struct {
	defaultCmd
	service serviceInfo
	timeout time.Duration
}

func (pexpose *projectExpose) command() *cobra.Command {
	if pexpose.cmd == nil {
		pexpose.cmd = &cobra.Command{
			Use:           "expose",
			Short:         "expose a service on the currently deployed devcontainer",
			Long:          `CDS will generate a port forward to the deployed container between a service on the machine and a local port. `,
			Args:          validateProjectNameFromArgsOrContext,
			RunE:          pexpose.execute,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		pexpose.initFlags()
	}
	return pexpose.cmd
}

func (pexpose *projectExpose) subCommands() []baseCmd {
	return pexpose.subCmds
}

func (pexpose *projectExpose) initFlags() {
	pexpose.cmd.Flags().StringVarP(&pexpose.service.local, "local", "l", "localhost:1337", "Local address where the service will be routed. (format IP:PORT)")
	pexpose.cmd.Flags().StringVarP(&pexpose.service.remote, "remote", "r", "", "Remote address where the service will be sought, this will be ignored if service is filled. (format IP:PORT)")
	pexpose.cmd.Flags().StringVarP(&pexpose.service.serviceName, "service", "s", "", "Choose a service to expose, leave both remote and service empty to get a selection prompt.")
	pexpose.cmd.Flags().DurationVarP(&pexpose.timeout, "timeout", "t", time.Hour, "Enter a duration that will be used to timeout the whole server if it doesn't receive activity in this period of time")
}

func (pexpose *projectExpose) execute(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to expose services from.\n" +
			getTipRun(projectName))
	}

	remote := pexpose.service.remote
	if pexpose.service.serviceName != "" {
		if _, _, err := net.SplitHostPort(pexpose.service.serviceName); err != nil {
			return cerr.NewError("service name lookup is not available through the agent yet; provide --remote HOST:PORT")
		}
		remote = pexpose.service.serviceName
	}
	if remote == "" {
		return cerr.NewError("remote address is required; provide --remote HOST:PORT")
	}
	if _, _, err := net.SplitHostPort(pexpose.service.local); err != nil {
		return cerr.AppendError(fmt.Sprintf("invalid local address %q", pexpose.service.local), err)
	}
	if _, _, err := net.SplitHostPort(remote); err != nil {
		return cerr.AppendError(fmt.Sprintf("invalid remote address %q", remote), err)
	}

	if err := syncProjectContainers(projectName); err != nil {
		return err
	}
	target, containerName, err := projectContainerSSHTarget(projectName)
	if err != nil {
		return err
	}

	clog.Info(fmt.Sprintf("Forwarding %s to %s through container %s.", pexpose.service.local, remote, containerName))
	return shexec.ForwardPort(target, pexpose.service.local, remote, pexpose.timeout)
}
