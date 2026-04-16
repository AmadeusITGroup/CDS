package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

type projectRun struct {
	engine           string
	engineArgs       string
	targetServer     string
	path             string
	srcRepo          string
	branch           string
	setSshTunnel     bool
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

var _ baseCmd = (*projectRun)(nil)

func (pr *projectRun) command() *cobra.Command {
	if pr.cmd == nil {
		pr.cmd = &cobra.Command{
			Use:     "run [PROJECT-NAME]",
			Aliases: []string{"r"},
			Short:   "Deploy containers and orchestration platforms",
			Long: `Use underlying OCI runtime engine to build OCI images, run containers and configure orchestration platforms.` +
				`A path to a project can be specified, it can be an initialized project or a new one.`,
			Args:              cobra.MaximumNArgs(1),
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

func (pr *projectRun) subCommands() []baseCmd {
	return pr.subCmds
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (pr *projectRun) initFlags() {
	pr.cmd.Flags().StringVarP(&pr.engine, "engine", "e", "podman", `Container runtime engine to use (default=podman, docker not supported yet)`)
	pr.cmd.Flags().StringVarP(&pr.engineArgs, "engine-args", "a", "", `Passed directly to the runtime engine. Warning: Be aware of what you're doing!!`)
	pr.cmd.Flags().StringVarP(&pr.targetServer, "target", "t", "", `Linux server to use to start containers (default=localdev).`)
	pr.cmd.Flags().StringVar(&pr.path, "path", "", `Path to .devcontainer dir. Exclusive argument vs positional argument projectName`)
	pr.cmd.Flags().StringVar(&pr.srcRepo, "src-repo", "", `Path to .git repository. Exclusive argument vs both --path option's argument and positional argument projectName.
Note: Currently, only HTTP type of clone URI is supported.`)
	pr.cmd.Flags().StringVar(&pr.branch, "branch", "", `.git repository's branch to clone. It can be used both with --path and --src-repo options.
The default branch of the targeted .git repository is used when branch is not specified`)
	pr.cmd.Flags().BoolVarP(&pr.setSshTunnel, "ssh-tunnel", "", false, `Update .ssh/config file to enable trigger of SSH Tunnel creation to access container.
This option is mainly used when the target host firewall is restricting access to part or all accessible host's ports.`)
	pr.cmd.Flags().StringVar(&pr.overrideImageTag, "override-image-tag", "", `Change the devcontainer underlying OCI image's tag to the specified one`)
	pr.cmd.Flags().BoolVarP(&pr.pullLatest, "pull-latest", "", false, `Change the devcontainer underlying OCI image's tag to 'latest'.`)
	pr.cmd.Flags().BoolVarP(&pr.pullGiven, "pull-given", "", false, `Ensures that the pulled image is of the given tag and not latest.`)
}

func (pr *projectRun) initSubCommands() {
	pr.subCmds = []baseCmd{}
}

func (pr *projectRun) preRunE(cmd *cobra.Command, args []string) error {
	clog.Debug("[commands.projectRun.preRunE] Start")
	defer clog.Debug("[commands.projectRun.preRunE] End")

	if err := pr.checkMutualExclusiveness(args); err != nil {
		return err
	}

	if err := pr.checkCommandSemantic(); err != nil {
		return err
	}

	projectName, errBuildProjInfo := pr.buildProjectInfo(cmd, args)
	if errBuildProjInfo != nil {
		return errBuildProjInfo
	}

	clog.Info(fmt.Sprintf("Using project '%s'.", projectName))

	if err := pr.buildProjectHostInfoAndConfig(projectName); err != nil {
		return err
	}

	if pr.setSshTunnel {
		if err := db.SetProjectSshTunnelNeeded(projectName); err != nil {
			return cerr.AppendError("Failed to update project", err)
		}
	}

	if err := pr.runSanityChecks(projectName); err != nil {
		return err
	}

	// Project is ready to be deployed. Still, ensure that no running containers
	// related to the project are up and running.
	if err := checkCandidateForRun(projectName); err != nil {
		return err
	}

	// set apply current project to configuration, in case of implicit run (usage of path or src-repo)
	// and if no context project set
	if err := pr.ensureContext(projectName); err != nil {
		return err
	}

	return nil
}

func (pr *projectRun) runE(cmd *cobra.Command, args []string) error {
	clog.Debug("[commands.projectRun.runE] Start")
	defer clog.Debug("[commands.projectRun.runE] End")

	projectName := db.GetCurrentProject()
	// TODO: Agent Service interaction needed — execute the full run orchestration:
	// - build and run the devcontainer (engine.run)
	// - build orchestration (KinD) if requested
	// - build registry if requested
	// - build SSM if requested
	// - inspect container, configure SSH, run post-install hooks, clone repo
	// Previously handled by runE() in run_utils.go which orchestrated concurrent goroutines
	// for orchestration, registry, SSM, and devcontainer build.
	clog.Info(fmt.Sprintf("Project '%s' is ready for deployment. Agent service required to proceed.", projectName))
	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute 'run' operation")
}

/************************************************************/
/*                                                          */
/*                     preRunE Helpers                      */
/*                                                          */
/************************************************************/

func (pr *projectRun) buildProjectHostInfoAndConfig(projectName string) error {
	var targetHostName string
	if len(pr.targetServer) != 0 {
		targetHostName = pr.targetServer
	}
	if targetHostName == "" {
		targetHostName = db.GetDefaultHostName()
	}

	if err := identifyProjectHost(projectName, targetHostName); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to identify host for project (%s)", projectName), err)
	}

	return nil
}

func (pr *projectRun) checkCommandSemantic() error {
	if len(pr.overrideImageTag) > 0 {
		if err := validateImageTagSyntax(pr.overrideImageTag); err != nil {
			return err
		}
	}
	return nil
}

func checkCandidateForRun(projectName string) error {
	// TODO: Agent Service interaction needed — align container statuses via engine (engine.NewContainerEngine + alignContainerStatuses)
	// For now, just check if the project already has containers configured
	containers := db.ProjectContainersName(projectName)
	if len(containers) > 0 {
		return cerr.NewError("Containers have been found to be already configured for this project." +
			"\n" + getTipClear(projectName))
	}
	return nil
}

func (pr *projectRun) checkMutualExclusiveness(args []string) error {
	if pr.path != "" && len(args) > 0 {
		return cerr.NewError("the --path option and the projectName argument are mutually exclusive")
	}
	if pr.path != "" && pr.srcRepo != "" {
		return cerr.NewError("the --path and the --src-repo options are mutually exclusive")
	}
	if pr.srcRepo != "" && len(args) > 0 {
		return cerr.NewError("the --src-repo option and the projectName argument are mutually exclusive")
	}
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

func (pr *projectRun) ensureContext(projectName string) error {
	if db.GetCurrentProject() == "" && (len(pr.path) != 0 || len(pr.srcRepo) != 0) {
		clog.Info("Empty context found. For your convenience we set", projectName, "as the new default project")
		if err := db.SetProject(projectName); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to set project (%s) for container project", projectName), err)
		}
	}
	return nil
}

func (pr *projectRun) runSanityChecks(projectName string) error {
	if err := pr.runProjectConfigSanityChecks(projectName); err != nil {
		return err
	}
	return nil
}

func (pr *projectRun) runProjectConfigSanityChecks(projectName string) error {
	// TODO: Artifactory Service interaction needed — verify if project's flavour needs update (container.RefreshFlavour)
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

func (pr *projectRun) handleImageTag(projectName string) error {
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

/************************************************************/
/*                                                          */
/*                      Scaffold                            */
/*                                                          */
/************************************************************/

func (pr *projectRun) buildProjectInfo(cmd *cobra.Command, args []string) (string, error) {
	// path is specified -> use it
	if pr.path != "" {
		return pr.handlePath()
	}

	if pr.srcRepo != "" {
		return pr.handleSrcRepo()
	}

	if err := validateProjectNameFromArgsOrContext(cmd, args); err != nil {
		return "", err
	}
	// at this point the project is all good
	projectName := getProjectNameFromArgsOrContext(args)
	return projectName, nil
}

func (pr *projectRun) handlePath() (string, error) {
	absPath, err := toAbsolute(pr.path)
	if err != nil {
		return "", cerr.AppendError("Failed to resolve path", err)
	}

	devcontainerPath := filepath.Join(absPath, db.KCdsProjectDefaultDir)

	// prevent creation of duplicated project using the same config
	if projectNames := db.GetProjectsUsingConfigDir(devcontainerPath); len(projectNames) > 0 {
		return "", cerr.NewError(fmt.Sprintf("The following projects already use this devcontainer.json configuration: %v\nAborting.", projectNames))
	}

	// TODO: derive project name from path (similar to getProjectNameFromRepoOrPath in old code)
	projectName := filepath.Base(absPath)
	if db.HasProject(projectName) {
		return "", cerr.NewError(fmt.Sprintf("Project name '%s' is already configured", projectName))
	}

	clog.Info(fmt.Sprintf("Creating project '%s' on the fly from path: %s", projectName, devcontainerPath))
	if err := db.AddProjectUsingConfDir(projectName, devcontainerPath); err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to register project at given path (%s)", absPath), err)
	}
	return projectName, nil
}

func (pr *projectRun) handleSrcRepo() (string, error) {
	// TODO: Agent Service interaction needed — parse git repository URL (net.ParseGitRepositoryUrl)
	// and handle source repo-based project creation (container.GetSourceRepository)
	return "", cerr.NewError("TODO: Agent service not yet implemented — source repo-based project creation unavailable")
}
