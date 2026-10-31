package command

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

// spcOrcStatus reports the current cluster status.
type spcOrcStatus struct {
	defaultCmd
	orcTarget
}

var _ baseCmd = (*spcOrcStatus)(nil)

func (s *spcOrcStatus) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "status",
			Aliases:       []string{"get", "show"},
			Short:         "Show the orchestration cluster status on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
	}
	return s.cmd
}

func (s *spcOrcStatus) subCommands() []baseCmd { return s.subCmds }

type spaceOrcStatusResult struct {
	Name   string `json:"name"`
	Node   string `json:"node"`
	Status string `json:"status"`
	Host   string `json:"host"`
}

func (r spaceOrcStatusResult) HumanReadable(output.OutputOptions) string {
	return fmt.Sprintf("Cluster: %s\nNode:    %s\nStatus:  %s\nHost:    %s\n", r.Name, r.Node, r.Status, r.Host)
}

func (r spaceOrcStatusResult) MachineReadable() any { return r }

func (s *spcOrcStatus) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.cluster.GetCluster(ctx, &cdspb.GetClusterRequest{})
		if err != nil {
			return err
		}
		cluster := resp.GetCluster()
		o := output.FromContext(cmd.Context())
		return output.Render(o, spaceOrcStatusResult{
			Name:   cluster.GetName(),
			Node:   cluster.GetNode(),
			Status: cluster.GetStatus(),
			Host:   cluster.GetHost(),
		})
	}
	return callback.executeOn(hostName, withoutAutostart())
}
