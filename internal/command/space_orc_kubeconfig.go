package command

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

// spcOrcKubeconfig fetches the cluster kubeconfig from the agent and either
// prints it or writes it to a local file, optionally rewriting the server
// address so a client-reachable endpoint (e.g. an SSH-forwarded port) can be
// used. This is the client half of "expose": the server-side kind API is
// reachable on the agent host, so the caller points --server at the address
// they can reach it on.
type spcOrcKubeconfig struct {
	defaultCmd
	orcTarget
	server string
	write  bool
}

var _ baseCmd = (*spcOrcKubeconfig)(nil)

func (s *spcOrcKubeconfig) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "kubeconfig",
			Aliases:       []string{"expose"},
			Short:         "Fetch the cluster kubeconfig from the target host",
			Long: `Fetch the kind cluster kubeconfig from the agent on the target host.

By default the kubeconfig is printed. With --write it is written to the local
kubeconfig (~/.kube/config or $KUBECONFIG), replacing it. Use --server to
rewrite the API server address to one reachable from this machine (for example
a local port forwarded to the agent host with 'ssh -L <port>:127.0.0.1:<kind-api-port>').`,
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
		s.cmd.Flags().StringVar(&s.server, "server", "", "rewrite the API server host:port (e.g. 127.0.0.1:6443)")
		s.cmd.Flags().BoolVar(&s.write, "write", false, "write to the local kubeconfig instead of printing")
	}
	return s.cmd
}

func (s *spcOrcKubeconfig) subCommands() []baseCmd { return s.subCmds }

type spaceOrcKubeconfigResult struct {
	Path       string `json:"path,omitempty"`
	Kubeconfig string `json:"kubeconfig,omitempty"`
}

func (r spaceOrcKubeconfigResult) HumanReadable(output.OutputOptions) string {
	if r.Path != "" {
		return fmt.Sprintf("Kubeconfig written to %s\n", r.Path)
	}
	return r.Kubeconfig
}

func (r spaceOrcKubeconfigResult) MachineReadable() any { return r }

func (s *spcOrcKubeconfig) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.cluster.GetKubeconfig(ctx, &cdspb.GetKubeconfigRequest{})
		if err != nil {
			return err
		}
		kubeconfig := resp.GetKubeconfig()
		if kubeconfig == cg.EmptyStr {
			return cerr.NewError("agent returned an empty kubeconfig; is the cluster built?")
		}

		data := []byte(kubeconfig)
		if s.server != cg.EmptyStr {
			rewritten, rwErr := rewriteKubeconfigServer(kubeconfig, s.server)
			if rwErr != nil {
				return rwErr
			}
			data = rewritten
		}

		o := output.FromContext(cmd.Context())
		if s.write {
			path := localKubeconfigPath()
			if err := writeKubeconfig(path, data); err != nil {
				return err
			}
			clog.Warn(fmt.Sprintf("Local kubeconfig %s was replaced with the cluster kubeconfig", path))
			return output.Render(o, spaceOrcKubeconfigResult{Path: path})
		}
		return output.Render(o, spaceOrcKubeconfigResult{Kubeconfig: string(data)})
	}
	return callback.executeOn(hostName, withoutAutostart())
}
