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
	"unicode/utf8"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/orchestration"
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
	cmds := containerDecorationCommands("KUBECONFIG_CONTENT", "CA_PEM", "", registryEnv{})
	require.Len(t, cmds, 2)
	assert.Contains(t, cmds[0].script, "/root/.kube/config")
	assert.Contains(t, cmds[0].script, "KUBECONFIG_CONTENT")
	assert.Contains(t, cmds[1].script, "/etc/containers/certs.d/"+registry.Hostname)
	assert.Contains(t, cmds[1].script, "CA_PEM")
}

func TestContainerDecorationCommandsSkipEmpty(t *testing.T) {
	assert.Empty(t, containerDecorationCommands("", "", "", registryEnv{}))

	onlyKube := containerDecorationCommands("KC", "", "", registryEnv{})
	require.Len(t, onlyKube, 1)
	assert.Contains(t, onlyKube[0].script, "/root/.kube/config")
}

// stubGetent puts a getent on PATH that gives user dev the home home; the
// scripts run as the test user, as they run as the container user.
func stubGetent(t *testing.T, home string) []string {
	t.Helper()
	stubs := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stubs, "getent"),
		[]byte("#!/bin/sh\necho \"dev:x:1000:1000::"+home+":/bin/sh\"\n"), 0o755))
	return append(os.Environ(), "PATH="+stubs+":"+os.Getenv("PATH"))
}

// runScript runs script with a real /bin/sh as the test user and returns its
// stdout: warnings must be there, a successful first-deploy step keeps only
// stdout.
func runScript(t *testing.T, home, script string) string {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-s")
	cmd.Env = stubGetent(t, home)
	cmd.Stdin = strings.NewReader(script)
	b, err := cmd.Output()
	require.NoError(t, err, string(b))
	return string(b)
}

// TestInstallUserKubeconfigScript: with a remoteUser the kubeconfig lands in
// that user's ~/.kube/config (600), and a link planted at the target is
// replaced, not followed.
func TestInstallUserKubeconfigScript(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".kube"), 0o700))
	victim := filepath.Join(t.TempDir(), "other-file")
	require.NoError(t, os.WriteFile(victim, []byte("other-data\n"), 0o644))
	require.NoError(t, os.Symlink(victim, filepath.Join(home, ".kube", "config")))

	script := installUserKubeconfigScript("apiVersion: v1\nkind: Config", "dev")
	runScript(t, home, script)
	runScript(t, home, script) // re-decoration is idempotent

	p := filepath.Join(home, ".kube", "config")
	fi, err := os.Lstat(p)
	require.NoError(t, err)
	assert.True(t, fi.Mode().IsRegular())
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "apiVersion: v1\nkind: Config\n", string(b))
	b, err = os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "other-data\n", string(b), "the link target is not written")
	matches, _ := filepath.Glob(filepath.Join(home, ".kube", ".config.*"))
	assert.Empty(t, matches, "no temp file left behind")

	// a user without ~/.kube gets it created, 700
	fresh := t.TempDir()
	runScript(t, fresh, script)
	assert.FileExists(t, filepath.Join(fresh, ".kube", "config"))
	fi, err = os.Stat(filepath.Join(fresh, ".kube"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
}

// TestInstallUserKubeconfigScriptUnwritableKubeDir: a ~/.kube the user cannot
// write is skipped with a warning, the script still succeeds.
func TestInstallUserKubeconfigScriptUnwritableKubeDir(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".kube"), []byte("not a dir\n"), 0o644))

	out := runScript(t, home, installUserKubeconfigScript("KC", "dev"))
	assert.Contains(t, out, "cds: cannot write to "+filepath.Join(home, ".kube"))
}

