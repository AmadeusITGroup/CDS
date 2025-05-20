package db

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/stretchr/testify/assert"
)

func TestListHostNames(t *testing.T) {
	tests := []struct {
		name            string
		bom             data
		hostNamesWanted []string
	}{
		{
			name:            "No Host added in Config",
			bom:             data{projects: projects{Projects: []*project{{Name: "Project1", Host: ""}}}},
			hostNamesWanted: []string{},
		},
		{
			name:            "Multiple host added in Config",
			bom:             data{hosts: hosts{Hosts: []*host{{Name: "myHost1", InUse: true}, {Name: "myHost2"}}}},
			hostNamesWanted: []string{"myHost1", "myHost2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()
			hostList := ListHostNames()
			assert.ElementsMatch(t, hostList, tt.hostNamesWanted)
		})
	}
}

func TestAddHost(t *testing.T) {
	tests := []struct {
		name         string
		initialData  data
		newHostName  string
		newHostUser  string
		expectedHost *host
	}{
		{
			name:         "Add new host on empty config",
			initialData:  data{hosts: hosts{Hosts: []*host{}}},
			newHostName:  "host1",
			newHostUser:  "user1",
			expectedHost: &host{Name: "host1", sshInfo: sshInfo{Username: "user1"}},
		},
		{
			name:         "Add new host on config with multiple hosts",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			newHostName:  "host3",
			newHostUser:  "user3",
			expectedHost: &host{Name: "host3", sshInfo: sshInfo{Username: "user3"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			AddHost(tt.newHostName, tt.newHostUser)
			host, err := instance().d.getHost(tt.newHostName)
			if err != nil {
				t.Errorf("Cannot get host %s", tt.newHostName)
			}
			assert.EqualValues(t, tt.expectedHost, host)
		})
	}
}

func TestRemoveHostFromHostList(t *testing.T) { //TODO: Rename to 'RemoveHost' once 'RemoveHostFromConfig' has been removed.
	tests := []struct {
		name         string
		initialData  data
		hostToRemove string
		expectedData data
	}{
		{
			name:         "Remove existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}, {Name: "host3"}}}},
			hostToRemove: "host1",
			expectedData: data{hosts: hosts{Hosts: []*host{{Name: "host2"}, {Name: "host3"}}}},
		},
		{
			name:         "Remove non-existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1"}}}},
			hostToRemove: "host2",
			expectedData: data{hosts: hosts{Hosts: []*host{{Name: "host1"}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			RemoveHostFromHostList(tt.hostToRemove)
			hostList := instance().d.getHostList()

			//convert slice of pointers to slice of values for comparison
			actualHosts := make([]host, len(hostList.Hosts))
			for i, h := range hostList.Hosts {
				actualHosts[i] = *h
			}

			expectedHosts := make([]host, len(tt.expectedData.Hosts))
			for i, h := range tt.expectedData.Hosts {
				expectedHosts[i] = *h
			}
			assert.EqualValues(t, expectedHosts, actualHosts)
		})
	}
}

func TestGetDefaultHostName(t *testing.T) {
	tests := []struct {
		name             string
		initialData      data
		expectedHostName string
	}{
		{
			name:             "Default host exists",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1", IsDefault: true}, {Name: "host2"}}}},
			expectedHostName: "host1",
		},
		{
			name:             "No default host",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2", IsDefault: false}}}},
			expectedHostName: "",
		},
		{
			name:             "Multiple hosts with one default",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1", IsDefault: false}, {Name: "host2", IsDefault: true}}}},
			expectedHostName: "host2",
		},
		{
			name:             "Empty host list",
			initialData:      data{hosts: hosts{Hosts: []*host{}}},
			expectedHostName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			defaultHostName := GetDefaultHostName()
			assert.Equal(t, tt.expectedHostName, defaultHostName)
		})
	}
}

func TestSetHostToDefault(t *testing.T) {
	tests := []struct {
		name             string
		initialData      data
		hostToSetDefault string
		expectedDefault  string
	}{
		{
			name:             "Set existing host as default",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			hostToSetDefault: "host1",
			expectedDefault:  "host1",
		},
		{
			name:             "Change default host",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1", IsDefault: true}, {Name: "host2"}}}},
			hostToSetDefault: "host2",
			expectedDefault:  "host2",
		},
		{
			name:             "Set non-existing host as default",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			hostToSetDefault: "host3",
			expectedDefault:  "",
		},
		{
			name:             "Set default on empty host list",
			initialData:      data{hosts: hosts{Hosts: []*host{}}},
			hostToSetDefault: "host1",
			expectedDefault:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			SetHostToDefault(tt.hostToSetDefault)
			hostList := instance().d.getHostList()
			var defaultHost string
			for _, host := range hostList.Hosts {
				if host.IsDefault {
					defaultHost = host.Name
					break
				}
			}
			assert.Equal(t, tt.expectedDefault, defaultHost)
		})
	}
}

