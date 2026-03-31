package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetCurrentProject_Empty(t *testing.T) {
	tearDown := setupTest(t, data{})
	defer tearDown()

	actual := GetCurrentProject()
	assert.Equal(t, "", actual)
}

func Test_SetAndGetCurrentProject(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{{Name: "myproject"}}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	err := SetProject("myproject")
	assert.Nil(t, err)

	actual := GetCurrentProject()
	assert.Equal(t, "myproject", actual)
}

func Test_SetProject_NotExists(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{{Name: "myproject"}}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	err := SetProject("nonexistent")
	assert.Error(t, err)
}

func Test_IsCurrentProject(t *testing.T) {
	bom := data{
		Context:  context{ProjectContext: "proj1"},
		projects: projects{Projects: []*project{{Name: "proj1"}, {Name: "proj2"}}},
	}
	tearDown := setupTest(t, bom)
	defer tearDown()

	assert.True(t, IsCurrentProject("proj1"))
	assert.False(t, IsCurrentProject("proj2"))
}

func Test_HasProject(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{{Name: "proj1"}}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	assert.True(t, HasProject("proj1"))
	assert.False(t, HasProject("nonexistent"))
}

func Test_ListProjects(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{{Name: "p1"}, {Name: "p2"}, {Name: "p3"}}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	names := ListProjects()
	assert.Equal(t, []string{"p1", "p2", "p3"}, names)
}

func Test_ListProjects_Empty(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	names := ListProjects()
	assert.Equal(t, []string{}, names)
}

func Test_AddProjectUsingConfDir(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	err := AddProjectUsingConfDir("newproj", "/path/to/.devcontainer")
	assert.Nil(t, err)
	assert.True(t, HasProject("newproj"))

	confDir := ProjectConfig("newproj")
	assert.Equal(t, "/path/to/.devcontainer", confDir)
}

func Test_AddProjectUsingConfDir_AlreadyExists(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{{Name: "existing"}}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	err := AddProjectUsingConfDir("existing", "/path")
	assert.Error(t, err)
}

func Test_AddProjectUsingFlavour(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	err := AddProjectUsingFlavour("flproj", "my-flavour", "/override")
	assert.Nil(t, err)
	assert.True(t, HasProject("flproj"))
	assert.True(t, IsProjectConfiguredWithFlavour("flproj"))
	assert.Equal(t, "my-flavour", ProjectFlavourName("flproj"))
}

func Test_AddProjectUsingFlavour_AlreadyExists(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{{Name: "existing"}}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	err := AddProjectUsingFlavour("existing", "flavour", "")
	assert.Error(t, err)
}
