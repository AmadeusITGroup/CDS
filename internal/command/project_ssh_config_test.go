package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerSSHConfigKeepsUserConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const userConfig = "Host work\n    HostName work.example.com\n"
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(userConfig), 0600))
	container := func(port int) bo.Container {
		return bo.Container{Name: "dev", RemoteUser: "alice", Pmapping: bo.PortMapping{bo.KSSHPortMapping: port}}
	}

	require.NoError(t, upsertContainerSSHConfig("remote.example.com", container(2222)))
	require.NoError(t, upsertContainerSSHConfig("remote.example.com", container(3333)))

	assert.Equal(t, `# >>> CDS managed host dev
Host dev
    HostName remote.example.com
    Port 3333
    User alice
    StrictHostKeyChecking accept-new
# <<< CDS managed host dev

`+userConfig, readSSHConfig(t))

	// a container without published SSH port is not reachable
	require.NoError(t, upsertContainerSSHConfig("remote.example.com", container(0)))
	assert.Equal(t, userConfig, readSSHConfig(t))
	require.NoError(t, removeContainerSSHConfig("dev"))
	assert.Equal(t, userConfig, readSSHConfig(t))
}

func TestContainerSSHConfigRejectsInvalidValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, c := range []bo.Container{
		{Name: "dev\nHost *", Pmapping: bo.PortMapping{bo.KSSHPortMapping: 22}},
		{Name: "dev", RemoteUser: "a b", Pmapping: bo.PortMapping{bo.KSSHPortMapping: 22}},
	} {
		assert.Error(t, upsertContainerSSHConfig("remote.example.com", c))
	}
	assert.Error(t, upsertContainerSSHConfig("bad host", bo.Container{Name: "dev", Pmapping: bo.PortMapping{bo.KSSHPortMapping: 22}}))
}
