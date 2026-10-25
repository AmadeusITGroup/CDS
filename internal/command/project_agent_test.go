package command

import (
	"context"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const kTestDevcontainer = `{
	// comments are part of the devcontainer.json format
	"name": "dev",
	"image": "alpine:3.20",
	"remoteUser": "developer"
}`

func onTestAgent(t *testing.T, projectName string, callback stubCallback) {
	t.Helper()
	require.NoError(t, onProjectAgent(projectName, callback, withoutAutostart()))
}

func deployTestProject(t *testing.T, agent *fakeAgent, projectName string) string {
	t.Helper()
	plan, err := buildProjectDeployPlan(projectName)
	require.NoError(t, err)
	var containerName string
	onTestAgent(t, projectName, func(services agentServices, ctx context.Context) error {
		containerName, err = deployProject(ctx, services, plan)
		return err
	})
	return containerName
}

func TestDeployProject(t *testing.T) {
	agent := setupTestAgent(t)
	setupTestProject(t, "p1", kTestDevcontainer)
	require.NoError(t, db.SetOverrideImageTag("p1", "1.2.3"))

	containerName := deployTestProject(t, agent, "p1")

	pubKeyID, err := containerconf.SingletonIdentifier(containerconf.KindPubKey)
	require.NoError(t, err)
	assert.Equal(t, "ssh-ed25519 AAAATEST user@test\n", string(agent.staged[pubKeyID]), "the user public key is staged")
	require.Len(t, agent.deploys, 1)
	deploy := agent.deploys[0]
	assert.Equal(t, "p1", deploy.GetProjectName())
	assert.Equal(t, containerName, deploy.GetContainerName())
	assert.True(t, strings.HasPrefix(containerName, "p1-dev-"))
	assert.Equal(t, kTestDevcontainer, string(deploy.GetDevcontainerConfig()), "devcontainer.json is sent as is")
	assert.Equal(t, "1.2.3", deploy.GetOverrideImageTag())
	assert.Equal(t, "p1", deploy.GetLabels()[engine.KCDSContainerProjectLabel])

	assert.Equal(t, []string{containerName}, db.ProjectContainersName("p1"))
	assert.Equal(t, 2222, db.ContainerSSHPort("p1", containerName))
	assert.Equal(t, "developer", db.ProjectContainerRemoteUser("p1", containerName))
	sshConfig := readSSHConfig(t)
	assert.Contains(t, sshConfig, "Host "+containerName+"\n")
	assert.Contains(t, sshConfig, "    Port 2222\n")
	assert.Contains(t, sshConfig, "    User developer\n")
	assert.Contains(t, sshConfig, "    HostName localhost\n")

	// artifacts already staged are not uploaded again
	agent.calls = nil
	require.NoError(t, db.RemoveProjectContainers("p1", containerName))
	deployTestProject(t, agent, "p1")
	assert.NotContains(t, agent.calls, "upload "+pubKeyID)
}

func TestDeployProjectFailure(t *testing.T) {
	agent := setupTestAgent(t)
	agent.deployErr = "image not found"
	setupTestProject(t, "p1", kTestDevcontainer)
	plan, err := buildProjectDeployPlan("p1")
	require.NoError(t, err)

	err = onProjectAgent("p1", func(services agentServices, ctx context.Context) error {
		_, err := deployProject(ctx, services, plan)
		return err
	}, withoutAutostart())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "image not found")
	assert.Empty(t, db.ProjectContainersName("p1"))
}

func TestRebuildProject(t *testing.T) {
	agent := setupTestAgent(t)
	setupTestProject(t, "p1", kTestDevcontainer)
	containerName := deployTestProject(t, agent, "p1")
	require.NoError(t, db.SetOverrideImageTag("p1", "latest"))

	plan, err := buildProjectDeployPlan("p1")
	require.NoError(t, err)
	assert.Equal(t, containerName, plan.containerName, "the deployed container name is kept")
	onTestAgent(t, "p1", func(services agentServices, ctx context.Context) error {
		return rebuildProject(ctx, services, plan)
	})

	require.Len(t, agent.rebuilds, 1)
	assert.Equal(t, containerName, agent.rebuilds[0].GetContainerName())
	assert.Equal(t, "latest", agent.rebuilds[0].GetOverrideImageTag())
	assert.Equal(t, kTestDevcontainer, string(agent.rebuilds[0].GetDevcontainerConfig()))
}

func TestSyncProjectContainers(t *testing.T) {
	agent := setupTestAgent(t)
	setupTestProject(t, "p1", kTestDevcontainer)
	containerName := deployTestProject(t, agent, "p1")
	require.NoError(t, db.UpsertProjectContainer("p1", bo.Container{Name: "gone", Pmapping: bo.PortMapping{bo.KSSHPortMapping: 3333}}))
	require.NoError(t, upsertContainerSSHConfig(cg.KLocalhost, bo.Container{Name: "gone", Pmapping: bo.PortMapping{bo.KSSHPortMapping: 3333}}))
	agent.containers[containerName].PortMapping["22/tcp"] = 4444

	onTestAgent(t, "p1", func(services agentServices, ctx context.Context) error {
		return syncProjectContainers(ctx, services.container, "p1")
	})

	assert.Equal(t, []string{containerName}, db.ProjectContainersName("p1"), "containers absent on the agent are forgotten")
	assert.Equal(t, 4444, db.ContainerSSHPort("p1", containerName))
	sshConfig := readSSHConfig(t)
	assert.NotContains(t, sshConfig, "Host gone")
	assert.Contains(t, sshConfig, "    Port 4444\n")
	assert.Equal(t, 1, strings.Count(sshConfig, "Host "+containerName+"\n"))
}

