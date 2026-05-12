package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/containerconf"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
	"github.com/stretchr/testify/assert"
)

type testResourceProvider struct {
	files map[string][]byte
}

func (p testResourceProvider) FetchFile(rt containerconf.ResourceType, identifier string) ([]byte, error) {
	data, ok := p.files[identifier]
	if !ok {
		return nil, fmt.Errorf("missing file %s", identifier)
	}
	return data, nil
}

func (p testResourceProvider) FileExists(rt containerconf.ResourceType, identifier string) bool {
	_, ok := p.files[identifier]
	return ok
}

func TestMounts(t *testing.T) {
	mockedMountString := "source=${localEnv:HOME}/workspace,target=/workspace,type=bind"
	config := containerconf.NewConfig()
	config.Set("mounts", []interface{}{mockedMountString})
	expectedDefaultMount := "source=${localEnv:HOME}/.devbox,target=/devbox,type=bind"

	ce := NewContainerEngine(WithContainerConfig(config))

	mounts, err := ce.mounts()

	assert.Nil(t, err)
	assert.Subset(t, mounts, []string{mockedMountString, expectedDefaultMount})
}

func TestMountsWithPvc(t *testing.T) {
	mockedMountString := "source=${localEnv:HOME}/workspace,target=/workspace,type=bind"
	config := containerconf.NewConfig()
	config.Set("mounts", []interface{}{mockedMountString})
	config.Set(cg.VariadicJoin(".", "orchestration", containerconf.KPersistentVolumeClaim), true)
	ce := NewContainerEngine(WithContainerConfig(config))

	mounts, err := ce.mounts()
	assert.Nil(t, err)
	assert.Subset(t, mounts, []string{mockedMountString, KPersistentVolumeMount})

}

func TestGetProfileAttributeValue(t *testing.T) {
	// test with empty local profile
	config, err := containerconf.ParseBytes(strings.NewReader("{}"))
	assert.Nil(t, err)
	ce := NewContainerEngine(WithContainerConfig(config))
	value := ce.getProfileAttributeValue("key")
	assert.Equal(t, "", value)

	// test when asking for unsupported attribute
	value = ce.getProfileAttributeValue("unsupported")
	assert.Equal(t, "", value)

	// test when attribute is defined in flavour profile
	flavourProfileConfig := make(map[string]interface{})
	flavourProfileConfig["defaultShell"] = "zsh"
	config.Set(containerconf.KCds, flavourProfileConfig)

	value = ce.getProfileAttributeValue("defaultShell")
	assert.Equal(t, "zsh", value)
}

func TestGetDevcontainerNameForConfigUsesProvidedConfig(t *testing.T) {
	configA := containerconf.NewConfig()
	configA.Set(containerconf.KName, "alpha")

	configB := containerconf.NewConfig()
	configB.Set(containerconf.KName, "beta")

	gotA := GetDevcontainerNameForConfig("project", configA)
	gotB := GetDevcontainerNameForConfig("project", configB)

	assert.Contains(t, gotA, "-alpha-")
	assert.Contains(t, gotB, "-beta-")
	assert.NotEqual(t, gotA, gotB)
}

func TestResolveUsersFromConfigDefaultsWithoutSharedConfig(t *testing.T) {
	assert.Equal(t, kDefaultUser, ResolveContainerUserFromConfig(nil))
	assert.Equal(t, kDefaultUser, ResolveRemoteUserFromConfig(nil))
}

func TestGetDevcontainerNameForConfigFallsBackWithoutConfig(t *testing.T) {
	got := GetDevcontainerNameForConfig("project", nil)

	assert.Contains(t, got, "project-")
	assert.NotContains(t, got, "-alpha-")
}

func TestRunRequiresExplicitConfig(t *testing.T) {
	ce := NewContainerEngine()
	ce.SetAction(K_ACTION_RUN)

	_, err := ce.BuildCommands()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "container configuration is required for devcontainer run")
}

func TestDevcontainerRunCommandRunsDetached(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	var runCmd string
	for _, cmd := range cmds {
		if strings.HasPrefix(cmd.Cmd(), "podman run ") {
			runCmd = cmd.Cmd()
			break
		}
	}
	assert.NotEmpty(t, runCmd)
	assert.Contains(t, runCmd, " --detach ")
}

func TestDevcontainerRunPublishesSSHPortByDefault(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runCmd := findCommandWithPrefix(cmds, "podman run ")
	assert.Contains(t, runCmd, " -p 22 ")
}

func TestDevcontainerRunDoesNotDuplicateConfiguredSSHPort(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	config.Set(containerconf.KAppPort, []interface{}{"2222:22"})

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runCmd := findCommandWithPrefix(cmds, "podman run ")
	assert.Contains(t, runCmd, " -p 2222:22 ")
	assert.NotContains(t, runCmd, " -p 22 ")
}

