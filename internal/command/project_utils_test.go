package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
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


