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
		"id":     fmt.Sprintf("echo %d", os.Getuid()), // the user owns its rc files
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

// TestInstallRegistryEnvScriptSkipsLinkedConfigDirs: the script runs as root,
// so it never creates dirs or writes registry.env through a user-planted
// ~/.config or ~/.config/cds link (CDS-055).
func TestInstallRegistryEnvScriptSkipsLinkedConfigDirs(t *testing.T) {
	for _, tc := range []struct {
		name string
		link string // path under home made a link to the target dir
	}{
		{"linked .config", ".config"},
		{"linked .config/cds", ".config/cds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			stubs := t.TempDir()
			for name, body := range map[string]string{
				"getent": `echo "dev:x:1000:1000::` + home + `:/bin/sh"`,
				"id":     fmt.Sprintf("echo %d", os.Getuid()),
				"chown":  `exit 0`,
			} {
				require.NoError(t, os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
			}
			// a root dir outside the home, linked from the user's config path
			target := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(target, "keep"), []byte("root-data\n"), 0o644))
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(home, tc.link)), 0o700))
			require.NoError(t, os.Symlink(target, filepath.Join(home, tc.link)))

			cmd := exec.Command("/bin/sh", "-s")
			cmd.Env = append(os.Environ(), "PATH="+stubs+":"+os.Getenv("PATH"))
			cmd.Stdin = strings.NewReader(installRegistryEnvScript(newRegistryEnv("cdsabc", "s3cr3t", "dev")))
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))

			entries, err := os.ReadDir(target)
			require.NoError(t, err)
			require.Len(t, entries, 1, "nothing is created in the link target")
			assert.Equal(t, "keep", entries[0].Name())
			fi, err := os.Lstat(filepath.Join(home, tc.link))
			require.NoError(t, err)
			assert.NotZero(t, fi.Mode()&os.ModeSymlink, "the link is left as is")
			assert.Contains(t, string(out), tc.link)
			assert.NoFileExists(t, filepath.Join(home, ".profile"), "no rc line for an env file that was not written")
		})
	}
}

// TestInstallRegistryEnvScriptSkipsUnsafeRcFiles: the script runs as root, so
// it never appends through a user-planted rc link or into an rc file the
// container user does not own (CDS-054).
func TestInstallRegistryEnvScriptSkipsUnsafeRcFiles(t *testing.T) {
	for _, tc := range []struct {
		name      string
		uid       int
		wantRcSet bool
	}{
		{"rc files owned by the user", os.Getuid(), true},
		{"rc files owned by someone else", os.Getuid() + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			stubs := t.TempDir()
			for name, body := range map[string]string{
				"getent": `echo "dev:x:1000:1000::` + home + `:/bin/sh"`,
				"id":     fmt.Sprintf("echo %d", tc.uid),
				"chown":  `exit 0`,
			} {
				require.NoError(t, os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
			}
			// a root-owned file outside the home, linked from the user's .zshrc
			target := filepath.Join(t.TempDir(), "qa-target")
			require.NoError(t, os.WriteFile(target, []byte("root-data\n"), 0o644))
			require.NoError(t, os.Symlink(target, filepath.Join(home, ".zshrc")))
			require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# user rc\n"), 0o644))

			cmd := exec.Command("/bin/sh", "-s")
			cmd.Env = append(os.Environ(), "PATH="+stubs+":"+os.Getenv("PATH"))
			cmd.Stdin = strings.NewReader(installRegistryEnvScript(newRegistryEnv("cdsabc", "s3cr3t", "dev")))
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))

			b, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, "root-data\n", string(b), "the link target is not written")
			fi, err := os.Lstat(filepath.Join(home, ".zshrc"))
			require.NoError(t, err)
			assert.NotZero(t, fi.Mode()&os.ModeSymlink, "the link is left as is")
			assert.Contains(t, string(out), ".zshrc")

			b, err = os.ReadFile(filepath.Join(home, ".bashrc"))
			require.NoError(t, err)
			assert.Equal(t, tc.wantRcSet, strings.Contains(string(b), "# cds registry env"), ".bashrc")
		})
	}
}