func TestDevcontainerRunStartsAsRootBeforeUserBootstrap(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	config.Set(containerconf.KContainerUser, "dev")

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runCmd := findCommandWithPrefix(cmds, "podman run ")
	assert.NotEmpty(t, runCmd)
	assert.Contains(t, runCmd, " -u root ")
	assert.NotContains(t, runCmd, " -u dev ")
}

func TestDevcontainerRunEnsuresConfiguredUsersBeforePostCreate(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	config.Set(containerconf.KContainerUser, "dev")
	config.Set(containerconf.KRemoteUser, "dev")
	config.Set(containerconf.KPostCreateCommand, "echo ready")

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runIndex := findCommandIndexWithPrefix(cmds, "podman run ")
	ensureIndex := findCommandIndexWithDescription(cmds, "Ensuring container user 'dev' exists")
	postCreateIndex := findCommandIndexWithDescription(cmds, "Running devcontainer postCreateCommand")
	assert.NotEqual(t, -1, runIndex)
	assert.NotEqual(t, -1, ensureIndex)
	assert.NotEqual(t, -1, postCreateIndex)
	assert.Less(t, runIndex, ensureIndex)
	assert.Less(t, ensureIndex, postCreateIndex)
	assert.Contains(t, cmds[ensureIndex].Cmd(), "useradd -m -s /bin/sh \"$user\"")
	assert.Contains(t, cmds[ensureIndex].Cmd(), "adduser -D -s /bin/sh \"$user\"")
	assert.Contains(t, cmds[ensureIndex].Cmd(), "chown \"$user\" \"$home_dir\"")
	assert.Contains(t, cmds[ensureIndex].Cmd(), "mkdir -p \"$ssh_dir\"")
	assert.Contains(t, cmds[ensureIndex].Cmd(), "chmod 600 \"$ssh_dir/authorized_keys\"")

	ensureCount := 0
	for _, cmd := range cmds {
		if cmd.Description() == "Ensuring container user 'dev' exists" {
			ensureCount++
		}
	}
	assert.Equal(t, 1, ensureCount)
}

func TestPreServerSkipsMissingAuthArtifact(t *testing.T) {
	ce := NewContainerEngine(WithResourceProvider(testResourceProvider{files: map[string][]byte{}}))

	err := ce.preServer()

	assert.NoError(t, err)
}

func TestSSHKeyCommandInstallsKeyAsRootForRemoteUser(t *testing.T) {
	identifier, err := containerconf.SingletonIdentifier(containerconf.KindPubKey)
	assert.NoError(t, err)
	config := containerconf.NewConfig()
	config.Set(containerconf.KRemoteUser, "developer")
	ce := NewContainerEngine(
		WithContainerConfig(config),
		WithResourceProvider(testResourceProvider{files: map[string][]byte{
			identifier: []byte("ssh-ed25519 AAAATEST developer@example.com\n"),
		}}),
	)
	ce.SetAction(K_ACTION_EXE)
	ce.SetExecuteCmd(K_EXEC_CMD_SSH)
	ce.SetContainerName("container-a")
	ce.SetRemoteUser("developer")

	cmds, err := ce.BuildCommands()
	assert.NoError(t, err)
	if assert.Len(t, cmds, 1) {
		cmd := cmds[0].Cmd()
		assert.Contains(t, cmd, "podman exec -u root -i container-a")
		assert.Contains(t, cmd, `user="developer"`)
		assert.Contains(t, cmd, "ssh-ed25519 AAAATEST developer@example.com")
		assert.Contains(t, cmd, "useradd -m -s /bin/bash")
	}
}

func TestDeleteContainerPassesContainerNameOnce(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "podman.log")
	podmanPath := filepath.Join(dir, "podman")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
count=0
for arg in "$@"; do
  if [ "$arg" = "sample-container" ]; then
    count=$((count + 1))
  fi
done
if [ "$count" -ne 1 ]; then
  echo "container name passed $count times" >&2
  exit 1
fi
`, logPath)
	assert.NoError(t, os.WriteFile(podmanPath, []byte(script), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := DeleteContainer("sample-container")

	assert.NoError(t, err)
	logged, readErr := os.ReadFile(logPath)
	assert.NoError(t, readErr)
	assert.Equal(t, "rm -f sample-container\n", string(logged))
}

func findCommandWithPrefix(cmds []shexec.ExecuteEvent, prefix string) string {
	for _, cmd := range cmds {
		if strings.HasPrefix(cmd.Cmd(), prefix) {
			return cmd.Cmd()
		}
	}
	return ""
}

func findCommandIndexWithPrefix(cmds []shexec.ExecuteEvent, prefix string) int {
	for index, cmd := range cmds {
		if strings.HasPrefix(cmd.Cmd(), prefix) {
			return index
		}
	}
	return -1
}

func findCommandIndexWithDescription(cmds []shexec.ExecuteEvent, description string) int {
	for index, cmd := range cmds {
		if cmd.Description() == description {
			return index
		}
	}
	return -1
}
