package db

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

const (
	// KDefaultProjectName is the default name given to a project when none is specified.
	KDefaultProjectName = "default"
	// KCdsProjectDefaultDir is the default directory name used for devcontainer configuration.
	KCdsProjectDefaultDir = ".devcontainer"
)

// HasProject returns true if the project exists in the database.
func HasProject(projectName string) bool {
	instance().Lock()
	defer instance().Unlock()
	_, err := instance().d.getProject(projectName)
	return err == nil
}

// ListProjects returns the names of all registered projects.
func ListProjects() []string {
	instance().Lock()
	defer instance().Unlock()
	names := make([]string, 0, len(instance().d.Projects))
	for _, p := range instance().d.Projects {
		names = append(names, p.Name)
	}
	return names
}

// AddProjectUsingConfDir registers a new project that uses an existing configuration directory.
func AddProjectUsingConfDir(projectName, confDir string) error {
	instance().Lock()
	defer instance().Unlock()

	// Verify project doesn't already exist
	if _, err := instance().d.getProject(projectName); err == nil {
		return cerr.NewError(fmt.Sprintf("Project '%s' already exists", projectName))
	}

	newProject := &project{
		Name:    projectName,
		ConfDir: confDir,
	}
	instance().d.Projects = append(instance().d.Projects, newProject)
	clog.Info(fmt.Sprintf("Project '%s' added using config dir: %s", projectName, confDir))
	return nil
}

// AddProjectUsingFlavour registers a new project that uses a standard Artifactory flavour.
func AddProjectUsingFlavour(projectName, flavourName, overrideDir string) error {
	instance().Lock()
	defer instance().Unlock()

	// Verify project doesn't already exist
	if _, err := instance().d.getProject(projectName); err == nil {
		return cerr.NewError(fmt.Sprintf("Project '%s' already exists", projectName))
	}

	newProject := &project{
		Name: projectName,
		Flavour: flavourInfo{
			Name:        flavourName,
			OverrideDir: overrideDir,
		},
	}
	instance().d.Projects = append(instance().d.Projects, newProject)
	clog.Info(fmt.Sprintf("Project '%s' added using flavour: %s", projectName, flavourName))
	return nil
}

// SetFlavourLocalConfDir updates the local configuration directory for a flavour-based project.
func SetFlavourLocalConfDir(projectName, localConfDir string) error {
	var fn decorateProject = func(p *project) {
		p.Flavour.LocalConfDir = localConfDir
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update flavour local conf dir for project %s", err, projectName)
	}
	return nil
}

// GetProjectsUsingConfigDir returns project names that reference the given config dir path.
func GetProjectsUsingConfigDir(path string) []string {
	instance().Lock()
	defer instance().Unlock()
	var names []string
	for _, p := range instance().d.Projects {
		if p.ConfDir == path {
			names = append(names, p.Name)
		}
		if p.Flavour.LocalConfDir == path {
			names = append(names, p.Name)
		}
	}
	return slices.Compact(names)
}

// BuildFlavourLocalConfDir creates the local configuration directory for a flavour-based project.
// TODO: Requires interaction with the Artifactory service to download and prepare the config.
func BuildFlavourLocalConfDir(projectName, flavourPath, overrideDir string) error {
	// The local conf dir path is derived from the CDS config directory
	localConfDir := filepath.Join(flavourPath, KCdsProjectDefaultDir)
	return SetFlavourLocalConfDir(projectName, localConfDir)
}
