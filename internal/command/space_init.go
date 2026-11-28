package command

import (
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/spf13/cobra"
)

type spcInit struct {
	profile string
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (sci *spcInit) initFlags() {
	// TODO:Analyse: make the profile fetchable on git or ar
	sci.cmd.Flags().StringVarP(&sci.profile, "use-profile", "p", "", `Path to the profile file`)
}

func (sci *spcInit) initSubCommands() {
	sci.subCmds = []baseCmd{}
}

func (sci *spcInit) runE(cmd *cobra.Command, args []string) error {
	if sci.profile != "" {
		return cerr.NewError("--use-profile is not implemented yet: `cds space init` does not apply a profile")
	}
	return nil
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*spcInit)(nil)

func (sci *spcInit) command() *cobra.Command {
	if sci.cmd == nil {
		sci.cmd = &cobra.Command{
			Use:           "init",
			Short:         "Initialize CDS configuration",
			Long:          `Initialize CDS configuration. Note: configuration can be based on an existing one`,
			Args:          cobra.NoArgs,
			RunE:          sci.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
			PersistentPreRun: func(cmd *cobra.Command, args []string) {
				// override CDS persistent pre run safe guarding cds usage without space init
			},
		}
		sci.initFlags()
		sci.initSubCommands()
	}
	return sci.cmd
}

func (sci *spcInit) subCommands() []baseCmd {
	return sci.subCmds
}
