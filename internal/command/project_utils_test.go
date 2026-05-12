package command

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateImageTagSyntax(t *testing.T) {
	inputTags := []string{"toto", "1.1.1", "toto-1.1", "1:1", "-1.2", "toto@123"}
	isTagValid := []bool{true, true, true, false, false, false}

	for index, tag := range inputTags {
		err := validateImageTagSyntax(tag)
		if isTagValid[index] {
			assert.Nil(t, err, "Tag %s should be valid", tag)
		} else {
			assert.Error(t, err, "Tag %s should be invalid", tag)
		}
	}
}

func TestIsValidProjectName(t *testing.T) {
	validProjects := []string{"myProject", "my-project", "my_project", "myproject", "myproject1", "my-project1", "my_project1"}
	for _, project := range validProjects {
		valid, err := isValidProjectName(project)
		assert.Nil(t, err, "Error while validating project name %s", project)
		assert.True(t, valid, "Project name %s should be valid", project)
	}

	invalidProjects := []string{"project+", "project$", "project*", "project&", "project#", "project!", "project@", "my/project", "{project}"}
	for _, project := range invalidProjects {
		valid, err := isValidProjectName(project)
		assert.Nil(t, err, "Error while validating project name %s", project)
		assert.False(t, valid, "Project name %s should be invalid", project)
	}
}

func TestIsValidProjectNameEmpty(t *testing.T) {
	valid, err := isValidProjectName("")
	assert.Nil(t, err)
	assert.False(t, valid)
}

func TestProjectNameFromArgsOrContextFallsBackToConfiguredDefaultProject(t *testing.T) {
	setupDefaultProjectWithoutContext(t)

	assert.Equal(t, db.KDefaultProjectName, getProjectNameFromArgsOrContext(nil))
	require.NoError(t, validateProjectNameFromArgsOrContext(&cobra.Command{}, nil))
}

func TestGetTipContainerSSHIncludesCopyPasteCommand(t *testing.T) {
	assert.Equal(t, "ssh default-app-host", getContainerSSHCommand("default-app-host"))
	assert.Equal(t, "  To connect, run: ssh default-app-host", getTipContainerSSH("default-app-host"))
}

func setupDefaultProjectWithoutContext(t *testing.T) {
	t.Helper()

	db.RemoveProject(db.KDefaultProjectName)
	require.NoError(t, db.FlushContext())
	require.NoError(t, db.AddProjectUsingConfDir(db.KDefaultProjectName, t.TempDir()))
	require.NoError(t, db.FlushContext())
	t.Cleanup(func() {
		db.RemoveProject(db.KDefaultProjectName)
		_ = db.FlushContext()
	})
}
