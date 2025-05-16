package db

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/amadeusitgroup/cds/internal/bo"
)

func Test_SetProjectHost(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		host        string
		hostWanted  string
		wantedError error
	}{
		{
			name:        "Full upper case",
			projectName: "Project1",
			host:        "MYHOST",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", Host: ""}}}},
			hostWanted:  "myhost",
			wantedError: nil,
		},
		{
			name:        "Replace host",
			projectName: "Project2",
			host:        "myhost",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2", Host: "anotherhost"}}}},
			hostWanted:  "myhost",
			wantedError: nil,
		},
		{
			name:        "Replace host on a project that doesn't exist",
			host:        "myhost",
			projectName: "Project3",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2", Host: "anotherhost"}}}},
			hostWanted:  "myhost",
			wantedError: fmt.Errorf("Failed to update project Project3"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if err := SetProjectHost(tt.projectName, tt.host); err != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.wantedError.Error()))
					return
				}
				t.Errorf("Cannot set %s host for project %s", tt.host, tt.projectName)
			}
			project, err := instance().d.getProject(tt.projectName)
			if err != nil {
				t.Errorf("Cannot get project %s", tt.projectName)
			}
			if project.Host != tt.hostWanted {
				t.Errorf("Host %s is not set correctly for project %s", tt.host, tt.projectName)
			}
		})
	}
}

func Test_SetOrchestrationRequested(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
		wantedError error
	}{
		{
			name:        "Project with orc",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", OrchestrationUsage: orchestrationUsage{Cluster: clusterUsage{Use: false}}}}}},
			want:        true,
			wantedError: nil,
		},
		{
			name:        "Project that doesn't exist",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", OrchestrationUsage: orchestrationUsage{Cluster: clusterUsage{Use: false}}}}}},
			want:        true,
			wantedError: fmt.Errorf("Failed to update project Project2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if err := SetOrchestrationRequested(tt.projectName); err != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.wantedError.Error()))
					return
				}
				t.Errorf("Cannot set orc requested for project %s", tt.projectName)
			}
			project, err := instance().d.getProject(tt.projectName)
			if err != nil {
				t.Errorf("Cannot get project %s", tt.projectName)
			}
			if project.OrchestrationUsage.Cluster.Use != tt.want {
				t.Errorf("Orchestration Usage is not set correctly for project %s", tt.projectName)
			}
		})
	}
}

func Test_SetORegistryRequested(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
		wantedError error
	}{
		{
			name:        "Project with registry",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", OrchestrationUsage: orchestrationUsage{Registry: registryUsage{Use: false}}}}}},
			want:        true,
			wantedError: nil,
		},
		{
			name:        "Project that doesn't exist",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", OrchestrationUsage: orchestrationUsage{Registry: registryUsage{Use: false}}}}}},
			want:        true,
			wantedError: fmt.Errorf("Failed to update project Project2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if err := SetProjectRegistryUsage(tt.projectName); err != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.wantedError.Error()))
					return
				}
				t.Errorf("Cannot set registry usage for project %s", tt.name)
			}
			project, err := instance().d.getProject(tt.projectName)
			if err != nil {
				t.Errorf("Cannot get project %s", tt.projectName)
			}
			if project.OrchestrationUsage.Registry.Use != tt.want {
				t.Errorf("Registry Usage is not set correctly for project %s", tt.projectName)
			}
		})
	}
}

func Test_SetProjectSshTunnelNeeded(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
		wantedError error
	}{
		{
			name:        "Project with ssh tunnel",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", UseSshTunnel: false}}}},
			want:        true,
			wantedError: nil,
		},
		{
			name:        "Project that doesn't exist",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", UseSshTunnel: false}}}},
			want:        true,
			wantedError: fmt.Errorf("Failed to update project Project2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if err := SetProjectSshTunnelNeeded(tt.projectName); err != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.wantedError.Error()))
					return
				}
				t.Errorf("Cannot set ssh tunnel usage for project %s", tt.name)
			}
			project, err := instance().d.getProject(tt.projectName)
			if err != nil {
				t.Errorf("Cannot get project %s", tt.projectName)
			}
			if project.UseSshTunnel != tt.want {
				t.Errorf("Ssh tunnel is not set correctly for project %s", tt.projectName)
			}
		})
	}
}

