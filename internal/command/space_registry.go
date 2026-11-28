package command

import "github.com/spf13/cobra"

// spcRegistry is the `cds space registry` command group: manage the secure
// container registry on an agent host.
type spcRegistry struct {
	defaultCmd
}

var _ baseCmd = (*spcRegistry)(nil)

func (s *spcRegistry) initSubCommands() {
	s.subCmds = append(s.subCmds,
		&spcRegistryRun{},
		&spcRegistryStart{},
		&spcRegistryStop{},
		&spcRegistryDelete{},
		&spcRegistryStatus{},
	)
}

func (s *spcRegistry) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "registry",
			Aliases:       []string{"reg"},
			Short:         "Manage the secure registry on a host",
			Long:          `Build, start, stop, delete and inspect the local secure container registry managed by the agent on a target host.`,
			Args:          cobra.NoArgs,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.initSubCommands()
	}
	return s.cmd
}

func (s *spcRegistry) subCommands() []baseCmd {
	return s.subCmds
}

// regTarget holds the common -t/--target flag for registry leaf commands.
type regTarget struct {
	target string
}

func (t *regTarget) bind(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&t.target, "target", "t", "localhost", "target host running the agent")
}

func (t *regTarget) hostName() (string, error) {
	return bootstrapHostName(t.target)
}
