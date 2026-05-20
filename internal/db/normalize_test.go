package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDeduplicatesDBState(t *testing.T) {
	d := data{
		Context: context{ProjectContext: "ProjectA"},
		projects: projects{Projects: []*project{
			nil,
			{
				Name: " ProjectA ",
				Host: " LOCALHOST ",
				Containers: []*containerInfo{
					{Name: " app ", Id: "old-id", State: " RUNNING "},
					{Name: "app", Id: "new-id", ExpectedState: " EXITED "},
				},
			},
			{
				Name:         "ProjectA",
				Host:         "localhost",
				UseSshTunnel: true,
				Containers:   []*containerInfo{{Name: "sidecar"}},
			},
			{Name: "ProjectB", Host: "RemoteHost"},
		}},
		hosts: hosts{Hosts: []*host{
			{Name: " localhost ", Projects: []string{"ProjectA", "ProjectA", ""}, IsDefault: true, sshInfo: sshInfo{Username: "old-user"}},
			{Name: "LOCALHOST", Projects: []string{"ProjectB"}, sshInfo: sshInfo{Username: "new-user", UseKey: true}},
			{Name: "remotehost", IsDefault: true},
		}},
		registryInstances: registryInstances{Instances: []*registryInstance{
			{Name: " registry "},
			{Name: "registry"},
			{Name: ""},
		}},
	}

	d.normalize()

	require.Len(t, d.Projects, 2)
	projectA, err := d.getProject("ProjectA")
	require.NoError(t, err)
	assert.Equal(t, "localhost", projectA.Host)
	assert.True(t, projectA.UseSshTunnel)
	require.Len(t, projectA.Containers, 2)
	assert.Equal(t, &containerInfo{Name: "app", Id: "new-id", State: "running", ExpectedState: "exited"}, projectA.Containers[0])
	assert.Equal(t, &containerInfo{Name: "sidecar"}, projectA.Containers[1])

	require.Len(t, d.Hosts, 2)
	localhost, err := d.getHost("LOCALHOST")
	require.NoError(t, err)
	assert.Equal(t, "new-user", localhost.Username)
	assert.True(t, localhost.UseKey)
	assert.False(t, localhost.IsDefault)
	assert.Equal(t, []string{"ProjectA"}, localhost.Projects)

	remoteHost, err := d.getHost("remotehost")
	require.NoError(t, err)
	assert.True(t, remoteHost.IsDefault)
	assert.Equal(t, []string{"ProjectB"}, remoteHost.Projects)

	require.Len(t, d.Instances, 1)
	assert.Equal(t, "registry", d.Instances[0].Name)
}

func TestNormalizeClearsInvalidContextAndSkipsZombies(t *testing.T) {
	d := data{
		Context: context{ProjectContext: "missing"},
		projects: projects{Projects: []*project{
			{Name: " "},
			{Name: "ProjectA", Containers: []*containerInfo{{Name: " "}}},
		}},
		hosts:             hosts{Hosts: []*host{{Name: " "}, {Name: "localhost", Projects: []string{"missing-project"}}}},
		registryInstances: registryInstances{Instances: []*registryInstance{{Name: " "}}},
	}

	d.normalize()

	assert.Empty(t, d.Context.ProjectContext)
	require.Len(t, d.Projects, 1)
	assert.Equal(t, "ProjectA", d.Projects[0].Name)
	assert.Empty(t, d.Projects[0].Containers)
	require.Len(t, d.Hosts, 1)
	assert.Empty(t, d.Hosts[0].Projects)
	assert.False(t, d.Hosts[0].InUse)
	assert.Empty(t, d.Instances)
}
