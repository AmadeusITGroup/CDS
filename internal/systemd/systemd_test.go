package systemd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHost records executed commands and copied files. Commands are answered
// from outputs (keyed by the full command line); unknown commands succeed
// with an empty output unless listed in failures.
type fakeHost struct {
	outputs  map[string]string
	failures map[string]bool
	executed []string
	copied   map[string]string // remote path -> content
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		outputs: map[string]string{
			"systemd-path user-configuration": "/home/remote-user/.config\n",
		},
		failures: map[string]bool{},
		copied:   map[string]string{},
	}
}

func (f *fakeHost) Execute(name string, args ...string) (string, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	f.executed = append(f.executed, cmd)
	if f.failures[cmd] {
		return "", errors.New("exit status 1")
	}
	return f.outputs[cmd], nil
}

func (f *fakeHost) Copy(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	f.copied[remotePath+"/"+filepath.Base(localPath)] = string(data)
	return nil
}

const kRemoteUnitDir = "/home/remote-user/.config/systemd/user"

func TestUserUnitDirIsResolvedOnTarget(t *testing.T) {
	h := newFakeHost()
	dir, err := newUnitManager(h).userUnitDir()

	require.NoError(t, err)
	assert.Equal(t, kRemoteUnitDir, dir)
}

func TestUserUnitDirFailsOnEmptyOutput(t *testing.T) {
	h := newFakeHost()
	h.outputs["systemd-path user-configuration"] = "  \n"

	_, err := newUnitManager(h).userUnitDir()

	assert.Error(t, err)
}

func TestStartServiceCreatesUnitsInTargetDirAndEnablesThemNow(t *testing.T) {
	h := newFakeHost()

	changed, err := New(WithTarget(h), WithServicePort(9090), WithServiceBinary("/opt/cds/cds-api-agent")).StartService()

	require.NoError(t, err)
	assert.True(t, changed, "units are created on first start")
	assert.Contains(t, h.executed, "mkdir -p "+kRemoteUnitDir)
	require.Contains(t, h.copied, kRemoteUnitDir+"/"+socketUnitName)
	require.Contains(t, h.copied, kRemoteUnitDir+"/"+serviceUnitName)
	assert.Contains(t, h.copied[kRemoteUnitDir+"/"+socketUnitName], "ListenStream=9090")
	assert.Contains(t, h.copied[kRemoteUnitDir+"/"+serviceUnitName], "ExecStart=/opt/cds/cds-api-agent")
	assert.Equal(t, []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now " + socketUnitName + " " + serviceUnitName,
	}, h.executed[len(h.executed)-2:])
}

// installUnits seeds the fake host with the unit files produced for binary/port,
// as an already provisioned host would have them, and answers `cat` with them.
func installUnits(t *testing.T, h *fakeHost, binary string, port int) {
	t.Helper()
	units, err := New(WithServiceBinary(binary), WithServicePort(port)).buildUnits(port)
	require.NoError(t, err)
	for _, name := range []string{socketUnitName, serviceUnitName} {
		h.outputs["cat "+kRemoteUnitDir+"/"+name] = string(units[name])
	}
}

func TestStartServiceLeavesUpToDateUnitsUnchanged(t *testing.T) {
	h := newFakeHost()
	installUnits(t, h, "/opt/cds/bin/a/cds-api-agent", 9090)

	changed, err := New(WithTarget(h), WithServicePort(9090), WithServiceBinary("/opt/cds/bin/a/cds-api-agent")).StartService()

	require.NoError(t, err)
	assert.False(t, changed, "identical units are not rewritten")
	assert.NotContains(t, h.copied, kRemoteUnitDir+"/"+serviceUnitName, "no unit file is copied")
}

// TestStartServiceReconcilesStaleBinary covers P1: an upgrade installs a new
// binary under a versioned path, so the service unit's ExecStart must be
// rewritten even though units already exist and are enabled. The reviewer's
// focused reproduction created units for binary A, then started for binary B
// and asserted ExecStart still pointed at A.
func TestStartServiceReconcilesStaleBinary(t *testing.T) {
	h := newFakeHost()
	installUnits(t, h, "/opt/cds/bin/a/cds-api-agent", 9090)

	changed, err := New(WithTarget(h), WithServicePort(9090), WithServiceBinary("/opt/cds/bin/b/cds-api-agent")).StartService()

	require.NoError(t, err)
	assert.True(t, changed, "a new binary rewrites the service unit")
	require.Contains(t, h.copied, kRemoteUnitDir+"/"+serviceUnitName)
	assert.Contains(t, h.copied[kRemoteUnitDir+"/"+serviceUnitName], "ExecStart=/opt/cds/bin/b/cds-api-agent")
	assert.NotContains(t, h.copied[kRemoteUnitDir+"/"+serviceUnitName], "/bin/a/")
	assert.Contains(t, h.executed, "systemctl --user daemon-reload")
}

func TestBuildUnitsDefaults(t *testing.T) {
	units, err := New().buildUnits(8087)

	require.NoError(t, err)
	assert.Contains(t, string(units[socketUnitName]), "ListenStream=8087")
	assert.Contains(t, string(units[serviceUnitName]), "Requires="+socketUnitName)
}

func TestRestartService(t *testing.T) {
	h := newFakeHost()

	require.NoError(t, New(WithTarget(h)).RestartService())
	assert.Equal(t, []string{"systemctl --user restart " + serviceUnitName}, h.executed)

	h.failures["systemctl --user restart "+serviceUnitName] = true
	assert.Error(t, New(WithTarget(h)).RestartService())
}
