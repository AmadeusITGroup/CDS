package orchestration

import (
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/systemd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner records the shell commands issued and replies from a scripted map
// (prefix match), defaulting to empty output / no error.
type fakeRunner struct {
	ran     []string
	replies map[string]string
	fail    map[string]bool
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{replies: map[string]string{}, fail: map[string]bool{}}
}

func (f *fakeRunner) run(command string) (string, error) {
	f.ran = append(f.ran, command)
	for prefix, out := range f.replies {
		if strings.Contains(command, prefix) {
			if f.fail[prefix] {
				return out, assertErr(prefix)
			}
			return out, nil
		}
	}
	return "", nil
}

type cmdErr string

func (e cmdErr) Error() string { return string(e) }
func assertErr(s string) error { return cmdErr("command failed: " + s) }

func (f *fakeRunner) ranContaining(sub string) bool {
	for _, c := range f.ran {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// kindProviderWithRunner builds a provider whose shell is the fake runner and
// whose supervision unit path is pre-resolved (so tests never touch podman on
// disk). units is left nil: tests here exercise only runner-driven methods.
func kindProviderWithRunner(r *fakeRunner) *kindProvider {
	return &kindProvider{runner: r.run, podmanPath: "/usr/bin/podman"}
}

func TestKindClusterExistsDetectsCluster(t *testing.T) {
	r := newFakeRunner()
	r.replies["kind get clusters"] = "foo\ncds-kind\nbar\n"
	k := kindProviderWithRunner(r)

	exists, err := k.clusterExists()
	require.NoError(t, err)
	assert.True(t, exists)
	assert.True(t, r.ranContaining("KIND_EXPERIMENTAL_PROVIDER=podman kind get clusters"))
}

func TestKindKubeconfigReadsFromControlPlane(t *testing.T) {
	r := newFakeRunner()
	r.replies["kubectl config view"] = "apiVersion: v1\nkind: Config\n"
	k := kindProviderWithRunner(r)

	cfg, err := k.Kubeconfig()
	require.NoError(t, err)
	assert.Contains(t, cfg, "kind: Config")
	assert.True(t, r.ranContaining("podman exec -u root cds-kind-control-plane kubectl config view --minify --flatten"))
}

func TestStatusActionTable(t *testing.T) {
	assert.Equal(t, "none", statusAction(bo.KContainerStatusRunning))
	assert.Equal(t, "start", statusAction(bo.KContainerStatusExited))
	assert.Equal(t, "build", statusAction(bo.KContainerStatusDeleted))
	assert.Equal(t, "build", statusAction(bo.KContainerStatusUnknown))
}

func TestProviderSelectsKind(t *testing.T) {
	p, err := Provider("")
	require.NoError(t, err)
	assert.Equal(t, "kind", p.Name())

	p, err = Provider("kind")
	require.NoError(t, err)
	assert.Equal(t, "kind", p.Name())

	_, err = Provider("k3s")
	assert.Error(t, err)
}

// fakeHostOps is a systemd.HostOps whose systemctl is-active always reports
// inactive, exercising the Stop fallback that stops the node directly.
type fakeHostOps struct{}

func (fakeHostOps) Execute(name string, args ...string) (string, error) {
	// is-active -> "inactive" so UnitManager.IsActive returns false
	return "inactive", nil
}
func (fakeHostOps) Copy(localPath, remotePath string) error { return nil }

func TestKindStopFallsBackToPodmanWhenUnitInactive(t *testing.T) {
	r := newFakeRunner()
	k := kindProviderWithRunner(r)
	k.units = systemd.NewUnitManager(fakeHostOps{})

	err := k.Stop()
	require.NoError(t, err)
	assert.True(t, r.ranContaining("podman stop cds-kind-control-plane"),
		"when the supervision unit is inactive, Stop stops the node directly")
}
