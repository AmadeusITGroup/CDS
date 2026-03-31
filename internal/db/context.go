package db

// GetCurrentProject returns the name of the currently selected project.
func GetCurrentProject() string {
	instance().Lock()
	defer instance().Unlock()
	return instance().d.Context.ProjectContext
}

// SetProject sets the currently selected project context.
func SetProject(projectName string) error {
	instance().Lock()
	defer instance().Unlock()
	// Verify the project exists
	if _, err := instance().d.getProject(projectName); err != nil {
		return err
	}
	instance().d.Context.ProjectContext = projectName
	return nil
}

// IsCurrentProject returns true if the given project name is the currently selected project.
func IsCurrentProject(projectName string) bool {
	return GetCurrentProject() == projectName
}