func TestChangeProjectContainersState(t *testing.T) {
	agent := setupTestAgent(t)
	setupTestProject(t, "p1", kTestDevcontainer)
	containerName := deployTestProject(t, agent, "p1")
	stop := func(services agentServices, ctx context.Context) error {
		return changeProjectContainersState(ctx, services.container, "p1", bo.KContainerStatusExited, func(name string) error {
			_, err := services.container.StopContainer(ctx, &cdspb.StopContainerRequest{ContainerName: name})
			return err
		})
	}

	onTestAgent(t, "p1", stop)
	onTestAgent(t, "p1", stop)

	assert.Equal(t, []string{"stop " + containerName}, agent.calls[len(agent.calls)-1:], "a stopped container is not stopped again")
	assert.Equal(t, 1, strings.Count(strings.Join(agent.calls, ","), "stop "))

	delete(agent.containers, containerName)
	err := onProjectAgent("p1", stop, withoutAutostart())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "was not found on agent host")
}

func TestClearProjectContainers(t *testing.T) {
	agent := setupTestAgent(t)
	setupTestProject(t, "p1", kTestDevcontainer)
	containerName := deployTestProject(t, agent, "p1")
	require.NoError(t, db.UpsertProjectContainer("p1", bo.Container{Name: "already-gone"}))

	onTestAgent(t, "p1", func(services agentServices, ctx context.Context) error {
		return clearProjectContainers(ctx, services.container, "p1")
	})

	assert.Contains(t, agent.calls, "delete "+containerName+" p1")
	assert.NotContains(t, agent.calls, "delete already-gone p1")
	assert.Empty(t, db.ProjectContainersName("p1"))
	assert.NotContains(t, readSSHConfig(t), "Host "+containerName)
}

func TestRenameProject(t *testing.T) {
	agent := setupTestAgent(t)
	setupTestProject(t, "p1", kTestDevcontainer)
	oldName := deployTestProject(t, agent, "p1")

	require.NoError(t, (&projectRename{projectName: "p1", newContainerName: "renamed"}).execute(nil, nil))

	assert.Equal(t, []string{"renamed"}, db.ProjectContainersName("p1"))
	assert.Equal(t, "developer", db.ProjectContainerRemoteUser("p1", "renamed"), "the remote user is kept")
	sshConfig := readSSHConfig(t)
	assert.NotContains(t, sshConfig, "Host "+oldName)
	assert.Contains(t, sshConfig, "Host renamed\n")
}

func TestShareAndUnshareProject(t *testing.T) {
	agent := setupTestAgent(t)
	setupTestProject(t, "p1", kTestDevcontainer)
	containerName := deployTestProject(t, agent, "p1")

	require.NoError(t, (&projectShare{}).execute(nil, []string{"p1"}))

	keyPair := projectSharedKeyPair("p1")
	publicKey, err := readPublicKey(keyPair)
	require.NoError(t, err)
	require.Len(t, agent.executed, 1)
	assert.Equal(t, containerName, agent.executed[0].GetContainerName())
	assert.Equal(t, engine.KRootUsr, agent.executed[0].GetUser())
	assert.Equal(t, engine.AuthorizedKeysCommand(true, "developer", publicKey), agent.executed[0].GetCommand())

	require.NoError(t, (&projectUnshare{}).execute(nil, []string{"p1"}))

	require.Len(t, agent.executed, 2)
	assert.Equal(t, engine.AuthorizedKeysCommand(false, "developer", publicKey), agent.executed[1].GetCommand())
	_, err = readPublicKey(keyPair)
	assert.Error(t, err, "the shared key pair is removed")
	assert.Error(t, (&projectUnshare{}).execute(nil, []string{"p1"}), "a project not shared can't be unshared")
}

func TestContainerSSHTarget(t *testing.T) {
	agent := setupTestAgent(t)
	setupTestProject(t, "p1", kTestDevcontainer)
	containerName := deployTestProject(t, agent, "p1")

	target, err := containerSSHTarget("p1", containerName, cg.EmptyStr)
	require.NoError(t, err)
	assert.Equal(t, cg.KLocalhost, target.FQDN())
	assert.Equal(t, 2222, target.Port())
	assert.Equal(t, "developer", target.Username())
	assert.True(t, strings.HasSuffix(target.PathToPrv(), "/.ssh/id_ed25519"))

	target, err = containerSSHTarget("p1", containerName, "root")
	require.NoError(t, err)
	assert.Equal(t, "root", target.Username(), "the user can be overridden")

	_, err = containerSSHTarget("p1", "unknown", cg.EmptyStr)
	assert.Error(t, err)
}
