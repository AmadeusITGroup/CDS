package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

type projectRebuild struct {
	overrideImageTag string
	pullLatest       bool
	pullGiven        bool
	defaultCmd
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*projectRebuild)(nil)

func (pr *projectRebuild) command() *cobra.Command {
	if pr.cmd == nil {
		pr.cmd = &cobra.Command{
			Use:               "rebuild",
			Aliases:           []string{"reb"},
			Short:             "Rebuild specified project",
			Long:              `Rebuild specified project: clearing the currently deployed resources and rebuilding them.`,
			Args:              validateProjectNameFromArgsOrContext,
			PreRunE:           pr.preRunE,
			RunE:              pr.runE,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		pr.initFlags()
		pr.initSubCommands()
	}
	return pr.cmd
}

func (pr *projectRebuild) subCommands() []baseCmd {
	return pr.subCmds
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (pr *projectRebuild) initFlags() {
	pr.cmd.Flags().StringVar(&pr.overrideImageTag, "override-image-tag", "", `Change the devcontainer underlying OCI image's tag to the specified one`)
	pr.cmd.Flags().BoolVarP(&pr.pullLatest, "pull-latest", "", false, `Change the devcontainer underlying OCI image's tag to the 'latest'`)
	pr.cmd.Flags().BoolVarP(&pr.pullGiven, "pull-given", "", false, `Ensures that the pulled image is of the given tag and not latest.`)
}

func (pr *projectRebuild) initSubCommands() {
	pr.subCmds = []baseCmd{}
}

func (pr *projectRebuild) preRunE(cmd *cobra.Command, args []string) error {
	if errArgs := pr.checkMutualExclusiveness(); errArgs != nil {
		return errArgs
	}
	if err := pr.checkCommandSemantic(); err != nil {
		return err
	}

	projectName := getProjectNameFromArgsOrContext(args)
	clog.Info(fmt.Sprintf("Using project '%s'.", projectName))

	// TODO: Artifactory Service interaction needed — verify if project's flavour needs update
	if db.IsProjectConfiguredWithFlavour(projectName) {
		clog.Debug("Skipping flavour update check — Artifactory service not yet implemented")
	}

	// TODO: Leverage ContainerConf package once implemented — validate devcontainer configuration
	clog.Debug("Skipping devcontainer configuration validation — ContainerConf not yet implemented")

	if err := pr.handleImageTag(projectName); err != nil {
		return err
	}

	return nil
}

func (pr *projectRebuild) runE(cmd *cobra.Command, args []string) error {
	clog.Debug("[commands.projectRebuild.runE] Start")
	defer clog.Debug("[commands.projectRebuild.runE] End")

	projectName := db.GetCurrentProject()

	// TODO: Agent Service interaction needed — full rebuild sequence:
	// 1. Stop and remove existing containers (stopRemoveContainersThenSync)
	// 2. Verify containers are deleted (checkContainerDeletion)
	// 3. Re-run the full build (runE with orchestration, registry, SSM, devcontainer)
	clog.Info(fmt.Sprintf("Project '%s' is ready for rebuilding. Agent service required to proceed.", projectName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'rebuild' operation")
}

/************************************************************/
/*                                                          */
/*                     preRunE Helpers                      */
/*                                                          */
/************************************************************/

func (pr *projectRebuild) checkMutualExclusiveness() error {
	if len(pr.overrideImageTag) != 0 && (pr.pullLatest || pr.pullGiven) {
		return cerr.NewError(kErrorImageTagFlagExclusiveness)
	}
	if pr.pullLatest && (len(pr.overrideImageTag) != 0 || pr.pullGiven) {
		return cerr.NewError(kErrorImageTagFlagExclusiveness)
	}
	if pr.pullGiven && (len(pr.overrideImageTag) != 0 || pr.pullLatest) {
		return cerr.NewError(kErrorImageTagFlagExclusiveness)
	}
	return nil
}

func (pr *projectRebuild) checkCommandSemantic() error {
	if len(pr.overrideImageTag) > 0 {
		if err := validateImageTagSyntax(pr.overrideImageTag); err != nil {
			return err
		}
	}
	return nil
}

func (pr *projectRebuild) handleImageTag(projectName string) error {
	if len(pr.overrideImageTag) > 0 {
		return db.SetOverrideImageTag(projectName, pr.overrideImageTag)
	}

	if pr.pullLatest {
		return db.SetOverrideImageTag(projectName, kLatestTag)
	}

	if pr.pullGiven {
		return db.SetOverrideImageTag(projectName, "")
	}
	return nil
}