func Test_SetNasRequested(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
		wantedError error
	}{
		{
			name:        "Project with NAS requested",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", NasRequested: false}}}},
			want:        true,
			wantedError: nil,
		},
		{
			name:        "Project that doesn't exist",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", NasRequested: false}}}},
			want:        true,
			wantedError: fmt.Errorf("Failed to update project Project2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if err := SetNasRequested(tt.projectName); err != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.wantedError.Error()))
					return
				}
				t.Errorf("Cannot set NAS for project %s", tt.projectName)
			}
			project, err := instance().d.getProject(tt.projectName)
			if err != nil {
				t.Errorf("Cannot get project %s", tt.projectName)
			}
			if project.NasRequested != tt.want {
				t.Errorf("Nas request is not set correctly for project %s", tt.name)
			}
		})
	}
}

func Test_SetOverrideImageTag(t *testing.T) {
	tests := []struct {
		name           string
		projectName    string
		bom            data
		imageTagWanted string
		wantedError    error
	}{
		{
			name:           "Project with override image tag",
			projectName:    "Project1",
			bom:            data{projects: projects{Projects: []*project{{Name: "Project1", OverrideImageTag: ""}}}},
			imageTagWanted: "mytag",
			wantedError:    nil,
		},
		{
			name:           "Project that doesn't exist",
			projectName:    "Project2",
			bom:            data{projects: projects{Projects: []*project{{Name: "Project1", OverrideImageTag: ""}}}},
			imageTagWanted: "mytag",
			wantedError:    fmt.Errorf("Failed to update project Project2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if err := SetOverrideImageTag(tt.projectName, tt.imageTagWanted); err != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.wantedError.Error()))
					return
				}
				t.Errorf("Cannot set image tag for project %s", tt.name)
			}
			project, err := instance().d.getProject(tt.projectName)
			if err != nil {
				t.Errorf("Cannot get project %s", tt.projectName)
			}
			if project.OverrideImageTag != tt.imageTagWanted {
				t.Errorf("Image tag is not set correctly for project %s", tt.projectName)
			}
		})
	}
}

func Test_SetProjectSrcRepoInfo(t *testing.T) {
	tests := []struct {
		name                 string
		projectName          string
		bom                  data
		srcRepoUriWanted     string
		srcRepoRefWanted     string
		srcRepoToCloneWanted bool
		wantedError          error
	}{
		{
			name:                 "Project with src repo",
			projectName:          "Project1",
			bom:                  data{projects: projects{Projects: []*project{{Name: "Project1", SrcRepo: srcRepoInfo{URI: "", Ref: "", ToClone: false}}}}},
			srcRepoUriWanted:     "myrepo",
			srcRepoRefWanted:     "myref",
			srcRepoToCloneWanted: true,
			wantedError:          nil,
		},
		{
			name:        "Project with src repo",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", SrcRepo: srcRepoInfo{URI: "", Ref: "", ToClone: false}}}}},
			wantedError: fmt.Errorf("Failed to update project Project2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			if err := SetProjectSrcRepoInfo(tt.projectName, tt.srcRepoUriWanted, tt.srcRepoRefWanted, tt.srcRepoToCloneWanted); err != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.wantedError.Error()))
					return
				}
				t.Errorf("Cannot set src repo info for project %s", tt.projectName)
			}
			project, err := instance().d.getProject(tt.projectName)
			if err != nil {
				t.Errorf("Cannot get project %s", tt.projectName)
			}
			if project.SrcRepo.URI != tt.srcRepoUriWanted {
				t.Errorf("Src Repo Uri is not set correctly for project %s", tt.projectName)
			}
			if project.SrcRepo.Ref != tt.srcRepoRefWanted {
				t.Errorf("Src Repo Ref is not set correctly for project %s", tt.projectName)
			}
			if project.SrcRepo.ToClone != tt.srcRepoToCloneWanted {
				t.Errorf("Src Repo to be cloned is not set correctly for project %s", tt.projectName)
			}

		})
	}
}

