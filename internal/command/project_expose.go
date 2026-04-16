package command

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
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

	// TODO: Leverage ContainerConf package once implemented — validate devcontainer configuration
	clog.Debug("Skipping devcontainer configuration validation — ContainerConf not yet implemented")

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cerr.NewError("No containers to expose services from.\n" +
			getTipRun(projectName))
	}

	// TODO: Agent Service interaction needed — expose service via port forwarding:
	// 1. Align container statuses via engine (alignContainerStatuses)
	// 2. Verify running containers exist
	// 3. Get first running container info (including SSH port mapping)
	// 4. Build target host with SSH key paths
	// 5. Resolve exposure: handle KinD service (patch local kubeconfig) or direct port forward
	// 6. Forward port via shexec.ForwardPort with timeout
	clog.Info(fmt.Sprintf("Project '%s' has containers configured. Agent service required for port forwarding.", projectName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'expose' operation")
}
