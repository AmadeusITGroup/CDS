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
	containerconf.Set("mounts", []interface{}{mockedMountString})
	expectedDefaultMount := "source=${localEnv:HOME}/.devbox,target=/devbox,type=bind"

	var ce ContainersEngine

	mounts, err := ce.mounts()

	assert.Nil(t, err)
	assert.Subset(t, mounts, []string{mockedMountString, expectedDefaultMount})
}

func TestMountsWithPvc(t *testing.T) {
	mockedMountString := "source=${localEnv:HOME}/workspace,target=/workspace,type=bind"
	containerconf.Set("mounts", []interface{}{mockedMountString})
	containerconf.Set(cg.VariadicJoin(".", "orchestration", containerconf.KPersistentVolumeClaim), true)
	var ce ContainersEngine

	mounts, err := ce.mounts()
	assert.Nil(t, err)
	assert.Subset(t, mounts, []string{mockedMountString, KPersistentVolumeMount})

}

func TestGetProfileAttributeValue(t *testing.T) {
	var ce ContainersEngine

	// test with empty local profile
	err := containerconf.LoadFromBytes(strings.NewReader("{}"))
	assert.Nil(t, err)
	value := ce.getProfileAttributeValue("key")
	assert.Equal(t, "", value)

	// test when asking for unsupported attribute
	value = ce.getProfileAttributeValue("unsupported")
	assert.Equal(t, "", value)

	// test when attribute is defined in flavour profile
	flavourProfileConfig := make(map[string]interface{})
	flavourProfileConfig["defaultShell"] = "zsh"
	containerconf.Set(containerconf.KCds, flavourProfileConfig)

	value = ce.getProfileAttributeValue("defaultShell")
	assert.Equal(t, "zsh", value)
}
