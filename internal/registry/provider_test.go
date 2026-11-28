package registry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/systemd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner records commands; replies default to empty/no error.
type fakeRunner struct{ ran []string }

func (f *fakeRunner) run(cmd string) (string, error) {
	f.ran = append(f.ran, cmd)
	return "", nil
}
func (f *fakeRunner) ranContaining(sub string) bool {
	for _, c := range f.ran {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// fakeHostOps reports every unit inactive so Stop uses the podman fallback.
type fakeHostOps struct{}

func (fakeHostOps) Execute(string, ...string) (string, error) { return "inactive", nil }
func (fakeHostOps) Copy(string, string) error                 { return nil }

func testProvider(t *testing.T) (*podmanProvider, *fakeRunner) {
	t.Helper()
	r := &fakeRunner{}
	p := &podmanProvider{
		engineName:   "",
		port:         DefaultPort,
		runner:       r.run,
		units:        systemd.NewUnitManager(systemd.LocalHost()),
		materialRoot: filepath.Join(t.TempDir(), "registry"),
		quadletDir:   filepath.Join(t.TempDir(), "systemd"),
		certsDBase:   filepath.Join(t.TempDir(), "certs.d"),
	}
	return p, r
}

func TestQuadletUnitHasExpectedShape(t *testing.T) {
	p, _ := testProvider(t)
	m, _, err := ensureMaterial(p.materialRoot)
	require.NoError(t, err)

	unit := string(p.quadletUnit(m))
	assert.Contains(t, unit, "[Container]")
	assert.Contains(t, unit, "ContainerName="+Name)
	assert.Contains(t, unit, "Image="+Image)
	// pinned version and digest, not a moving tag
	assert.Regexp(t, `(?m)^Image=docker\.io/library/registry:2\.\d+\.\d+@sha256:[0-9a-f]{64}$`, unit)
	// loopback only: kind nodes and devcontainers reach it on the kind network
	// podman picks the host port, so users on a shared host do not clash (CDS-075)
	assert.Contains(t, unit, "PublishPort=127.0.0.1::5000")
	assert.Contains(t, unit, "REGISTRY_AUTH=htpasswd")
	assert.Contains(t, unit, "REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt")
	assert.Contains(t, unit, "Network=kind")
	assert.Contains(t, unit, "SuccessExitStatus=2 137 143", "clean stop must not leave the unit failed")
	assert.Contains(t, unit, "NetworkAlias="+Hostname)
	assert.Contains(t, unit, m.certsDir()+":/certs:Z")
	assert.Contains(t, unit, m.authDir()+":/auth:Z")
	assert.Contains(t, unit, m.dataDir()+":/var/lib/registry:Z")
}

// portRunner answers `podman port` with a fixed published port.
func portRunner(port string) func(string) (string, error) {
	return func(cmd string) (string, error) {
		if strings.HasPrefix(cmd, "podman port "+Name+" 5000/tcp") {
			return "127.0.0.1:" + port + "\n", nil
		}
		return "", nil
	}
}

func TestInstallHostTrustWritesCertForThePublishedPort(t *testing.T) {
	p, _ := testProvider(t)
	p.runner = portRunner("41234")
	m, _, err := ensureMaterial(p.materialRoot)
	require.NoError(t, err)

	require.NoError(t, p.installHostTrust(m))
	want, err := os.ReadFile(m.certFile())
	require.NoError(t, err)
	for _, addr := range []string{"127.0.0.1:41234", "localhost:41234"} {
		got, err := os.ReadFile(filepath.Join(p.certsDBase, addr, "ca.crt"))
		require.NoError(t, err, addr)
		assert.Equal(t, want, got, "registry cert is installed as the certs.d CA for %s", addr)
	}
}

func TestInstallHostTrustMovesToTheNewPort(t *testing.T) {
	p, _ := testProvider(t)
	m, _, err := ensureMaterial(p.materialRoot)
	require.NoError(t, err)
	// another registry's trust on loopback is not ours to remove
	other := filepath.Join(p.certsDBase, "127.0.0.1:6000")
	require.NoError(t, os.MkdirAll(other, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(other, "ca.crt"), []byte("not ours"), 0644))

	p.runner = portRunner("41234")
	require.NoError(t, p.installHostTrust(m))
	p.runner = portRunner("41999")
	require.NoError(t, p.installHostTrust(m))

	assert.NoDirExists(t, filepath.Join(p.certsDBase, "127.0.0.1:41234"))
	assert.NoDirExists(t, filepath.Join(p.certsDBase, "localhost:41234"))
	assert.FileExists(t, filepath.Join(p.certsDBase, "127.0.0.1:41999", "ca.crt"))
	assert.FileExists(t, filepath.Join(p.certsDBase, "localhost:41999", "ca.crt"))
	assert.FileExists(t, filepath.Join(other, "ca.crt"))
}

func TestInstallHostTrustFailsWithoutAPublishedPort(t *testing.T) {
	p, _ := testProvider(t)
	m, _, err := ensureMaterial(p.materialRoot)
	require.NoError(t, err)
	// fakeRunner answers `podman port` with nothing
	assert.Error(t, p.installHostTrust(m))
}

func TestParseHostPort(t *testing.T) {
	port, err := parseHostPort("127.0.0.1:41234\n")
	require.NoError(t, err)
	assert.Equal(t, 41234, port)
	_, err = parseHostPort("")
	assert.Error(t, err)
}

func TestStopFallsBackToPodmanWhenUnitInactive(t *testing.T) {
	p, r := testProvider(t)
	p.units = systemd.NewUnitManager(fakeHostOps{})
	require.NoError(t, p.Stop())
	assert.True(t, r.ranContaining("podman stop "+Name))
}

// TestStopIsIdempotent: the Quadlet container is removed on stop, so a second
// stop finds no container; that is "already stopped", not a failure. Any
// other podman failure is still returned.
func TestStopIsIdempotent(t *testing.T) {
	p, _ := testProvider(t)
	p.units = systemd.NewUnitManager(fakeHostOps{})
	p.runner = func(string) (string, error) {
		return `Error: no container with name or ID "cds-sec-registry" found: no such container`, errors.New("exit status 125")
	}
	require.NoError(t, p.Stop())

	p.runner = func(string) (string, error) { return "Error: permission denied", errors.New("exit status 125") }
	require.Error(t, p.Stop())
}

// TestStartFailsWithoutRegistry: start after delete (no Quadlet file) fails
// with a clear message and runs nothing (CDS-100). units is nil, so any unit
// call would panic.
func TestStartFailsWithoutRegistry(t *testing.T) {
	p, r := testProvider(t)
	p.units = nil

	err := p.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registry cds-sec-registry does not exist")
	assert.Contains(t, err.Error(), "registry run")
	assert.Empty(t, r.ran)
}

func TestDeleteRemovesContainer(t *testing.T) {
	p, r := testProvider(t)
	p.units = systemd.NewUnitManager(fakeHostOps{})
	require.NoError(t, p.Delete(true))
	assert.True(t, r.ranContaining("podman rm -f "+Name))
}

// recordingHostOps reports every unit active and records the commands run.
type recordingHostOps struct{ cmds []string }

func (r *recordingHostOps) Execute(name string, args ...string) (string, error) {
	r.cmds = append(r.cmds, name+" "+strings.Join(args, " "))
	return "active", nil
}
func (r *recordingHostOps) Copy(string, string) error { return nil }

func TestDeleteStopsRunningUnitBeforeRemovingIt(t *testing.T) {
	p, _ := testProvider(t)
	h := &recordingHostOps{}
	p.units = systemd.NewUnitManager(h)
	require.NoError(t, p.Delete(true))
	stop, rm := -1, -1
	for i, c := range h.cmds {
		if c == "systemctl --user stop "+quadletService {
			stop = i
		}
		if strings.HasPrefix(c, "rm -f ") && rm < 0 {
			rm = i
		}
	}
	require.GreaterOrEqual(t, stop, 0, "running unit must be stopped")
	assert.Less(t, stop, rm, "stop before the unit file is removed")
}

func TestUnitAwareStatusDistinguishesStopFromDelete(t *testing.T) {
	p, _ := testProvider(t)
	assert.Equal(t, bo.KContainerStatusDeleted, p.unitAwareStatus(bo.KContainerStatusDeleted), "no unit file: deleted")

	require.NoError(t, os.MkdirAll(p.quadletDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(p.quadletDir, quadletUnitFile), []byte("x"), 0644))
	assert.Equal(t, bo.KContainerStatusExited, p.unitAwareStatus(bo.KContainerStatusDeleted), "unit kept: exited")
	assert.Equal(t, bo.KContainerStatusRunning, p.unitAwareStatus(bo.KContainerStatusRunning))
}

// failingHostOps reports every unit active and fails the commands whose
// "name args" line starts with one of the given prefixes.
type failingHostOps struct{ failPrefixes []string }

func (f failingHostOps) Execute(name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	for _, p := range f.failPrefixes {
		if strings.HasPrefix(line, p) {
			return "boom", errors.New("exit status 1")
		}
	}
	return "active", nil
}
func (failingHostOps) Copy(string, string) error { return nil }

func TestDeleteReturnsTeardownFailures(t *testing.T) {
	cases := map[string]struct {
		failPrefixes []string
		rmOut        string
		rmErr        error
		want         string
	}{
		"unit stop fails":    {failPrefixes: []string{"systemctl --user stop"}, want: "failed to stop registry service"},
		"unit file rm fails": {failPrefixes: []string{"rm -f "}, want: "failed to remove registry Quadlet unit"},
		"podman rm fails":    {rmOut: "Error: cannot remove container: device busy", rmErr: errors.New("exit status 125"), want: "failed to remove registry container"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, _ := testProvider(t)
			p.units = systemd.NewUnitManager(failingHostOps{failPrefixes: tc.failPrefixes})
			p.runner = func(string) (string, error) { return tc.rmOut, tc.rmErr }
			err := p.Delete(true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestDeleteIgnoresAlreadyGone(t *testing.T) {
	p, _ := testProvider(t)
	// unit not loaded: disable --now fails (tolerated), unit inactive.
	p.units = systemd.NewUnitManager(fakeHostOps{})
	p.runner = func(string) (string, error) {
		return "Error: no such container " + Name, errors.New("exit status 1")
	}
	require.NoError(t, p.Delete(true))
}