func Test_ContainerSSHPort(t *testing.T) {
	tests := []struct {
		name            string
		projectName     string
		path            string
		bom             data
		wantedContainer string
		wantedPort      int
	}{
		{
			name:            "Project with dummy container",
			projectName:     "Project1",
			wantedContainer: "dummy",
			bom:             data{projects: projects{Projects: []*project{{Name: "Project1", Containers: []*containerInfo{{Name: "dummy", PortSSH: 1000}}}}}},
			wantedPort:      1000,
		},
		{
			name:            "Container that doesn't exist",
			projectName:     "Project1",
			wantedContainer: "dummy2",
			bom:             data{projects: projects{Projects: []*project{{Name: "Project1", Containers: []*containerInfo{{Name: "dummy", PortSSH: 1000}}}}}},
			wantedPort:      nilPort,
		},
		{
			name:            "Project that doesn't exist",
			projectName:     "Project2",
			wantedContainer: "dummy1",
			bom:             data{projects: projects{Projects: []*project{{Name: "Project1", Containers: []*containerInfo{{Name: "dummy", PortSSH: 1000}}}}}},
			wantedPort:      nilPort,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actualPort := ContainerSSHPort(tt.projectName, tt.wantedContainer)

			if actualPort != tt.wantedPort {
				t.Errorf("Cannot get the wanted port %d for project %s", tt.wantedPort, tt.projectName)
			}
		})
	}
}

func Test_AddContainerInfo(t *testing.T) {
	tests := []struct {
		name            string
		projectName     string
		bom             data
		wantedContainer bo.Container
		wantedError     error
	}{
		{
			name:            "Add container info",
			projectName:     "Project 1",
			bom:             data{projects: projects{Projects: []*project{{Name: "Project 1", Containers: []*containerInfo{}}}}},
			wantedContainer: bo.Container{Id: "dummy", Name: "dummy", Status: bo.KContainerStatusUnknown, ExpectedStatus: bo.KContainerStatusUnknown, RemoteUser: "", User: "", Pmapping: map[string]int{bo.KSSHPortMapping: 22}},
			wantedError:     nil,
		},
		{
			name:            "Add container info with a container that already exists",
			projectName:     "Project 1",
			bom:             data{projects: projects{Projects: []*project{{Name: "Project 1", Containers: []*containerInfo{{Name: "dummy"}}}}}},
			wantedContainer: bo.Container{Name: "dummy"},
			wantedError:     nil,
		},
		{
			name:            "Add container info to a project that doesn't exist",
			projectName:     "Project 2",
			bom:             data{projects: projects{Projects: []*project{{Name: "Project 1", Containers: []*containerInfo{{Name: "dummy"}}}}}},
			wantedContainer: bo.Container{},
			wantedError:     fmt.Errorf("Failed to update project Project 2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			err := AddContainerInfo(tt.projectName, tt.wantedContainer)
			if err != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.wantedError.Error()))
					return
				}
				t.Fatalf("Cannot add container info for project %s. Err: %s", tt.name, err)
			}

			project, err2 := instance().d.getProject(tt.projectName)
			if err2 != nil {
				t.Errorf("Cannot get project %s", tt.name)
			}

			if len(project.Containers) != 1 {
				t.Errorf("Cannot add container info for project %s", tt.name)
			}
		})
	}
}

func Test_RemoveProject(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		path        string
		bom         data
	}{
		{
			name:        "Delete dummy project with Flavour",
			projectName: "Dummy",
			path:        "/dummy/config",
			bom:         data{projects: projects{Projects: []*project{{Name: "Dummy", Flavour: flavourInfo{LocalConfDir: "/dummy/config"}}}}},
		},
		{
			name:        "Delete dummy project with SrcRepo",
			projectName: "Dummy",
			path:        "/dummy/config",
			bom:         data{projects: projects{Projects: []*project{{Name: "Dummy", SrcRepo: srcRepoInfo{LocalConfDir: "/dummy/config"}}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			RemoveProject(tt.projectName)

			if len(instance().d.Projects) != 0 {
				t.Errorf("Project %s is not removed from list", tt.projectName)
			}
		})
	}
}

func Test_RemoveHostAndContainersFromProject(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		wantedError error
	}{
		{
			name:        "Remove Host And Containers from a dummy project",
			projectName: "Dummy",
			bom:         data{projects: projects{Projects: []*project{{Name: "Dummy", Host: "dummy host", Containers: []*containerInfo{{Name: "dummy", PortSSH: 1000}}}}}},
			wantedError: nil,
		},
		{
			name:        "Remove Host And Containers from a project that doesnt exist",
			projectName: "DummyThatDoesNotExist",
			bom:         data{projects: projects{Projects: []*project{{Name: "Dummy", Host: "dummy host", Containers: []*containerInfo{{Name: "dummy", PortSSH: 1000}}}}}},
			wantedError: fmt.Errorf("Failed to update project DummyThatDoesNotExist"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			errRemove := RemoveHostAndContainersFromProject(tt.projectName)
			if errRemove != nil {
				if tt.wantedError != nil {
					assert.True(t, strings.Contains(errRemove.Error(), tt.wantedError.Error()))
					return
				}
				t.Fatalf("Cannot remove host and containers from project project %s. Err: %s", tt.name, errRemove)
			}

			project, errProj := instance().d.getProject(tt.projectName)
			if errProj != nil {
				t.Errorf("Cannot get project %s", tt.name)
			}

			if project.Host != "" {
				t.Errorf("Project %s host is not drained", tt.projectName)
			}

			if len(project.Containers) != 0 {
				t.Errorf("Project %s containers are not removed", tt.projectName)
			}

		})
	}
}

