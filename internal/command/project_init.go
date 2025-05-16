package command

import (
	"github.com/spf13/cobra"
)

type projectInit struct {
	confDir          string
	projectName      string
	flavour          string
	overrideDir      string
	pullLatest       bool
	overrideImageTag string
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/
func (pi *projectInit) initFlags() {
	pi.cmd.Flags().StringVarP(&pi.confDir, "conf-dir", "C", "", `Directory where .devcontainer/ can be found or generated
(absolute or relative path, default=$PWD)`)
	_ = pi.cmd.MarkFlagDirname("conf-dir")
	pi.cmd.Flags().StringVarP(&pi.projectName, "name", "n", "", `Project name to use (default=default)`)
	pi.cmd.Flags().StringVarP(&pi.flavour, "flavour", "f", "", `Development environment flavour to use`)
	_ = pi.cmd.RegisterFlagCompletionFunc("flavour", completionFlavour)
	pi.cmd.Flags().StringVarP(&pi.overrideDir, "override-dir", "O", "", `Path to where to add configuration override dir
.override (absolute path | relative path, default=$PWD)`)
	_ = pi.cmd.MarkFlagDirname("override-dir")
	pi.cmd.Flags().StringVar(&pi.overrideImageTag, "override-image-tag", "", `Change the devcontainer underlying OCI image's tag to the specified one`)
	pi.cmd.Flags().BoolVarP(&pi.pullLatest, "pull-latest", "", false, `Change the devcontainer underlying OCI image's tag to 'latest'`)
}

func (pi *projectInit) initSubCommands() {
	pi.subCmds = []baseCmd{}
}

func (pi *projectInit) preRunE(cmd *cobra.Command, args []string) error {
	return nil
}

func (pi *projectInit) runE(cmd *cobra.Command, args []string) error {
	return nil
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/
var _ baseCmd = (*projectInit)(nil)

func (pi *projectInit) command() *cobra.Command {
	if pi.cmd == nil {
		pi.cmd = &cobra.Command{
			Use:           "init",
			Short:         "Initialize a CDS project",
			Long:          `Register a new CDS project by either providing a ".devcontainer" or letting CDS generate a template one`,
			Args:          cobra.NoArgs,
			PreRunE:       pi.preRunE,
			RunE:          pi.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		pi.initFlags()
		pi.initSubCommands()
	}
	return pi.cmd
}

func (pi *projectInit) subCommands() []baseCmd {
	return pi.subCmds
}
