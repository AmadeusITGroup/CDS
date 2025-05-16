package command

import (
	"github.com/spf13/cobra"
	"github.com/amadeusitgroup/cds/internal/clog"
)

type cds struct {
	verbose bool
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (c *cds) initFlags() {
	c.cmd.PersistentFlags().BoolVarP(&c.verbose, "verbose", "v", false, "Verbose output, switches log level to Debug")
}

func (c *cds) initSubCommands() {
	c.subCmds = append(c.subCmds, &project{}, &spc{}, &version{} /*, &marketplace{}*/)
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*cds)(nil)

func (c *cds) command() *cobra.Command {
	if c.cmd == nil {
		c.cmd = &cobra.Command{
			Use:   "cds",
			Short: "Containers Development Space",
			Long: `CDS is a CLI for bootstrapping development environments.
The tool helps to create and configure the needed containers to quickly
create a containerized workspace, and a ready-to-use lightweight
containers orchestration platform.`,
			Args: cobra.NoArgs,
			PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
				if c.verbose {
					clog.Verbose()
				}
				return nil
			},
		}
		c.initFlags()
		c.initSubCommands()
	}
	return c.cmd
}

func (c *cds) subCommands() []baseCmd {
	return c.subCmds
}

/************************************************************/
/*                                                          */
/*              Implement `main.cmd` interface              */
/*                                                          */
/************************************************************/

func (c *cds) Execute() error {
	return c.cmd.Execute()
}