func Test_ProjectConfig(t *testing.T) {
	tests := []struct {
		name             string
		projectName      string
		wantedConfigPath string
		bom              data
	}{
		{
			name:             " project config with dummy project set with Conf dir ",
			projectName:      "Dummy 1",
			wantedConfigPath: "/dummy1/config",
			bom:              data{projects: projects{Projects: []*project{{Name: "Dummy 1", ConfDir: "/dummy1/config"}}}},
		},
		{
			name:             " project config with dummy project set with Src Repo",
			projectName:      "Dummy 2",
			wantedConfigPath: "/dummy2/config",
			bom:              data{projects: projects{Projects: []*project{{Name: "Dummy 2", SrcRepo: srcRepoInfo{LocalConfDir: "/dummy2/config"}}}}},
		},
		{
			name:             " project config with dummy project set with Flavour",
			projectName:      "Dummy 3",
			wantedConfigPath: "/dummy3/config",
			bom:              data{projects: projects{Projects: []*project{{Name: "Dummy 3", Flavour: flavourInfo{LocalConfDir: "/dummy3/config"}}}}},
		},
		{
			name:             "project that doesn't exist",
			projectName:      "Dummy 4",
			wantedConfigPath: "",
			bom:              data{projects: projects{Projects: []*project{{Name: "Dummy 3", Flavour: flavourInfo{LocalConfDir: "/dummy3/config"}}}}},
		},
		{
			name:             "project with nothing",
			projectName:      "Dummy 1",
			wantedConfigPath: "",
			bom:              data{projects: projects{Projects: []*project{{Name: "Dummy 1"}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			configFile := ProjectConfig(tt.projectName)
			if configFile != tt.wantedConfigPath {
				t.Errorf("Cannot get the wanted config path %s for project %s", tt.wantedConfigPath, tt.projectName)
			}

		})
	}
}

func Test_ProjectContainersName(t *testing.T) {
	tests := []struct {
		name                 string
		projectName          string
		wantedContainerNames []string
		bom                  data
	}{
		{
			name:                 "Project with dummy containers",
			projectName:          "Project1",
			bom:                  data{projects: projects{Projects: []*project{{Name: "Project1", Containers: []*containerInfo{{Name: "dummy"}, {Name: "dummy2"}, {Name: "dummy3"}}}}}},
			wantedContainerNames: []string{"dummy", "dummy2", "dummy3"},
		},
		{
			name:                 "Project that doesn't exist",
			projectName:          "Project2",
			bom:                  data{projects: projects{Projects: []*project{{Name: "Project1", Containers: []*containerInfo{{Name: "dummy"}, {Name: "dummy2"}, {Name: "dummy3"}}}}}},
			wantedContainerNames: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actualContainerNames := ProjectContainersName(tt.projectName)
			fmt.Println(actualContainerNames)

			if !slices.Equal(actualContainerNames, tt.wantedContainerNames) {
				t.Errorf("Cannot get the wanted container names for test %s", tt.name)
			}
		})
	}
}

func Test_ProjectContainersUser(t *testing.T) {
	tests := []struct {
		name             string
		projectName      string
		wantedRemoteUser string
		bom              data
		containerName    string
	}{
		{
			name:             "Project with dummy container",
			projectName:      "Project1",
			containerName:    "dummy",
			bom:              data{projects: projects{Projects: []*project{{Name: "Project1", Containers: []*containerInfo{{Name: "dummy", RemoteUser: "test"}}}}}},
			wantedRemoteUser: "test",
		},
		{
			name:             "Container that doesn't exist",
			projectName:      "Project1",
			containerName:    "dummy1",
			bom:              data{projects: projects{Projects: []*project{{Name: "Project1", Containers: []*containerInfo{{Name: "dummy", RemoteUser: "test"}}}}}},
			wantedRemoteUser: "",
		},
		{
			name:             "Project that doesn't exist",
			projectName:      "Project2",
			bom:              data{projects: projects{Projects: []*project{{Name: "Project1", Containers: []*containerInfo{{Name: "dummy", RemoteUser: "test"}}}}}},
			wantedRemoteUser: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actualRemoteUser := ProjectContainerRemoteUser(tt.projectName, tt.containerName)
			if actualRemoteUser != tt.wantedRemoteUser {
				t.Errorf("Cannot get the wanted remote user %s for test %s", tt.wantedRemoteUser, tt.name)
			}
		})
	}
}