// TestDecorationRootNeverWritesUserHome: root writes only root-owned paths;
// every write into the container user's home runs as that user, so a link
// the user swaps in can never redirect a root write.
func TestDecorationRootNeverWritesUserHome(t *testing.T) {
	for _, owner := range []string{"", "root", "dev"} {
		cmds := containerDecorationCommands("KC", "CA", owner, newRegistryEnv("cdsabc", "secret", owner))
		var asUser []string
		for _, dc := range cmds {
			if dc.runAs() == "root" && owner == "dev" {
				assert.NotContains(t, dc.script, "$home", "%s: %s runs as root", owner, dc.info)
			}
			assert.NotContains(t, dc.script, "chown", "%s: %s", owner, dc.info)
			if dc.runAs() != "root" {
				asUser = append(asUser, dc.info)
				assert.Equal(t, owner, dc.runAs(), dc.info)
			}
		}
		if owner == "dev" {
			assert.Equal(t, []string{"Installing cluster kubeconfig for the container user", "Exporting registry environment in container"}, asUser)
		} else {
			assert.Empty(t, asUser, owner)
		}
	}
}

// TestInstallKubeconfigScriptRootOnly: a root (or unset) remoteUser only gets
// root's copy, as before.
func TestInstallKubeconfigScriptRootOnly(t *testing.T) {
	for _, owner := range []string{"", "root"} {
		cmds := containerDecorationCommands("KC", "", owner, registryEnv{})
		require.Len(t, cmds, 1, owner)
		assert.Contains(t, cmds[0].script, "/root/.kube/config", owner)
	}
	cmds := containerDecorationCommands("KC", "", "dev", registryEnv{})
	require.Len(t, cmds, 2)
	assert.Contains(t, cmds[1].script, "u='dev'")
	assert.Contains(t, cmds[1].script, "$home/.kube")
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
	cmds := containerDecorationCommands("KC", "CA", "dev", reg)
	// kubeconfig (root + user) + trust + legacy cleanup + registry env
	require.Len(t, cmds, 5)
	// the old world-readable profile script is removed, as root
	assert.Equal(t, kRemoveLegacyRegistryEnvScript, cmds[3].script)
	assert.Equal(t, "root", cmds[3].runAs())
	last := cmds[4]
	assert.Equal(t, "dev", last.runAs())
	assert.Contains(t, last.script, "CDS_REGISTRY_NAME=registry.cds.internal")
	assert.Contains(t, last.script, "CDS_REGISTRY_PORT=5000")
	assert.Contains(t, last.script, "CDS_REGISTRY_USR=cdsabc")
	assert.Contains(t, last.script, "CDS_REGISTRY_PWD=secret")
	assert.Contains(t, last.script, "u='dev'")
	assert.Contains(t, last.script, registryEnvFile)
	assert.NotContains(t, last.script, "chmod 644")
}

func TestNewRegistryEnvDefaultsOwnerToRoot(t *testing.T) {
	assert.Equal(t, "root", newRegistryEnv("cdsabc", "secret", "").owner)
}

// TestInstallRegistryEnvScriptRealShell runs the script with a real /bin/sh
// against a temp home, as the test user (the script runs as the container
// user); only getent is stubbed.
func TestInstallRegistryEnvScriptRealShell(t *testing.T) {
	home := t.TempDir()
	// an existing .bashrc is appended to; a missing .zshrc is not created
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# user rc\n"), 0o644))

	script := installRegistryEnvScript(newRegistryEnv("cdsabc", "s3cr3t", "dev"))
	runScript(t, home, script)
	runScript(t, home, script) // re-decoration is idempotent

	envFile := filepath.Join(home, registryEnvFile)
	info, err := os.Stat(envFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	for _, d := range []string{".config", ".config/cds"} {
		fi, err := os.Stat(filepath.Join(home, d))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm(), d)
	}
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

// TestInstallRegistryEnvScriptUnwritableConfigDir: a ~/.config the user cannot
// write makes the script skip the env file and rc files with a warning, and
// still succeed.
func TestInstallRegistryEnvScriptUnwritableConfigDir(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".config"), []byte("not a dir\n"), 0o644))

	out := runScript(t, home, installRegistryEnvScript(newRegistryEnv("cdsabc", "s3cr3t", "dev")))
	assert.Contains(t, out, "cds: cannot write to "+filepath.Join(home, ".config/cds"))
	assert.NoFileExists(t, filepath.Join(home, ".profile"), "no rc line for an env file that was not written")
}

