package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureOrchestrationNoopWhenNotRequested(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}
	var built int32
	cluster.buildProgress = nil

	err := ensureOrchestration(context.Background(), cluster, reg,
		containerconf.KindOrchestrationConfig{Requested: false},
		func(deployProgress) error { atomic.AddInt32(&built, 1); return nil })

	require.NoError(t, err)
	assert.Zero(t, built, "nothing is ensured when orchestration is not requested")
}

func TestEnsureOrchestrationBuildsClusterAndRegistry(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{
		Requested: true,
		Engine:    "kind",
		Registry:  containerconf.RegistryConfig{Requested: true},
	}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	assert.True(t, reg.built, "registry built when requested")
}

func TestEnsureOrchestrationSkipsRegistryWhenNotRequested(t *testing.T) {
	cluster := &mockClusterOps{}
	// a registry that would error if built, to prove it is not built
	reg := &mockRegistryOps{buildErr: errors.New("registry should not be built")}

	oc := containerconf.KindOrchestrationConfig{
		Requested: true,
		Registry:  containerconf.RegistryConfig{Requested: false},
	}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err, "registry is not built when not requested, so its error never surfaces")
}

func TestEnsureOrchestrationPropagatesClusterError(t *testing.T) {
	cluster := &mockClusterOps{buildErr: errors.New("cluster boom")}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{Requested: true}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster boom")
}

func TestContainerDecorationCommandsShape(t *testing.T) {
	cmds := containerDecorationCommands("KUBECONFIG_CONTENT", "CA_PEM", registryEnv{})
	require.Len(t, cmds, 2)
	assert.Contains(t, cmds[0].script, "/root/.kube/config")
	assert.Contains(t, cmds[0].script, "KUBECONFIG_CONTENT")
	assert.Contains(t, cmds[1].script, "/etc/containers/certs.d/"+registry.Hostname)
	assert.Contains(t, cmds[1].script, "CA_PEM")
}

func TestContainerDecorationCommandsSkipEmpty(t *testing.T) {
	assert.Empty(t, containerDecorationCommands("", "", registryEnv{}))

	onlyKube := containerDecorationCommands("KC", "", registryEnv{})
	require.Len(t, onlyKube, 1)
	assert.Contains(t, onlyKube[0].script, "/root/.kube/config")
}

func TestEnsureOrchestrationAppliesMirrors(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{
		Requested: true,
		Mirrors: []containerconf.Mirror{
			{Registry: "docker.io", Endpoint: "https://mirror/docker"},
		},
	}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	require.Len(t, cluster.appliedMirrors, 1)
	assert.Equal(t, "docker.io", cluster.appliedMirrors[0].Registry)
	assert.Equal(t, "https://mirror/docker", cluster.appliedMirrors[0].Endpoint)
}

func TestEnsureOrchestrationDeploysIngressWhenRequested(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{Requested: true, Ingress: true}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	assert.True(t, cluster.ingressDeployed, "ingress is deployed when requested")
}

func TestEnsureOrchestrationSkipsIngressWhenNotRequested(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}

	oc := containerconf.KindOrchestrationConfig{Requested: true, Ingress: false}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	assert.False(t, cluster.ingressDeployed)
}

func TestContainerDecorationIncludesRegistryEnv(t *testing.T) {
	reg := newRegistryEnv("cdsabc", "secret", "dev")
	cmds := containerDecorationCommands("KC", "CA", reg)
	// kubeconfig + trust + registry env
	require.Len(t, cmds, 3)
	last := cmds[2].script
	assert.Contains(t, last, "CDS_REGISTRY_NAME=registry.cds.internal")
	assert.Contains(t, last, "CDS_REGISTRY_PORT=5000")
	assert.Contains(t, last, "CDS_REGISTRY_USR=cdsabc")
	assert.Contains(t, last, "CDS_REGISTRY_PWD=secret")
	assert.Contains(t, last, "u='dev'")
	assert.Contains(t, last, registryEnvFile)
	// the old world-readable profile script is removed, never written (CDS-024)
	assert.Contains(t, last, "rm -f /etc/profile.d/cds-registry.sh")
	assert.NotContains(t, last, "chmod 644")
}

func TestNewRegistryEnvDefaultsOwnerToRoot(t *testing.T) {
	assert.Equal(t, "root", newRegistryEnv("cdsabc", "secret", "").owner)
}

