package command

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

// spcRegistryStatus reports the current registry status.
type spcRegistryStatus struct {
	defaultCmd
	regTarget
}

var _ baseCmd = (*spcRegistryStatus)(nil)

func (s *spcRegistryStatus) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "status",
			Aliases:       []string{"get", "show"},
			Short:         "Show the secure registry status on the target host",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.bind(s.cmd)
	}
	return s.cmd
}

func (s *spcRegistryStatus) subCommands() []baseCmd { return s.subCmds }

type spaceRegistryStatusResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Port     int32  `json:"port"`
	Hostname string `json:"hostname"`
	Host     string `json:"host"`
}

func (r spaceRegistryStatusResult) HumanReadable(output.OutputOptions) string {
	return fmt.Sprintf("Registry: %s\nStatus:   %s\nEndpoint: %s:%d\nHost:     %s\n", r.Name, r.Status, r.Hostname, r.Port, r.Host)
}

func (r spaceRegistryStatusResult) MachineReadable() any { return r }

func (s *spcRegistryStatus) runE(cmd *cobra.Command, _ []string) error {
	hostName, err := s.hostName()
	if err != nil {
		return err
	}
	var callback stubCallback = func(c agentServices, ctx context.Context) error {
		resp, err := c.registry.GetRegistry(ctx, &cdspb.GetRegistryRequest{})
		if err != nil {
			return err
		}
		reg := resp.GetRegistry()
		o := output.FromContext(cmd.Context())
		return output.Render(o, spaceRegistryStatusResult{
			Name:     reg.GetName(),
			Status:   reg.GetStatus(),
			Port:     reg.GetPort(),
			Hostname: reg.GetHostname(),
			Host:     reg.GetHost(),
		})
	}
	return callback.executeOn(hostName, withoutAutostart())
}