func TestHasHost(t *testing.T) {
	tests := []struct {
		name         string
		initialData  data
		hostToCheck  string
		expectedBool bool
	}{
		{
			name:         "Host exists",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			hostToCheck:  "host1",
			expectedBool: true,
		},
		{
			name:         "Host does not exist",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			hostToCheck:  "host3",
			expectedBool: false,
		},
		{
			name:         "Empty host list",
			initialData:  data{hosts: hosts{Hosts: []*host{}}},
			hostToCheck:  "host1",
			expectedBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			hasHost := HasHost(tt.hostToCheck)
			assert.Equal(t, tt.expectedBool, hasHost)
		})
	}
}

func TestGetHostKey(t *testing.T) {
	tests := []struct {
		name         string
		initialData  data
		hostToGetKey string
		expectedKey  string
	}{
		{
			name:         "Get key for existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1", sshInfo: sshInfo{PathToKey: "key1"}}, {Name: "host2", sshInfo: sshInfo{PathToKey: "key2"}}}}},
			hostToGetKey: "host1",
			expectedKey:  "key1",
		},
		{
			name:         "Get key for non-existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1", sshInfo: sshInfo{PathToKey: "key1"}}, {Name: "host2", sshInfo: sshInfo{PathToKey: "key2"}}}}},
			hostToGetKey: "host3",
			expectedKey:  "",
		},
		{
			name:         "Empty host list",
			initialData:  data{hosts: hosts{Hosts: []*host{}}},
			hostToGetKey: "host1",
			expectedKey:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			key := GetHostKey(tt.hostToGetKey)
			assert.Equal(t, tt.expectedKey, key)
		})
	}
}

func TestGetHostPubKey(t *testing.T) {
	tests := []struct {
		name            string
		initialData     data
		hostToGetPubKey string
		expectedPubKey  string
	}{
		{
			name:            "Get public key for existing host",
			initialData:     data{hosts: hosts{Hosts: []*host{{Name: "host1", sshInfo: sshInfo{PathToPubKey: "pubkey1"}}, {Name: "host2", sshInfo: sshInfo{PathToPubKey: "pubkey2"}}}}},
			hostToGetPubKey: "host1",
			expectedPubKey:  "pubkey1",
		},
		{
			name:            "Get public key for non-existing host",
			initialData:     data{hosts: hosts{Hosts: []*host{{Name: "host1", sshInfo: sshInfo{PathToPubKey: "pubkey1"}}, {Name: "host2", sshInfo: sshInfo{PathToPubKey: "pubkey2"}}}}},
			hostToGetPubKey: "host3",
			expectedPubKey:  "",
		},
		{
			name:            "Empty host list",
			initialData:     data{hosts: hosts{Hosts: []*host{}}},
			hostToGetPubKey: "host1",
			expectedPubKey:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			pubKey := GetHostPubKey(tt.hostToGetPubKey)
			assert.Equal(t, tt.expectedPubKey, pubKey)
		})
	}
}

func TestUpdateHostKey(t *testing.T) {
	tests := []struct {
		name        string
		initialData data
		boHost      bo.Host
		expectError bool
	}{
		{
			name:        "Update key for existing host",
			initialData: data{hosts: hosts{Hosts: []*host{{Name: "host1", sshInfo: sshInfo{PathToKey: "key1", PathToPubKey: "pubkey1"}}, {Name: "host2", sshInfo: sshInfo{PathToKey: "key1", PathToPubKey: "pubkey1"}}}}},
			boHost:      bo.Host{Name: "host1", KeyPair: bo.KeyPair{PathToPrv: "newkey1", PathToPub: "newpubkey1"}},
			expectError: false,
		},
		{
			name:        "Update key for non-existing host",
			initialData: data{hosts: hosts{Hosts: []*host{{Name: "host1", sshInfo: sshInfo{PathToKey: "key1", PathToPubKey: "pubkey1"}}, {Name: "host2", sshInfo: sshInfo{PathToKey: "key1", PathToPubKey: "pubkey1"}}}}},
			boHost:      bo.Host{Name: "host3", KeyPair: bo.KeyPair{PathToPrv: "newkey3", PathToPub: "newpubkey3"}},
			expectError: true,
		},
		{
			name:        "Empty host list",
			initialData: data{hosts: hosts{Hosts: []*host{}}},
			boHost:      bo.Host{Name: "host1", KeyPair: bo.KeyPair{PathToPrv: "newkey1", PathToPub: "newpubkey1"}},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			err := UpdateHostKey(tt.boHost)
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			host, err := instance().d.getHost(tt.boHost.Name)

			assert.NoError(t, err)
			assert.Equal(t, tt.boHost.PathToPrv, host.PathToKey)
			assert.Equal(t, tt.boHost.PathToPub, host.PathToPubKey)
			assert.Equal(t, true, host.UseKey)
		})
	}
}

