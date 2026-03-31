package command

import (
	_ "embed"
	"fmt"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

const (
	kNbArgsProjectNameOnly = 1
	kImageTagRegex         = `^[\w][\w.-]{0,127}$`
	// containerNamesRegex is the naming convention regex for project/container names.
	containerNamesRegex = `^([a-zA-Z0-9][a-zA-Z0-9_.-]*)$`
)

// isValidProjectName checks whether the given project name is valid per naming conventions.
func isValidProjectName(projectName string) (bool, error) {
	clog.Debug("Validate project name")
	defer clog.Debug("Validate project name done")

	matchRes, err := regexp.MatchString(containerNamesRegex, projectName)
	if err != nil {
		return false, cerr.AppendError("Failed to evaluate regex while checking project name validity", err)
	}
	return matchRes, nil
}

// validateProjectNameFromArgsOrContext validates that a single PROJECT-NAME positional arg is given
// or falls back to the current project context. Used with cobra's Args field.
func validateProjectNameFromArgsOrContext(cmd *cobra.Command, args []string) error {
	var projectName string
	if len(args) == 0 {
		projectName = db.GetCurrentProject()
		if len(projectName) == 0 {
			clog.Info("Tip: You can list projects with 'cds project list' and set context using 'cds project use'")
			return cerr.NewError("Could not identify project to run the command. No project is set!")
		}
	} else if len(args) > kNbArgsProjectNameOnly {
		if err := cmd.Help(); err != nil {
			return cerr.AppendError("Incorrect command usage, CDS also failed to display usage", err)
		}
		return cerr.NewError("Incorrect command usage, expected only project name as the one argument of the command!")
	} else {
		projectName = args[0]
	}

	if !db.HasProject(projectName) {
		clog.Info("Tip: You can list projects with 'cds project list'")
		return cerr.NewError(fmt.Sprintf("Project '%s' is not defined in cds configuration!", projectName))
	}

	return nil
}

// getProjectNameFromArgsOrContext returns the project name from args or the current context.
// Should be called after validateProjectNameFromArgsOrContext has run.
func getProjectNameFromArgsOrContext(args []string) string {
	if len(args) == kNbArgsProjectNameOnly {
		return args[0]
	}
	return db.GetCurrentProject()
}

// validateImageTagSyntax checks whether the given image tag matches the expected regex format.
func validateImageTagSyntax(imageTag string) error {
	imageTagReg := regexp.MustCompile(kImageTagRegex)
	if !imageTagReg.MatchString(imageTag) {
		return cerr.NewError(fmt.Sprintf("Failed to override image tag. Tag doesn't match corresponding regexp: %s", imageTag))
	}
	return nil
}

// completionProject provides shell completion for project names.
func completionProject(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	projectNames := db.ListProjects()
	completion := make([]string, len(projectNames))
	for i, name := range projectNames {
		completion[i] = fmt.Sprintf("%s\n", name)
	}
	return completion, cobra.ShellCompDirectiveNoFileComp
}

// confInUse checks if the given configuration directory is already in use by another project.
func confInUse(path string) bool {
	projectNames := db.GetProjectsUsingConfigDir(path)
	if len(projectNames) != 0 {
		clog.Warn(fmt.Sprintf(`Configuration dir "%v" is currently used by: "%v"`, path, projectNames))
		return true
	}
	return false
}