// TestInstallRegistryEnvScriptWithoutFind: an image without find still gets
// registry.env in an existing ~/.config and the guard line in the user's own
// rc files; the owner checks must not read "no find" as "not owned" (CDS-057).
func TestInstallRegistryEnvScriptWithoutFind(t *testing.T) {
	home := t.TempDir()
	stubs := t.TempDir()
	for name, body := range map[string]string{
		"getent": `echo "dev:x:1000:1000::` + home + `:/bin/sh"`,
		"id":     fmt.Sprintf("echo %d", os.Getuid()),
		"chown":  `exit 0`,
		"find":   `echo "find: not found" >&2; exit 127`, // the image has no find
	} {
		require.NoError(t, os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config"), 0o755))
	for _, rc := range []string{".bashrc", ".bash_profile"} {
		require.NoError(t, os.WriteFile(filepath.Join(home, rc), []byte("# user rc\n"), 0o644))
	}

	cmd := exec.Command("/bin/sh", "-s")
	cmd.Env = append(os.Environ(), "PATH="+stubs+":"+os.Getenv("PATH"))
	cmd.Stdin = strings.NewReader(installRegistryEnvScript(newRegistryEnv("cdsabc", "s3cr3t", "dev")))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	assert.FileExists(t, filepath.Join(home, registryEnvFile))
	for _, rc := range []string{".profile", ".bashrc", ".bash_profile"} {
		b, err := os.ReadFile(filepath.Join(home, rc))
		require.NoError(t, err, rc)
		assert.Equal(t, 1, strings.Count(string(b), "# cds registry env"), rc)
	}
	assert.NotContains(t, string(out), "not changed")
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

// TestRedecorateOneReportsSkipWarnings: a decoration script that skips a write
// prints a "cds: " line and exits 0; that line must reach the client as a
// warning, not be dropped with the output (CDS-061).
func TestRedecorateOneReportsSkipWarnings(t *testing.T) {
	ops := &mockContainerOps{execOutput: "cds: /home/u/.config is a link or not a directory owned by u, /home/u/.config/cds/registry.env not written\nother output\n"}
	s := newTestServer(t, ops)

	var got []deployProgress
	s.redecorateOne(context.Background(), "dev-1", []decorationCommand{{info: "Exporting registry environment", script: "true"}},
		func(p deployProgress) error {
			got = append(got, p)
			return nil
		})

	var warnings []string
	for _, p := range got {
		if p.phase == kProgressPhaseWarning {
			warnings = append(warnings, p.message)
		}
	}
	assert.Equal(t, []string{"dev-1: /home/u/.config is a link or not a directory owned by u, /home/u/.config/cds/registry.env not written"}, warnings)
}

// TestReportDecorationWarnings: only "cds: " lines are warnings, one each, and
// a failed report is returned so the deploy stops like for other output.
func TestReportDecorationWarnings(t *testing.T) {
	var got []deployProgress
	err := reportDecorationWarnings(func(p deployProgress) error {
		got = append(got, p)
		return nil
	}, "dev-1", "noise\ncds: a not changed\r\n  cds: b not written\nxcds: c\n")
	require.NoError(t, err)
	assert.Equal(t, []deployProgress{
		{phase: kProgressPhaseWarning, message: "dev-1: a not changed"},
		{phase: kProgressPhaseWarning, message: "dev-1: b not written"},
	}, got)

	err = reportDecorationWarnings(func(deployProgress) error { return assert.AnError }, "dev-1", "cds: a\n")
	assert.ErrorIs(t, err, assert.AnError)
	assert.NoError(t, reportDecorationWarnings(nil, "dev-1", "cds: a\n"))
}
