package engine

import (
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/containerconf"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/stretchr/testify/assert"
)

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
