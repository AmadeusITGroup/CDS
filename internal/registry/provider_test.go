package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestBuildRejectsInsecure(t *testing.T) {
	p, _ := testProvider(t)
	err := p.Build(false, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insecure registry is not supported")
}

func TestQuadletUnitHasExpectedShape(t *testing.T) {
	p, _ := testProvider(t)
	m, err := ensureMaterial(p.materialRoot)
	require.NoError(t, err)

	unit := string(p.quadletUnit(m))
	assert.Contains(t, unit, "[Container]")
	assert.Contains(t, unit, "ContainerName="+Name)
	assert.Contains(t, unit, "Image="+Image)
	assert.Contains(t, unit, "PublishPort=5000:5000")
	assert.Contains(t, unit, "REGISTRY_AUTH=htpasswd")
	assert.Contains(t, unit, "REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt")
	assert.Contains(t, unit, "Network=kind")
	assert.Contains(t, unit, "NetworkAlias="+Hostname)
	assert.Contains(t, unit, m.certsDir()+":/certs:Z")
	assert.Contains(t, unit, m.authDir()+":/auth:Z")
	assert.Contains(t, unit, m.dataDir()+":/var/lib/registry:Z")
}

func TestInstallHostTrustWritesCert(t *testing.T) {
	p, _ := testProvider(t)
	m, err := ensureMaterial(p.materialRoot)
	require.NoError(t, err)

	require.NoError(t, p.installHostTrust(m))
	caPath := filepath.Join(p.certsDBase, Hostname+":5000", "ca.crt")
	got, err := os.ReadFile(caPath)
	require.NoError(t, err)
	want, err := os.ReadFile(m.certFile())
	require.NoError(t, err)
	assert.Equal(t, want, got, "registry cert is installed as the certs.d CA")
}

func TestStopFallsBackToPodmanWhenUnitInactive(t *testing.T) {
	p, r := testProvider(t)
	p.units = systemd.NewUnitManager(fakeHostOps{})
	require.NoError(t, p.Stop())
	assert.True(t, r.ranContaining("podman stop "+Name))
}

func TestDeleteRemovesContainer(t *testing.T) {
	p, r := testProvider(t)
	require.NoError(t, p.Delete(true))
	assert.True(t, r.ranContaining("podman rm -f "+Name))
}
