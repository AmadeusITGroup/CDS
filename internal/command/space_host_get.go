package command

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

type spcHostGet struct {
	defaultCmd
}

var _ baseCmd = (*spcHostGet)(nil)

func (s *spcHostGet) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "get <target-server>",
			Aliases:       []string{"show"},
			Short:         "Show a registered agent host",
			Args:          cobra.ExactArgs(1),
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
	}
	return s.cmd
}

func (s *spcHostGet) subCommands() []baseCmd {
	return s.subCmds
}

type spaceHostGetResult struct {
	Hostname  string `json:"hostname"`
	Reachable bool   `json:"reachable"`
	Version   string `json:"version"`
}

func (r spaceHostGetResult) HumanReadable(output.OutputOptions) string {
	reachable := "unreachable"
	if r.Reachable {
		reachable = "reachable"
	}
	if r.Version != "" {
		return fmt.Sprintf("Host:    %s\nStatus:  %s\nVersion: %s\n", r.Hostname, reachable, r.Version)
	}
	return fmt.Sprintf("Host:    %s\nStatus:  %s\n", r.Hostname, reachable)
}

func (r spaceHostGetResult) MachineReadable() any {
	return r
}

func (s *spcHostGet) runE(cmd *cobra.Command, args []string) error {
	agent, err := config.RegisteredAgent(args[0])
	if err != nil {
		return err
	}

	// TODO: Sync agent status.
	reachable := func() bool {
		return false
	}()

	// TODO: Do the actual RPC call to the agent to know its version
	version := func() string {
		return "0.0.0-unimplemented"
	}

	o := output.FromContext(cmd.Context())
	return output.Render(o, spaceHostGetResult{
		Hostname:  agent.TargetSrv,
		Reachable: reachable,
		Version:   version(),
	})
}
