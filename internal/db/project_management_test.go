package db

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
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

func Test_GetProjectInfo_Basic(t *testing.T) {
	bom := data{
		Context: context{ProjectContext: "proj1"},
		projects: projects{Projects: []*project{
			{
				Name: "proj1",
				Host: "myhost.example.com",
				Containers: []*containerInfo{
					{Id: "abc123", Name: "web", State: "running", ExpectedState: "running", PortSSH: 2222, RemoteUser: "dev"},
					{Id: "def456", Name: "db", State: "running", ExpectedState: "running", PortSSH: 2223, RemoteUser: "root"},
				},
				OrchestrationUsage: orchestrationUsage{
					Cluster:  clusterUsage{Use: true},
					Registry: registryUsage{Use: false},
				},
			},
		}},
	}
	tearDown := setupTest(t, bom)
	defer tearDown()

	info := GetProjectInfo("proj1")

	assert.Equal(t, "proj1", info.Name)
	assert.Equal(t, "myhost.example.com", info.Host)
	assert.True(t, info.Current)
	assert.Equal(t, "running", info.Status)
	assert.Len(t, info.Containers, 2)
	assert.Equal(t, "abc123", info.Containers[0].Id)
	assert.Equal(t, "web", info.Containers[0].Name)
	assert.Equal(t, "running", info.Containers[0].Status)
	assert.True(t, info.OrchestrationUsage.Cluster.Use)
	assert.False(t, info.OrchestrationUsage.Registry.Use)
}

func Test_GetProjectInfo_NotCurrent(t *testing.T) {
	bom := data{
		Context: context{ProjectContext: "other"},
		projects: projects{Projects: []*project{
			{Name: "proj1"},
			{Name: "other"},
		}},
	}
	tearDown := setupTest(t, bom)
	defer tearDown()

	info := GetProjectInfo("proj1")

	assert.Equal(t, "proj1", info.Name)
	assert.False(t, info.Current)
}

func Test_GetProjectInfo_EmptyContainers(t *testing.T) {
	bom := data{
		projects: projects{Projects: []*project{
			{Name: "proj1", Containers: []*containerInfo{}},
		}},
	}
	tearDown := setupTest(t, bom)
	defer tearDown()

	info := GetProjectInfo("proj1")

	assert.Equal(t, "empty", info.Status)
	assert.Empty(t, info.Containers)
}

func Test_GetProjectInfo_NonExistent(t *testing.T) {
	bom := data{projects: projects{Projects: []*project{}}}
	tearDown := setupTest(t, bom)
	defer tearDown()

	info := GetProjectInfo("ghost")

	assert.Equal(t, "ghost", info.Name)
	assert.Equal(t, bo.ProjectInfo{Name: "ghost"}, info)
}

func Test_GetProjectInfo_MixedContainerStates(t *testing.T) {
	bom := data{
		projects: projects{Projects: []*project{
			{
				Name: "proj1",
				Containers: []*containerInfo{
					{Id: "1", Name: "c1", State: "running"},
					{Id: "2", Name: "c2", State: "exited"},
				},
			},
		}},
	}
	tearDown := setupTest(t, bom)
	defer tearDown()

	info := GetProjectInfo("proj1")

	assert.Equal(t, "unknown", info.Status)
	assert.Len(t, info.Containers, 2)
}