// TestInstallRegistryEnvScriptRealShell runs the script with a real /bin/sh
// against a temp home: getent/id/chown are stubbed (the test is not root), the
// rest (mktemp, mv, grep, rc sourcing) is real.
func TestInstallRegistryEnvScriptRealShell(t *testing.T) {
	home := t.TempDir()
	stubs := t.TempDir()
	for name, body := range map[string]string{
		"getent": `echo "dev:x:1000:1000::` + home + `:/bin/sh"`,
		"id":     `echo 1000`,
		"chown":  `exit 0`,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	}
	// an existing .bashrc is appended to; a missing .zshrc is not created
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# user rc\n"), 0o644))

	script := installRegistryEnvScript(newRegistryEnv("cdsabc", "s3cr3t", "dev"))
	run := func() {
		cmd := exec.Command("/bin/sh", "-s")
		cmd.Env = append(os.Environ(), "PATH="+stubs+":"+os.Getenv("PATH"))
		cmd.Stdin = strings.NewReader(script)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run()
	run() // re-decoration is idempotent

	envFile := filepath.Join(home, registryEnvFile)
	info, err := os.Stat(envFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	matches, _ := filepath.Glob(filepath.Join(home, ".config/cds/.registry.env.*"))
	assert.Empty(t, matches, "no temp file left behind")

	for _, rc := range []string{".profile", ".bashrc"} {
		b, err := os.ReadFile(filepath.Join(home, rc))
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(b), "# cds registry env"), rc)
	}
	_, err = os.Stat(filepath.Join(home, ".zshrc"))
	assert.True(t, os.IsNotExist(err))

	// a login shell of the user gets the documented CDS_REGISTRY_* env
	cmd := exec.Command("/bin/sh", "-c", `. "$HOME/.profile"; echo "$CDS_REGISTRY_USR:$CDS_REGISTRY_PWD:$CDS_REGISTRY_NAME:$CDS_REGISTRY_PORT"`)
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Equal(t, "cdsabc:s3cr3t:"+registry.Hostname+":5000\n", string(out))
}

func TestContainerDecorationSkipsRegistryEnvWhenUnset(t *testing.T) {
	cmds := containerDecorationCommands("KC", "CA", registryEnv{})
	require.Len(t, cmds, 2, "no registry env step when creds are not set")
}

func TestEnsureOrchestrationCreatesPullSecret(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{user: "cdsu", pass: "cdsp"}

	oc := containerconf.KindOrchestrationConfig{
		Requested: true,
		Registry:  containerconf.RegistryConfig{Requested: true},
	}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	assert.True(t, cluster.registrySecretCreated, "in-cluster pull secret is created when a registry is requested")
}

// TestRegistryPortSettingIsIgnored: a leftover orchestration.registry.port
// no longer moves node trust, the pull secret or the container env off the
// port the registry really listens on (CDS-033).
func TestRegistryPortSettingIsIgnored(t *testing.T) {
	config, err := containerconf.ParseBytes(strings.NewReader(`{
		"image": "alpine:3.19",
		"orchestration": {"engine": "kind", "registry": {"secure": true, "port": 6000}}
	}`))
	require.NoError(t, err)
	require.True(t, config.RegistryPortIgnored())

	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{user: "cdsu", pass: "cdsp", ca: "CA"}
	require.NoError(t, ensureOrchestration(context.Background(), cluster, reg, config.OrchestrationConfig(), func(deployProgress) error { return nil }))
	assert.Equal(t, []int{registry.DefaultPort, registry.DefaultPort}, cluster.registryPorts, "trust and pull secret use the registry port")

	env := newRegistryEnv("cdsu", "cdsp", "")
	assert.Equal(t, registry.DefaultPort, env.port)
	assert.Contains(t, env.caPath, fmt.Sprintf(":%d/ca.crt", registry.DefaultPort))
}

// TestEnsureOrchestrationRegistrySecureKey: the registry is always secured, so
// an absent or true `secure` key deploys and anything else fails before any
// build starts (CDS-034).
func TestEnsureOrchestrationRegistrySecureKey(t *testing.T) {
	for _, tc := range []struct {
		name, registry string
		wantErr        bool
	}{
		{"empty object", `{}`, false},
		{"secure true", `{"secure": true}`, false},
		{"secure false", `{"secure": false}`, true},
		{"secure not a bool", `{"secure": "yes"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, err := containerconf.ParseBytes(strings.NewReader(`{
				"image": "alpine:3.19",
				"orchestration": {"engine": "kind", "registry": ` + tc.registry + `}
			}`))
			require.NoError(t, err)

			cluster := &mockClusterOps{}
			reg := &mockRegistryOps{user: "cdsu", pass: "cdsp"}
			err = ensureOrchestration(context.Background(), cluster, reg, config.OrchestrationConfig(), func(deployProgress) error { return nil })
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "insecure registry is not supported")
				assert.False(t, reg.built, "nothing is built for an insecure request")
				return
			}
			require.NoError(t, err)
			assert.True(t, reg.built)
			assert.True(t, cluster.registrySecretCreated)
		})
	}
}

func TestEnsureOrchestrationNoPullSecretWithoutRegistry(t *testing.T) {
	cluster := &mockClusterOps{}
	reg := &mockRegistryOps{}
	oc := containerconf.KindOrchestrationConfig{Requested: true, Registry: containerconf.RegistryConfig{Requested: false}}
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error { return nil })
	require.NoError(t, err)
	assert.False(t, cluster.registrySecretCreated)
}

func TestEnsureOrchestrationSerializesReports(t *testing.T) {
	cluster := &mockClusterOps{buildProgress: []clusterProgress{{message: "a"}, {message: "b"}, {message: "c"}}}
	reg := &mockRegistryOps{buildProgress: []registryProgress{{message: "x"}, {message: "y"}, {message: "z"}}}
	oc := containerconf.KindOrchestrationConfig{
		Requested: true,
		Registry:  containerconf.RegistryConfig{Requested: true},
	}

	var inFlight, overlaps int32
	err := ensureOrchestration(context.Background(), cluster, reg, oc, func(deployProgress) error {
		if atomic.AddInt32(&inFlight, 1) > 1 {
			atomic.AddInt32(&overlaps, 1)
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	})
	require.NoError(t, err)
	assert.Zero(t, overlaps, "report must never run concurrently")
}