// TestInstallRegistryEnvScriptUnwritableRcFile: an rc file the user cannot
// append to (here a link to a read-only file) is skipped with a warning; the
// other rc files still get the line.
func TestInstallRegistryEnvScriptUnwritableRcFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write a read-only file")
	}
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "read-only")
	require.NoError(t, os.WriteFile(target, []byte("other-data\n"), 0o444))
	require.NoError(t, os.Symlink(target, filepath.Join(home, ".zshrc")))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# user rc\n"), 0o644))

	out := runScript(t, home, installRegistryEnvScript(newRegistryEnv("cdsabc", "s3cr3t", "dev")))

	b, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "other-data\n", string(b), "the read-only file is not written")
	assert.Contains(t, out, "cds: cannot write to "+filepath.Join(home, ".zshrc"))
	b, err = os.ReadFile(filepath.Join(home, ".bashrc"))
	require.NoError(t, err)
	assert.Contains(t, string(b), "# cds registry env")
}

func TestContainerDecorationSkipsRegistryEnvWhenUnset(t *testing.T) {
	cmds := containerDecorationCommands("KC", "CA", "", registryEnv{})
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
// port the registry really listens on.
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
// build starts.
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
// warning, not be dropped with the output.
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

// TestRedecorateOneReportsFailedScript: a script that exits non-zero (set -eu,
// e.g. no home directory) is not an error from ExecuteScriptInContainer; it
// must still reach the client as a warning with the output tail.
func TestRedecorateOneReportsFailedScript(t *testing.T) {
	ops := &mockContainerOps{execOutput: "container user dev has no home directory\n", execCode: 1}
	s := newTestServer(t, ops)

	var warnings []string
	s.redecorateOne(context.Background(), "dev-1", []decorationCommand{{info: "Exporting registry environment", script: "true"}},
		func(p deployProgress) error {
			if p.phase == kProgressPhaseWarning {
				warnings = append(warnings, p.message)
			}
			return nil
		})

	assert.Equal(t, []string{"dev-1: Exporting registry environment failed (exit 1): container user dev has no home directory"}, warnings)
}

// TestDecorationFailureTail: only the last kMaxDecorationTail bytes are kept,
// cut on a UTF-8 boundary.
func TestDecorationFailureTail(t *testing.T) {
	assert.Equal(t, "", decorationFailureTail("  \n"))
	long := strings.Repeat("é", kMaxDecorationTail) + "end"
	got := decorationFailureTail(long)
	assert.True(t, strings.HasPrefix(got, "..."))
	assert.True(t, strings.HasSuffix(got, "end"))
	assert.LessOrEqual(t, len(got), kMaxDecorationTail+len("..."))
	assert.True(t, utf8.ValidString(got))
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

// TestEnsureAndResolveDecorationMergesAndPrunesMirrors: a deploy applies its own
// mirrors plus those of the other recorded deployments (its own wins on a
// conflict, with a warning), and prunes every other CDS mirror, such as one
// this container's previous config had.
func TestEnsureAndResolveDecorationMergesAndPrunesMirrors(t *testing.T) {
	cluster := &mockClusterOps{}
	srv := newTestOrchestrationServer(t, cluster, &mockRegistryOps{},
		`{"image":"ubuntu:24.04","orchestration":{"engine":"kind","mirrors":[
			{"registry":"docker.io","endpoint":"https://a"},{"registry":"quay.io","endpoint":"https://q"}]}}`,
		`{"image":"ubuntu:24.04","orchestration":{"engine":"kind","mirrors":[{"registry":"ghcr.io","endpoint":"https://g"}]}}`)

	oc := containerconf.KindOrchestrationConfig{Requested: true, Mirrors: []containerconf.Mirror{{Registry: "docker.io", Endpoint: "https://b"}}}
	var warnings []string
	_, err := (&containerServiceServer{core: srv}).ensureAndResolveDecoration(context.Background(),
		deployContainerSpec{containerName: "dev-1", orchestration: oc},
		func(p deployProgress) error {
			if p.phase == kProgressPhaseWarning {
				warnings = append(warnings, p.message)
			}
			return nil
		})
	require.NoError(t, err)

	want := []orchestration.Mirror{{Registry: "docker.io", Endpoint: "https://b"}, {Registry: "quay.io", Endpoint: "https://q"}}
	assert.Equal(t, want, cluster.appliedMirrors)
	assert.True(t, cluster.pruned)
	assert.Equal(t, want, cluster.prunedKeep, "ghcr.io from dev-1's old config is pruned")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "conflict for docker.io: using https://b, ignoring https://a")
}

// TestEnsureAndResolveDecorationRestoresOthersOnCreatedCluster: a deploy that
// creates the cluster (it was deleted) restores the other recorded
// deployments' ingress and registry access and re-decorates their running
// containers; a deploy onto an existing cluster does not.
func TestEnsureAndResolveDecorationRestoresOthersOnCreatedCluster(t *testing.T) {
	for name, tc := range map[string]struct {
		status  bo.ContainerStatus
		restore bool
	}{
		"cluster created":  {status: bo.KContainerStatusDeleted, restore: true},
		"cluster existing": {status: bo.KContainerStatusRunning, restore: false},
	} {
		t.Run(name, func(t *testing.T) {
			cluster := &mockClusterOps{cluster: orchestration.Cluster{Status: tc.status}, kubeconfig: "NEWKC"}
			containers := &mockContainerOps{containers: bo.Containers{{Name: "dev-0", Status: bo.KContainerStatusRunning}}}
			srv, err := newgrpcServer(NewConfig(
				withStagingDir(t.TempDir()),
				withStateDir(t.TempDir()),
				withContainerOps(containers),
				withClusterOps(cluster),
				withRegistryOps(&mockRegistryOps{ca: "CA", user: "u", pass: "p"}),
			))
			require.NoError(t, err)
			require.NoError(t, srv.deployments().save(&cdspb.DeployContainerRequest{
				ContainerName: "dev-0", ProjectName: "a", DevcontainerConfig: []byte(kOrchestratedConfig),
			}))

			// project B: no ingress, no registry
			oc := containerconf.KindOrchestrationConfig{Requested: true}
			_, err = (&containerServiceServer{core: srv}).ensureAndResolveDecoration(context.Background(),
				deployContainerSpec{containerName: "dev-1", orchestration: oc},
				func(deployProgress) error { return nil })
			require.NoError(t, err)

			assert.Equal(t, tc.restore, cluster.ingressDeployed, "A's ingress")
			assert.Equal(t, tc.restore, cluster.registryTrusted, "A's registry CA on the nodes")
			assert.Equal(t, tc.restore, cluster.registrySecretCreated, "A's pull secret")
			if tc.restore {
				assert.Contains(t, containers.lastExecCmd, "CDS_REGISTRY_USR=u", "dev-0 re-decorated")
			} else {
				assert.Empty(t, containers.lastExecCmd, "dev-0 left alone")
			}
		})
	}
}

// TestWarnReapplyReportsWarning: a failed re-apply step reaches the client as
// a warning (a WARN line), not as an info line with a "Warning: " prefix,
// and a resolved mirror conflict is not labelled a failed re-apply.
func TestWarnReapplyReportsWarning(t *testing.T) {
	var got []deployProgress
	report := func(p deployProgress) error {
		got = append(got, p)
		return nil
	}
	warnReapply(report, errors.New("ingress not ready"))
	warnReapply(report, nil)
	// a resolved mirror conflict is a warning too, not a failed re-apply
	warnMirrorConflicts(report, []string{"registry mirror conflict for docker.io: using a, ignoring b set by another deployment"})
	assert.Equal(t, []deployProgress{
		{phase: kProgressPhaseWarning, message: "failed to re-apply orchestration setting: ingress not ready"},
		{phase: kProgressPhaseWarning, message: "registry mirror conflict for docker.io: using a, ignoring b set by another deployment"},
	}, got)
}
