package command

import "github.com/spf13/cobra"

type spcHost struct {
	defaultCmd
}

func (s *spcHost) initSubCommands() {
	s.subCmds = append(s.subCmds,
		&spcHostAdd{},
		&spcHostList{},
		&spcHostGet{},
		&spcHostDelete{},
	)
}

var _ baseCmd = (*spcHost)(nil)

func (s *spcHost) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "host",
			Aliases:       []string{"hosts"},
			Short:         "Bootstrap and manage registered agent hosts",
			Long:          `Bootstrap agent hosts and manage the registered agent hosts stored in the CLI configuration`,
			Args:          cobra.NoArgs,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.initSubCommands()
	}
	return s.cmd
}

func (s *spcHost) subCommands() []baseCmd {
	return s.subCmds
}
