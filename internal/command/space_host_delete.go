package command

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/bootstrap"
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/db"
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
	hostName, err := bootstrapHostName(args[0])
	if err != nil {
		return err
	}

	// Unregister is the mirror of `host add`: stop the local agent while the CLI
	// config still knows its address, remove the agent from the CLI config, and
	// for a local host also drop the db.json host entry that registration wrote
	// (runtimeInfo). Deleting from the config is the source of truth, so its
	// failure aborts; the db cleanup only runs for a host we actually track.
	if isLocalTarget(hostName) {
		if err := bootstrap.StopAgent(hostName); err != nil {
			return err
		}
	}
	if err := config.DeleteAgentFromConfig(args[0]); err != nil {
		return err
	}
	if isLocalTarget(hostName) && db.HasHost(hostName) {
		db.RemoveHostFromHostList(hostName)
	}

	o := output.FromContext(cmd.Context())
	return output.Render(o, output.SimpleResult{Message: fmt.Sprintf("Deleted host %q", hostName)})
}