func TestProjectNamesFromHost(t *testing.T) {
	tests := []struct {
		name              string
		initialData       data
		hostToGetProjects string
		expectedProjects  []string
	}{
		{
			name:              "Get projects for existing host",
			initialData:       data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{"project1", "project2"}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToGetProjects: "host1",
			expectedProjects:  []string{"project1", "project2"},
		},
		{
			name:              "Get projects for non-existing host",
			initialData:       data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{"project1", "project2"}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToGetProjects: "host3",
			expectedProjects:  []string{},
		},
		{
			name:              "Empty host list",
			initialData:       data{hosts: hosts{Hosts: []*host{}}},
			hostToGetProjects: "host1",
			expectedProjects:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			projects := ProjectNamesFromHost(tt.hostToGetProjects)
			assert.Equal(t, tt.expectedProjects, projects)
		})
	}
}

func TestRemoveProjectFromHost(t *testing.T) {
	tests := []struct {
		name             string
		initialData      data
		hostToUpdate     string
		projectToRemove  string
		expectedProjects []string
		expectError      bool
	}{
		{
			name:             "Remove existing project from host",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{"project1", "project2"}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToUpdate:     "host1",
			projectToRemove:  "project1",
			expectedProjects: []string{"project2"},
			expectError:      false,
		},
		{
			name:             "Remove non-existing project from host",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{"project1", "project2"}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToUpdate:     "host1",
			projectToRemove:  "project3",
			expectedProjects: []string{"project1", "project2"},
			expectError:      false,
		},
		{
			name:             "Remove project from non-existing host",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{"project1", "project2"}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToUpdate:     "host3",
			projectToRemove:  "project1",
			expectedProjects: []string{},
			expectError:      true,
		},
		{
			name:             "Remove project from host with empty project list",
			initialData:      data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToUpdate:     "host1",
			projectToRemove:  "project1",
			expectedProjects: []string{},
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			err := RemoveProjectFromHost(tt.hostToUpdate, tt.projectToRemove)
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			host, err := instance().d.getHost(tt.hostToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedProjects, host.Projects)
		})
	}
}

func TestRegisterProjectInHost(t *testing.T) {
	tests := []struct {
		name              string
		initialData       data
		hostToUpdate      string
		projectToRegister string
		expectedProjects  []string
		expectError       bool
	}{
		{
			name:              "Register new project to existing host",
			initialData:       data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{"project1"}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToUpdate:      "host1",
			projectToRegister: "project2",
			expectedProjects:  []string{"project1", "project2"},
			expectError:       false,
		},
		{
			name:              "Register existing project to existing host",
			initialData:       data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{"project1", "project2"}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToUpdate:      "host1",
			projectToRegister: "project2",
			expectedProjects:  []string{"project1", "project2"},
			expectError:       false,
		},
		{
			name:              "Register project to non-existing host",
			initialData:       data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{"project1"}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToUpdate:      "host3",
			projectToRegister: "project1",
			expectedProjects:  []string{},
			expectError:       true,
		},
		{
			name:              "Register project to host with empty project list",
			initialData:       data{hosts: hosts{Hosts: []*host{{Name: "host1", Projects: []string{}}, {Name: "host2", Projects: []string{"project3"}}}}},
			hostToUpdate:      "host1",
			projectToRegister: "project1",
			expectedProjects:  []string{"project1"},
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			err := RegisterProjectInHost(tt.hostToUpdate, tt.projectToRegister)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			host, err := instance().d.getHost(tt.hostToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedProjects, host.Projects)
		})
	}
}

func TestGetHostUsername(t *testing.T) {
	tests := []struct {
		name              string
		initialData       data
		hostToGetUsername string
		expectedUsername  string
	}{
		{
			name:              "Get username for existing host",
			initialData:       data{hosts: hosts{Hosts: []*host{{Name: "host1", sshInfo: sshInfo{Username: "user1"}}, {Name: "host2", sshInfo: sshInfo{Username: "user2"}}}}},
			hostToGetUsername: "host1",
			expectedUsername:  "user1",
		},
		{
			name:              "Get username for non-existing host",
			initialData:       data{hosts: hosts{Hosts: []*host{{Name: "host1", sshInfo: sshInfo{Username: "user1"}}, {Name: "host2", sshInfo: sshInfo{Username: "user2"}}}}},
			hostToGetUsername: "host3",
			expectedUsername:  "",
		},
		{
			name:              "Empty host list",
			initialData:       data{hosts: hosts{Hosts: []*host{}}},
			hostToGetUsername: "host1",
			expectedUsername:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()
			username := GetHostUsername(tt.hostToGetUsername)
			assert.Equal(t, tt.expectedUsername, username)
		})
	}
}

