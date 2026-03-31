package command

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

const (
	kLatestTag = "latest"
)

type projectInit struct {
	confDir          string
	projectName      string
	flavour          string
	overrideDir      string
	pullLatest       bool
	overrideImageTag string
	nonInteractive   bool
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
	pi.cmd.Flags().BoolVarP(&pi.nonInteractive, "non-interactive", "", false, `Run in non-interactive mode (e.g. for CI)`)
}

func (pi *projectInit) initSubCommands() {
	pi.subCmds = []baseCmd{}
}

func (pi *projectInit) preRunE(cmd *cobra.Command, args []string) error {
	if _, err := isValidProjectName(pi.projectName); err != nil {
		return err
	}

	if len(pi.projectName) == 0 {
		pi.projectName = db.KDefaultProjectName
	}

	if db.HasProject(pi.projectName) {
		return cerr.NewError(fmt.Sprintf(
			"Project %q is defined in the space configuration. Use a different name with --name",
			pi.projectName))
	}

	if errArgs := pi.checkMutualExclusiveness(); errArgs != nil {
		return errArgs
	}

	if len(pi.overrideImageTag) > 0 {
		if err := validateImageTagSyntax(pi.overrideImageTag); err != nil {
			return err
		}
	}

	// Handling --confDir use case:
	if len(pi.confDir) != 0 {
		return pi.handleConfDir()
	}

	// Handling (--flavour, --override-dir) use case
	// TODO: Artifactory Service interaction needed — prepareCredentials for Artifactory auth
	// if err := prepareCredentials(pi.artifactoryUser, pi.artifactoryPassword); err != nil {
	// 	return cerr.AppendError("Failed to prepare credentials for flavour", err)
	// }

	if err := pi.handleFlavour(); err != nil {
		return cerr.AppendError("Failed to determine flavour", err)
	}

	if len(pi.overrideDir) != 0 {
		var absErr error
		pi.overrideDir, absErr = toAbsolute(pi.overrideDir)
		if absErr != nil {
			return cerr.AppendError("Failed to transform dir!", absErr)
		}
	}

	return nil
}

func (pi *projectInit) handleFlavour() error {
	// Enforce usage of override dir if flavour is specified
	if len(pi.flavour) == 0 && len(pi.overrideDir) != 0 {
		clog.Warn("Override directory specified through --override-dir option is ignored as no flavour is specified in the same command!")
		pi.overrideDir = ""
	}

	// TODO: Artifactory Service interaction needed — fetch flavour list from Artifactory/marketplace
	// flavoursNames, err := container.ListFlavours()
	flavoursNames := []string{} // Placeholder until server integration
	_ = flavoursNames

	if len(pi.flavour) == 0 {
		if pi.nonInteractive {
			return cerr.NewError("No flavour specified and running in non-interactive mode. Please specify a flavour using --flavour flag")
		}
		pi.flavour, pi.overrideDir = setFlavourInteractively(flavoursNames)
		return nil
	}

	// TODO: Artifactory Service interaction needed — validate flavour exists in marketplace
	// if !slices.Contains(flavoursNames, pi.flavour) {
	// 	...
	// }

	return nil
}

func (pi *projectInit) handleConfDir() error {
	var absErr error
	pi.confDir, absErr = toAbsolute(pi.confDir)
	if absErr != nil {
		return cerr.AppendError("Failed to transform dir!", absErr)
	}

	if confInUse(pi.confDir) {
		return cerr.NewError("Specified directory is already in use!")
	}
	return nil
}

func (pi *projectInit) checkMutualExclusiveness() error {
	if len(pi.confDir) != 0 && (len(pi.flavour) != 0 || len(pi.overrideDir) != 0) {
		var message string
		if len(pi.flavour) != 0 {
			message = "--conf-dir and --flavour are mutually exclusive"
		} else if len(pi.overrideDir) != 0 {
			message = "--conf-dir and --override-conf are mutually exclusive"
		}
		return cerr.NewError(message)
	}
	if len(pi.overrideImageTag) != 0 && pi.pullLatest {
		return cerr.NewError("--override-dir and --pull-latest are mutually exclusive")
	}
	return nil
}

