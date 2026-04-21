package command

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

type spcHostDelete struct {
	defaultCmd
}

var _ baseCmd = (*spcHostDelete)(nil)

func (s *spcHostDelete) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "delete <target-server>",
			Aliases:       []string{"remove", "rm"},
			Short:         "Delete a registered agent host",
			Args:          cobra.ExactArgs(1),
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
	}
	return s.cmd
}

func (s *spcHostDelete) subCommands() []baseCmd {
	return s.subCmds
}

func (s *spcHostDelete) runE(cmd *cobra.Command, args []string) error {
	target := args[0]
	// TODO delete the agent from the host
	// bootstrap package looks to be only in charge of the startup of the agents which leave a gap in functionality for the management of the registered agents (listing, deletion...).
	// I think we should move the responsibility of managing the registered agents to a dedicated package.
	// bootstrap would either be included in this package or called by this package when starting an agent.
	// Like `agentManager.(Start|Stop|List|Delete)Agent(...)` with start relying on bootstrap ?
	if err := config.DeleteAgentFromConfig(target); err != nil {
		return err
	}
	o := output.FromContext(cmd.Context())
	return output.Render(o, output.SimpleResult{Message: fmt.Sprintf("Deleted host %q", target)})
}