func TestSetOrcInfoName(t *testing.T) {
	tests := []struct {
		name         string
		initialData  data
		hostToUpdate string
		newName      string
		expectedName string
		expectError  bool
	}{
		{
			name:         "Set orchestration info name for existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1", OrchestrationInfo: orchestrationInfo{Name: "oldName"}}, {Name: "host2"}}}},
			hostToUpdate: "host1",
			newName:      "newName",
			expectedName: "newName",
			expectError:  false,
		},
		{
			name:         "Set orchestration info name for non-existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			hostToUpdate: "host3",
			newName:      "newName",
			expectedName: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()

			err := SetOrcInfoName(tt.hostToUpdate, tt.newName)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			host, err := instance().d.getHost(tt.hostToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedName, host.OrchestrationInfo.Name)
		})
	}
}

func TestSetOrcInfoStatus(t *testing.T) {
	tests := []struct {
		name           string
		initialData    data
		hostToUpdate   string
		newStatus      bo.ContainerStatus
		expectedStatus string
		expectError    bool
	}{
		{
			name:           "Set orchestration info status for existing host",
			initialData:    data{hosts: hosts{Hosts: []*host{{Name: "host1", OrchestrationInfo: orchestrationInfo{State: "running"}}, {Name: "host2"}}}},
			hostToUpdate:   "host1",
			newStatus:      bo.KContainerStatusExited,
			expectedStatus: "exited",
			expectError:    false,
		},
		{
			name:           "Set orchestration info status for non-existing host",
			initialData:    data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			hostToUpdate:   "host3",
			newStatus:      bo.KContainerStatusExited,
			expectedStatus: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()

			err := SetOrcInfoStatus(tt.hostToUpdate, tt.newStatus)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			host, err := instance().d.getHost(tt.hostToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, host.OrchestrationInfo.State)
		})
	}
}

func TestSetOrcInfoRegistryStatus(t *testing.T) {
	tests := []struct {
		name                   string
		initialData            data
		hostToUpdate           string
		newRegistryStatus      bo.ContainerStatus
		expectedRegistryStatus string
		expectError            bool
	}{
		{
			name:                   "Set registry status for existing host",
			initialData:            data{hosts: hosts{Hosts: []*host{{Name: "host1", OrchestrationInfo: orchestrationInfo{RegistryInfo: registryInfo{State: "running"}}}, {Name: "host2"}}}},
			hostToUpdate:           "host1",
			newRegistryStatus:      bo.KContainerStatusExited,
			expectedRegistryStatus: "exited",
			expectError:            false,
		},
		{
			name:                   "Set registry status for non-existing host",
			initialData:            data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			hostToUpdate:           "host3",
			newRegistryStatus:      bo.KContainerStatusExited,
			expectedRegistryStatus: "",
			expectError:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()

			err := SetOrcInfoRegistryStatus(tt.hostToUpdate, tt.newRegistryStatus)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			host, err := instance().d.getHost(tt.hostToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedRegistryStatus, host.OrchestrationInfo.RegistryInfo.State)
		})
	}
}

func TestSetOrCInfoRegPort(t *testing.T) {
	tests := []struct {
		name         string
		initialData  data
		hostToUpdate string
		newPort      int
		expectedPort int
		expectError  bool
	}{
		{
			name:         "Set registry port for existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1", OrchestrationInfo: orchestrationInfo{RegistryInfo: registryInfo{Port: 8080}}}, {Name: "host2"}}}},
			hostToUpdate: "host1",
			newPort:      9090,
			expectedPort: 9090,
			expectError:  false,
		},
		{
			name:         "Set registry port for non-existing host",
			initialData:  data{hosts: hosts{Hosts: []*host{{Name: "host1"}, {Name: "host2"}}}},
			hostToUpdate: "host3",
			newPort:      9090,
			expectedPort: 0,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.initialData)
			defer tearDown()

			err := SetOrcInfoRegPort(tt.hostToUpdate, tt.newPort)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			host, err := instance().d.getHost(tt.hostToUpdate)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPort, host.OrchestrationInfo.RegistryInfo.Port)
		})
	}
}