func Test_removeProjectFromList(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
	}{
		{
			name:        "Remove project 1 from list",
			projectName: "Project 1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project 1"}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			instance().d.removeProjectFromList(tt.projectName)
			if len(instance().d.Projects) != 0 {
				t.Errorf("Project %s is not removed from list", tt.projectName)
			}
		})
	}
}

func Test_IsSshTunnelNeeded(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
	}{
		{
			name:        "Project with SSH tunnel needed",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", UseSshTunnel: true}}}},
			want:        true,
		},
		{
			name:        "Project without SSH tunnel needed",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2", UseSshTunnel: false}}}},
			want:        false,
		},
		{
			name:        "Project that doesn't exist",
			projectName: "Project3",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2", UseSshTunnel: false}}}},
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := IsSshTunnelNeeded(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_IsSshTunnelNeeded --> actual = %v, want %v", actual, tt.want)
			}
		})
	}
}

func Test_ProjectSrcRepoInfo(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bo.SrcRepoInfo
	}{
		{
			name:        "Project with src repo info",
			projectName: "Project1",
			bom: data{
				projects: projects{
					Projects: []*project{
						{
							Name: "Project1",
							SrcRepo: srcRepoInfo{
								URI:     "myrepo",
								Ref:     "myref",
								ToClone: true,
							},
						},
					},
				},
			},
			want: bo.SrcRepoInfo{
				RepoURI: "myrepo",
				RepoRef: "myref",
				ToClone: true,
			},
		},
		{
			name:        "Project that doesn't exist",
			projectName: "Project2",
			bom:         data{projects: projects{}},
			want:        bo.SrcRepoInfo{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actualSrcRepo := ProjectSrcRepoInfo(tt.projectName)

			if !reflect.DeepEqual(actualSrcRepo, tt.want) {
				t.Errorf("Test_ProjectSrcRepoInfo --> actual = %v, want %v", actualSrcRepo, tt.want)
			}
		})
	}
}

func Test_IsProjectConfiguredWithFlavour(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
	}{
		{
			name:        "Project with flavour",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", Flavour: flavourInfo{Name: "Flavour1"}}}}},
			want:        true,
		},
		{
			name:        "Project without flavour",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2"}}}},
			want:        false,
		},
		{
			name:        "Project that doesn't exit",
			projectName: "Project3",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2"}}}},
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := IsProjectConfiguredWithFlavour(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_IsProjectConfiguredWithFlavour -->  actual = %v, want %v", actual, tt.want)
			}
		})
	}
}

func Test_ProjectFlavourName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        string
	}{
		{
			name:        "Project with flavour",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", Flavour: flavourInfo{Name: "Flavour1"}}}}},
			want:        "Flavour1",
		},
		{
			name:        "Project without flavour",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2"}}}},
			want:        "",
		},
		{
			name:        "Project that doesn't exit",
			projectName: "Project3",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2"}}}},
			want:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := ProjectFlavourName(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_ProjectFlavourName --> actual = %v, want %v", actual, tt.want)
			}
		})
	}
}

func Test_ProjectHostName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        string
	}{
		{
			name:        "Project with host name",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", Host: "myhost"}}}},
			want:        "myhost",
		},
		{
			name:        "Project without host name",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2", Host: ""}}}},
			want:        "",
		},
		{
			name:        "Project that doesnt exist",
			projectName: "Project3",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2", Host: ""}}}},
			want:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := ProjectHostName(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_ProjectHostName --> actual = %v, want %v", actual, tt.want)
			}
		})
	}
}

