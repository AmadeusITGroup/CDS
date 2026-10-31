package command

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
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

	if err := validateHostPort("local", pexpose.service.local); err != nil {
		return err
	}

	// Resolve the remote address to forward to. For --service kind the remote is
	// the cluster API server (and the local kubeconfig is patched to target the
	// local forward); otherwise --remote is used directly.
	remote := pexpose.service.remote
	if pexpose.service.serviceName != cg.EmptyStr {
		resolved, err := pexpose.resolveServiceRemote(projectName)
		if err != nil {
			return err
		}
		remote = resolved
	}
	if err := validateHostPort("remote", remote); err != nil {
		return err
	}

	containerName, err := syncedProjectPrimaryContainer(projectName)
	if err != nil {
		return err
	}
	target, err := containerSSHTarget(projectName, containerName, cg.EmptyStr)
	if err != nil {
		return err
	}

	clog.Info(fmt.Sprintf("Forwarding %s to %s through container %s.", pexpose.service.local, remote, containerName))
	return shexec.ForwardPort(target, pexpose.service.local, remote, pexpose.timeout)
}

// validateHostPort checks addr is a HOST:PORT.
func validateHostPort(flag, addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return cerr.AppendErrorFmt("invalid --%s address %q, expected HOST:PORT", err, flag, addr)
	}
	return nil
}

// resolveServiceRemote resolves the remote HOST:PORT to forward to for a named
// service. The only supported service today is "kind": the remote is the kind
// cluster API server (read from the cluster kubeconfig), and the local
// kubeconfig is patched to target the local forward address so kubectl on this
// machine reaches the cluster through the tunnel.
func (pexpose *projectExpose) resolveServiceRemote(projectName string) (string, error) {
	switch pexpose.service.serviceName {
	case "kind":
		return pexpose.exposeKind(projectName)
	default:
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("unknown service %q; supported: kind", pexpose.service.serviceName))
	}
}

// exposeKind fetches the cluster kubeconfig from the project's agent, extracts
// the API server address (the remote to forward to, reachable through the
// devcontainer on the kind network), and writes a local kubeconfig whose server
// points at the local forward address.
func (pexpose *projectExpose) exposeKind(projectName string) (string, error) {
	var kubeconfig string
	if err := onProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		resp, err := services.cluster.GetKubeconfig(ctx, &cdspb.GetKubeconfigRequest{})
		if err != nil {
			return err
		}
		kubeconfig = resp.GetKubeconfig()
		return nil
	}); err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to fetch cluster kubeconfig", err)
	}
	if kubeconfig == cg.EmptyStr {
		return cg.EmptyStr, cerr.NewError("cluster returned an empty kubeconfig; is the cluster built?")
	}

	remote, err := kubeconfigServerHostPort(kubeconfig)
	if err != nil {
		return cg.EmptyStr, err
	}

	// patch a local kubeconfig so kubectl targets the local forward address.
	patched, err := rewriteKubeconfigServer(kubeconfig, pexpose.service.local)
	if err != nil {
		return cg.EmptyStr, err
	}
	path := localKubeconfigPath()
	if err := writeKubeconfig(path, patched); err != nil {
		return cg.EmptyStr, err
	}
	clog.Info(fmt.Sprintf("Local kubeconfig %s now targets the cluster via %s", path, pexpose.service.local))
	return remote, nil
}
