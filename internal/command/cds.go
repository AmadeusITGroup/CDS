package command

import (
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

type cds struct {
	verbose    bool
	outputFlag string // --output: "json", "text", "auto"
	quiet      bool   // --quiet: suppress all output
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (c *cds) initFlags() {
	c.cmd.PersistentFlags().BoolVarP(&c.verbose, "verbose", "v", false, "Verbose output, switches log level to Debug")
	c.cmd.PersistentFlags().StringVarP(&c.outputFlag, "output", "o", "auto", `Output format: "auto", "json", "text"`)
	c.cmd.PersistentFlags().BoolVarP(&c.quiet, "quiet", "q", false, "Suppress all output; exit code signals success or failure")
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
				cmdPath := strings.TrimPrefix(cmd.CommandPath(), "cds ")
				preparedCmd := strings.ReplaceAll(cmdPath, " ", ".")
				octx, err := output.NewOutputOptions(output.WithDetect(c.outputFlag, c.quiet, c.verbose), output.WithCommand(preparedCmd))
				if err != nil {
					return cerr.AppendError("There was an error initializing output options", err)
				}
				cmd.SetContext(output.WithOutputOptions(cmd.Context(), octx))
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