func (pi *projectInit) handleImageTag() error {
	if len(pi.overrideImageTag) > 0 {
		return db.SetOverrideImageTag(pi.projectName, pi.overrideImageTag)
	}

	if pi.pullLatest {
		return db.SetOverrideImageTag(pi.projectName, kLatestTag)
	}
	return nil
}

func (pi *projectInit) runE(cmd *cobra.Command, args []string) error {
	if len(pi.confDir) != 0 {
		pathToConfDir := filepath.Join(pi.confDir, db.KCdsProjectDefaultDir)

		// TODO: Leverage ContainerConf package once implemented — engine.InitConfigDir(pathToConfDir) to validate/prepare the config directory
		clog.Debug(fmt.Sprintf("Using config directory: %s", pathToConfDir))

		if err := db.AddProjectUsingConfDir(pi.projectName, pathToConfDir); err != nil {
			return cerr.AppendError(
				fmt.Sprintf("Failed to update configuration (project: %s, config path: %s)", pi.projectName, pathToConfDir), err)
		}
	} else {
		// Flavour use case
		if err := pi.handleFlavourUseCase(); err != nil {
			return cerr.AppendError("Failed to initialize project with flavour", err)
		}
	}

	if len(db.GetCurrentProject()) == 0 {
		clog.Info(fmt.Sprintf("Project selection was empty -> selecting project '%s'", pi.projectName))
		if errSetProj := db.SetProject(pi.projectName); errSetProj != nil {
			return cerr.AppendError("Failed to set project to newly initialized project", errSetProj)
		}
	}

	// TODO: Leverage ContainerConf package — validate devcontainer configuration
	// if err := initializeProjectConfiguration(pi.projectName); err != nil {
	// 	return cerr.AppendError(fmt.Sprintf("Failed to initialize project %s's configuration", pi.projectName), err)
	// }

	if err := pi.handleImageTag(); err != nil {
		return err
	}

	return nil
}

func (pi *projectInit) handleFlavourUseCase() error {
	// TODO: Artifactory Service interaction needed — download and prepare flavour from Artifactory/marketplace
	// f, errFlavour := container.GetLocalFlavour(pi.flavour)
	// if errFlavour != nil {
	// 	return cerr.AppendErrorFmt("Failed to get flavour '%s'", errFlavour, pi.flavour)
	// }
	// if err := db.BuildFlavourLocalConfDir(pi.projectName, f.Path, pi.overrideDir); err != nil {
	// 	return cerr.AppendError(...)
	// }

	if err := db.AddProjectUsingFlavour(pi.projectName, pi.flavour, pi.overrideDir); err != nil {
		return cerr.AppendError(
			fmt.Sprintf("Failed to update configuration (project: %s, flavour: %s, override path: %s)",
				pi.projectName,
				pi.flavour,
				pi.overrideDir), err)
	}

	return nil
}

// toAbsolute converts a relative path to an absolute one using the current working directory.
func toAbsolute(path string) (string, error) {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "~") {
		return path, nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", cerr.AppendError("Failed to get absolute path", err)
	}
	return absPath, nil
}

// setFlavourInteractively prompts the user to select a flavour and optionally set an override directory.
func setFlavourInteractively(options []string) (selectedFlavour, overrideDir string) {
	if len(options) == 0 {
		clog.Warn("No flavours available for selection")
		return "", ""
	}
	selectedFlavour, _ = pterm.DefaultInteractiveSelect.WithOptions(options).Show("Please select a flavour")
	pterm.Info.Printfln("Selected Flavour: %s", pterm.Green(selectedFlavour))

	result, _ := pterm.DefaultInteractiveConfirm.Show("Add an override directory?")
	pterm.Println() // Blank line
	if result {
		overrideDir, _ = pterm.DefaultInteractiveTextInput.WithMultiLine(false).Show("Type your override dir")
	}
	return
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
