package systemd

import (
	"errors"
	"strings"
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

// TestManagerRemoveResetsFailedState: a unit that ended failed keeps a
// "not-found failed" entry after its file is gone, so remove clears it, and a
// reset-failed error (nothing to reset) does not fail teardown.
func TestManagerRemoveResetsFailedState(t *testing.T) {
	h := newFakeHost()
	const dir = "/home/remote-user/.config/systemd/user"
	h.failures["systemctl --user reset-failed cds-kind.service"] = true

	require.NoError(t, newUnitManager(h).remove(dir, "cds-kind.service"))
	assert.Equal(t, "systemctl --user reset-failed cds-kind.service", h.executed[len(h.executed)-1])
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

// probingHost is a fakeHost that also offers Probe, like the remote host: the
// unit manager must send its "is it there / is it up" questions through it.
type probingHost struct {
	*fakeHost
	probed []string
}

func (p *probingHost) Probe(name string, args ...string) (string, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	p.probed = append(p.probed, cmd)
	if p.failures[cmd] {
		return "", errors.New("exit status 1")
	}
	return p.outputs[cmd], nil
}

// TestManagerProbesExpectedNoAnswersQuietly: `is-active` (exit 3 when
// inactive) and the unit-file `cat` (exit 1 when missing) are probes whose
// "no" is a normal answer, so they go through Probe, never Execute, which
// warns the user on a non-zero exit. Real changes still use Execute.
func TestManagerProbesExpectedNoAnswersQuietly(t *testing.T) {
	h := &probingHost{fakeHost: newFakeHost()}
	const dir = "/home/remote-user/.config/systemd/user"
	h.failures["systemctl --user is-active cds.service"] = true
	h.failures["cat "+dir+"/cds.socket"] = true

	m := newUnitManager(h)
	assert.False(t, m.isActive("cds.service"))
	changed, err := m.reconcile(dir, Unit{Name: "cds.socket", Content: []byte("[Socket]\n")})
	require.NoError(t, err)
	assert.True(t, changed, "a missing unit file is still written")

	assert.Equal(t, []string{"systemctl --user is-active cds.service", "cat " + dir + "/cds.socket"}, h.probed)
	assert.Equal(t, []string{"mkdir -p " + dir, "systemctl --user daemon-reload"}, h.executed)
}
