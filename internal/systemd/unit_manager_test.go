package systemd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerAvailable(t *testing.T) {
	h := newFakeHost()
	assert.True(t, newUnitManager(h).available())

	h.failures["systemctl --user --no-pager"] = true
	assert.False(t, newUnitManager(h).available())
}

func TestManagerIsActive(t *testing.T) {
	h := newFakeHost()
	h.outputs["systemctl --user is-active cds-sec-registry.service"] = "active\n"
	assert.True(t, newUnitManager(h).isActive("cds-sec-registry.service"))

	h.outputs["systemctl --user is-active cds-sec-registry.service"] = "inactive\n"
	assert.False(t, newUnitManager(h).isActive("cds-sec-registry.service"))

	h.failures["systemctl --user is-active cds-kind.service"] = true
	assert.False(t, newUnitManager(h).isActive("cds-kind.service"))
}

// TestManagerReconcileArbitraryUnits proves the generic manager reconciles any
// named unit into any directory by content, which is what the registry Quadlet
// / kind unit / devcontainer Quadlets rely on.
func TestManagerReconcileArbitraryUnits(t *testing.T) {
	h := newFakeHost()
	const dir = "/home/remote-user/.config/containers/systemd"
	unitA := Unit{Name: "cds-sec-registry.container", Content: []byte("[Container]\nImage=registry:2\n")}

	changed, err := newUnitManager(h).reconcile(dir, unitA)
	require.NoError(t, err)
	assert.True(t, changed, "missing unit is written on first reconcile")
	assert.Contains(t, h.executed, "mkdir -p "+dir)
	require.Contains(t, h.copied, dir+"/"+unitA.Name)
	assert.Equal(t, string(unitA.Content), h.copied[dir+"/"+unitA.Name])
	assert.Contains(t, h.executed, "systemctl --user daemon-reload")
}

func TestManagerReconcileSkipsUpToDateUnit(t *testing.T) {
	h := newFakeHost()
	const dir = "/home/remote-user/.config/containers/systemd"
	content := []byte("[Container]\nImage=registry:2\n")
	h.outputs["cat "+dir+"/cds-sec-registry.container"] = string(content)

	changed, err := newUnitManager(h).reconcile(dir, Unit{Name: "cds-sec-registry.container", Content: content})
	require.NoError(t, err)
	assert.False(t, changed, "an identical unit is not rewritten")
	assert.NotContains(t, h.copied, dir+"/cds-sec-registry.container")
}

func TestManagerStartDoesNotEnable(t *testing.T) {
	h := newFakeHost()
	require.NoError(t, newUnitManager(h).start("cds-kind.service"))
	assert.Equal(t, []string{"systemctl --user start cds-kind.service"}, h.executed)
}

func TestManagerEnableNow(t *testing.T) {
	h := newFakeHost()
	require.NoError(t, newUnitManager(h).enableNow("cds-sec-registry.container"))
	assert.Equal(t, []string{"systemctl --user enable --now cds-sec-registry.container"}, h.executed)
}

// TestManagerRemoveDeletesFilesAndToleratesDisableFailure proves teardown
// removes the unit files and reloads even when `disable` fails (e.g. a unit
// that was never enabled), which is required for `space host delete` and
// force teardown paths.
func TestManagerRemoveDeletesFilesAndToleratesDisableFailure(t *testing.T) {
	h := newFakeHost()
	const dir = "/home/remote-user/.config/containers/systemd"
	h.failures["systemctl --user disable --now cds-sec-registry.container"] = true

	err := newUnitManager(h).remove(dir, "cds-sec-registry.container")
	require.NoError(t, err, "disable failure must not fail teardown")
	assert.Contains(t, h.executed, "rm -f "+dir+"/cds-sec-registry.container")
	assert.Contains(t, h.executed, "systemctl --user daemon-reload")
}

func TestManagerRestart(t *testing.T) {
	h := newFakeHost()
	require.NoError(t, newUnitManager(h).restart("cds.service"))
	assert.Equal(t, []string{"systemctl --user restart cds.service"}, h.executed)
}

func TestManagerStop(t *testing.T) {
	h := newFakeHost()
	require.NoError(t, newUnitManager(h).stop("cds.socket", "cds.service"))
	assert.Equal(t, []string{"systemctl --user stop cds.socket cds.service"}, h.executed)
}
