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
	defined  bool
	built    bool
	outputs  map[string]string
	failures map[string]bool
	executed []string
	copied   map[string]string // remote path -> content
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		defined: true,
		outputs: map[string]string{
			"systemd-path user-configuration": "/home/remote-user/.config\n",
		},
		failures: map[string]bool{},
		copied:   map[string]string{},
	}
}

func (f *fakeHost) Defined() bool { return f.defined }

func (f *fakeHost) Build() error {
	f.built = true
	return nil
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
	dir, err := New(WithTarget(h)).userUnitDir()

	require.NoError(t, err)
	assert.Equal(t, kRemoteUnitDir, dir)
}

func TestUserUnitDirFailsOnEmptyOutput(t *testing.T) {
	h := newFakeHost()
	h.outputs["systemd-path user-configuration"] = "  \n"

	_, err := New(WithTarget(h)).userUnitDir()

	assert.Error(t, err)
}

func TestStartServiceCreatesUnitsInTargetDirAndEnablesThemNow(t *testing.T) {
	h := newFakeHost()
	h.failures["test -f "+kRemoteUnitDir+"/"+socketUnitName] = true

	err := New(WithTarget(h), WithServicePort(9090), WithServiceBinary("/opt/cds/cds-api-agent")).StartService()

	require.NoError(t, err)
	assert.Contains(t, h.executed, "mkdir -p "+kRemoteUnitDir)
	require.Contains(t, h.copied, kRemoteUnitDir+"/"+socketUnitName)
	require.Contains(t, h.copied, kRemoteUnitDir+"/"+serviceUnitName)
	assert.Contains(t, h.copied[kRemoteUnitDir+"/"+socketUnitName], "ListenStream=9090")
	assert.Contains(t, h.copied[kRemoteUnitDir+"/"+serviceUnitName], "ExecStart=/opt/cds/cds-api-agent")
	assert.Equal(t, []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now " + socketUnitName + " " + serviceUnitName,
	}, h.executed[len(h.executed)-2:])
	assert.False(t, h.built, "a defined host must not be rebuilt")
}

func TestIsUnitReady(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(*fakeHost)
		want  bool
	}{
		"both units enabled": {
			setup: func(h *fakeHost) {
				h.outputs["systemctl --user is-enabled "+socketUnitName] = "enabled\n"
				h.outputs["systemctl --user is-enabled "+serviceUnitName] = "enabled\n"
			},
			want: true,
		},
		"disabled unit is not mistaken for enabled": {
			setup: func(h *fakeHost) {
				h.outputs["systemctl --user is-enabled "+socketUnitName] = "enabled\n"
				h.outputs["systemctl --user is-enabled "+serviceUnitName] = "disabled\n"
			},
			want: false,
		},
		"is-enabled non-zero exit": {
			setup: func(h *fakeHost) {
				h.failures["systemctl --user is-enabled "+socketUnitName] = true
			},
			want: false,
		},
		"missing unit file": {
			setup: func(h *fakeHost) {
				h.failures["test -f "+kRemoteUnitDir+"/"+serviceUnitName] = true
			},
			want: false,
		},
		"unit dir cannot be resolved": {
			setup: func(h *fakeHost) {
				h.failures["systemd-path user-configuration"] = true
			},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFakeHost()
			tc.setup(h)
			assert.Equal(t, tc.want, New(WithTarget(h)).isUnitReady())
		})
	}
}

func TestBuildUnitsDefaults(t *testing.T) {
	units, err := New().buildUnits(8087)

	require.NoError(t, err)
	assert.Contains(t, string(units[socketUnitName]), "ListenStream=8087")
	assert.Contains(t, string(units[serviceUnitName]), "Requires="+socketUnitName)
}
