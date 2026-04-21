package command

import "github.com/spf13/cobra"

type spc struct {
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/
func (s *spc) initSubCommands() {
	s.subCmds = append(s.subCmds,
		&spcInit{},
		&spcHost{},
		//&spcRegistry{},
		//&spcProfile{},
		//&spcOrc{},
	)
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*spc)(nil)

func (s *spc) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:     "space",
			Aliases: []string{"s", "sp"},
			Short:   "cds's space configuration and state.",
			Long:    `Manage cds's space configuration and monitor its state`,
		}
		s.initSubCommands()
	}
	return s.cmd
}

func (s *spc) subCommands() []baseCmd {
	return s.subCmds
}
