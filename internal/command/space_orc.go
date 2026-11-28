package command

import "github.com/spf13/cobra"

// spcOrc is the `cds space orc` command group: manage the local kind
// orchestration cluster on an agent host.
type spcOrc struct {
	defaultCmd
}

var _ baseCmd = (*spcOrc)(nil)

func (s *spcOrc) initSubCommands() {
	s.subCmds = append(s.subCmds,
		&spcOrcRun{},
		&spcOrcStart{},
		&spcOrcStop{},
		&spcOrcDelete{},
		&spcOrcRebuild{},
		&spcOrcStatus{},
		&spcOrcKubeconfig{},
	)
}

func (s *spcOrc) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "orc",
			Aliases:       []string{"orchestration", "cluster"},
			Short:         "Manage the orchestration cluster (kind) on a host",
			Long:          `Build, start, stop, delete and inspect the local kind orchestration cluster managed by the agent on a target host.`,
			Args:          cobra.NoArgs,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.initSubCommands()
	}
	return s.cmd
}

func (s *spcOrc) subCommands() []baseCmd {
	return s.subCmds
}

// orcTarget holds the common -t/--target flag for orc leaf commands. It
// defaults to localhost.
type orcTarget struct {
	target string
}

func (t *orcTarget) bind(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&t.target, "target", "t", "localhost", "target host running the agent")
}

func (t *orcTarget) hostName() (string, error) {
	return bootstrapHostName(t.target)
}