func Test_HasProjectSrcRepoToBeCloned(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
	}{
		{
			name:        "Project with src repo to be cloned",
			projectName: "Project1",
			bom: data{
				projects: projects{
					Projects: []*project{
						{
							Name: "Project1",
							SrcRepo: srcRepoInfo{
								ToClone: true,
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name:        "Project without src repo to be cloned",
			projectName: "Project2",
			bom: data{
				projects: projects{
					Projects: []*project{
						{
							Name: "Project2",
							SrcRepo: srcRepoInfo{
								ToClone: false,
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name:        "Project that doesn't exit",
			projectName: "Project3",
			bom: data{
				projects: projects{
					Projects: []*project{
						{
							Name: "Project2",
							SrcRepo: srcRepoInfo{
								ToClone: false,
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := HasProjectSrcRepoToBeCloned(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_HasProjectSrcRepoToBeCloned --> actual = %v, want %v", actual, tt.want)
			}
		})
	}
}
func Test_IsNasRequested(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
	}{
		{
			name:        "Project with NAS requested",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", NasRequested: true}}}},
			want:        true,
		},
		{
			name:        "Project that doesn't exist",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", NasRequested: true}}}},
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := IsNasRequested(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_IsNasRequested --> actual = %v, want %v", actual, tt.want)
			}
		})
	}
}

func Test_OverrideImageTag(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        string
	}{
		{
			name:        "Project with override image tag",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", OverrideImageTag: "mytag"}}}},
			want:        "mytag",
		},
		{
			name:        "Project that doesn'exist",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", OverrideImageTag: "mytag"}}}},
			want:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := OverrideImageTag(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_OverrideImageTag --> actual = %v, want %v", actual, tt.want)
			}
		})
	}
}
func Test_IsOrchestrationUsed(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
	}{
		{
			name:        "Project with orchestration used",
			projectName: "Project1",
			bom: data{
				projects: projects{
					Projects: []*project{
						{
							Name: "Project1",
							OrchestrationUsage: orchestrationUsage{
								Cluster: clusterUsage{
									Use: true,
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name:        "Project without orchestration used",
			projectName: "Project2",
			bom: data{
				projects: projects{
					Projects: []*project{
						{
							Name: "Project2",
							OrchestrationUsage: orchestrationUsage{
								Cluster: clusterUsage{
									Use: false,
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name:        "Project that doens't exist",
			projectName: "Project3",
			bom: data{
				projects: projects{
					Projects: []*project{
						{
							Name: "Project2",
							OrchestrationUsage: orchestrationUsage{
								Cluster: clusterUsage{
									Use: false,
								},
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := IsOrchestrationUsed(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_IsOrchestrationUsed --> actual = %v, want %v", actual, tt.want)
			}
		})
	}
}
func Test_IsRegistryUsed(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		bom         data
		want        bool
	}{
		{
			name:        "Project with registry usage",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", OrchestrationUsage: orchestrationUsage{Registry: registryUsage{Use: true}}}}}},
			want:        true,
		},
		{
			name:        "Project without registry usage",
			projectName: "Project2",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2", OrchestrationUsage: orchestrationUsage{Registry: registryUsage{Use: false}}}}}},
			want:        false,
		},
		{
			name:        "Project that doesn't exist",
			projectName: "Project3",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project2", OrchestrationUsage: orchestrationUsage{Registry: registryUsage{Use: false}}}}}},
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			actual := IsRegistryUsed(tt.projectName)

			if actual != tt.want {
				t.Errorf("Test_IsRegistryUsed ---> actual = %v, want %v", actual, tt.want)
			}
		})
	}
}
func Test_ProjectsOrchestrationUsage(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		wantUsage   bo.OrchestrationUsage
		bom         data
	}{
		{
			name:        "Project with orchestration and registry usage",
			projectName: "Project1",
			bom:         data{projects: projects{Projects: []*project{{Name: "Project1", OrchestrationUsage: orchestrationUsage{Cluster: clusterUsage{Use: true}, Registry: registryUsage{Use: true}}}}}},
			wantUsage:   bo.OrchestrationUsage{Cluster: bo.ClusterUsage{Use: true}, Registry: bo.RegistryUsage{Use: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			actual := ProjectsOrchestrationUsage(tt.projectName)
			if !reflect.DeepEqual(actual, tt.wantUsage) {
				t.Errorf("Test_ProjectsOrchestrationUsage --> actual = %v, want %v", actual, tt.wantUsage)
			}
		})
	}
}
