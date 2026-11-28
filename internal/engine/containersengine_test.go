package engine

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/containerconf"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testResourceProvider struct {
	files map[string][]byte
}

func (p testResourceProvider) FetchFile(rt containerconf.ResourceType, identifier string) ([]byte, error) {
	data, ok := p.files[identifier]
	if !ok {
		return nil, fmt.Errorf("missing file %s", identifier)
	}
	return data, nil
}

func (p testResourceProvider) FileExists(rt containerconf.ResourceType, identifier string) bool {
	_, ok := p.files[identifier]
	return ok
}

func TestMounts(t *testing.T) {
	mockedMountString := "source=${localEnv:HOME}/workspace,target=/workspace,type=bind"
	config := containerconf.NewConfig()
	config.Set("mounts", []interface{}{mockedMountString})
	expectedDefaultMount := "source=${localEnv:HOME}/.devbox,target=/devbox,type=bind"

	ce := NewContainerEngine(WithContainerConfig(config))

	mounts, err := ce.mounts()

	assert.Nil(t, err)
	assert.Subset(t, mounts, []string{mockedMountString, expectedDefaultMount})
}

func TestMountsWithPvc(t *testing.T) {
	mockedMountString := "source=${localEnv:HOME}/workspace,target=/workspace,type=bind"
	config := containerconf.NewConfig()
	config.Set("mounts", []interface{}{mockedMountString})
	config.Set(cg.VariadicJoin(".", "orchestration", containerconf.KPersistentVolumeClaim), true)
	ce := NewContainerEngine(WithContainerConfig(config))

	mounts, err := ce.mounts()
	assert.Nil(t, err)
	assert.Subset(t, mounts, []string{mockedMountString, KPersistentVolumeMount})

}

func TestGetProfileAttributeValue(t *testing.T) {
	// test with empty local profile
	config, err := containerconf.ParseBytes(strings.NewReader("{}"))
	assert.Nil(t, err)
	ce := NewContainerEngine(WithContainerConfig(config))
	value := ce.getProfileAttributeValue("key")
	assert.Equal(t, "", value)

	// test when asking for unsupported attribute
	value = ce.getProfileAttributeValue("unsupported")
	assert.Equal(t, "", value)

	// test when attribute is defined in flavour profile
	flavourProfileConfig := make(map[string]interface{})
	flavourProfileConfig["defaultShell"] = "zsh"
	config.Set(containerconf.KCds, flavourProfileConfig)

	value = ce.getProfileAttributeValue("defaultShell")
	assert.Equal(t, "zsh", value)
}

func TestGetDevcontainerNameForConfigUsesProvidedConfig(t *testing.T) {
	configA := containerconf.NewConfig()
	configA.Set(containerconf.KName, "alpha")

	configB := containerconf.NewConfig()
	configB.Set(containerconf.KName, "beta")

	gotA := GetDevcontainerNameForConfig("project", configA)
	gotB := GetDevcontainerNameForConfig("project", configB)

	assert.Contains(t, gotA, "-alpha-")
	assert.Contains(t, gotB, "-beta-")
	assert.NotEqual(t, gotA, gotB)
}

func TestResolveUsersFromConfigDefaultsWithoutSharedConfig(t *testing.T) {
	assert.Equal(t, kDefaultUser, ResolveContainerUserFromConfig(nil))
	assert.Equal(t, kDefaultUser, ResolveRemoteUserFromConfig(nil))
}

func TestGetDevcontainerNameForConfigFallsBackWithoutConfig(t *testing.T) {
	got := GetDevcontainerNameForConfig("project", nil)

	assert.Contains(t, got, "project-")
	assert.NotContains(t, got, "-alpha-")
}

func TestRunRequiresExplicitConfig(t *testing.T) {
	ce := NewContainerEngine()
	ce.SetAction(K_ACTION_RUN)

	_, err := ce.BuildCommands()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "container configuration is required for devcontainer run")
}

func TestDevcontainerRunCommandRunsDetached(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	var runCmd string
	for _, cmd := range cmds {
		if strings.HasPrefix(cmd.Cmd(), "podman run ") {
			runCmd = cmd.Cmd()
			break
		}
	}
	assert.NotEmpty(t, runCmd)
	assert.Contains(t, runCmd, " --detach ")
}

func TestDevcontainerRunUsesDefaultPodmanNetwork(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runCmd := findCommandWithPrefix(cmds, "podman run ")
	assert.Contains(t, runCmd, "--network=podman", "default network is the podman network when WithNetwork is unset")
}

func TestDevcontainerRunAttachesToConfiguredNetwork(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	ce := NewContainerEngine(WithContainerConfig(config), WithNetwork("kind"))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runCmd := findCommandWithPrefix(cmds, "podman run ")
	assert.Contains(t, runCmd, "--network=kind", "WithNetwork attaches the run to the given network")
	assert.NotContains(t, runCmd, "--network=podman")
}

func TestDevcontainerRunPublishesSSHPortByDefault(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runCmd := findCommandWithPrefix(cmds, "podman run ")
	assert.Contains(t, runCmd, " -p 22 ")
}

func TestDevcontainerRunDoesNotDuplicateConfiguredSSHPort(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	config.Set(containerconf.KAppPort, []interface{}{"2222:22"})

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runCmd := findCommandWithPrefix(cmds, "podman run ")
	assert.Contains(t, runCmd, " -p 2222:22 ")
	assert.NotContains(t, runCmd, " -p 22 ")
}

func TestDevcontainerRunStartsAsRootBeforeUserBootstrap(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	config.Set(containerconf.KContainerUser, "dev")

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runCmd := findCommandWithPrefix(cmds, "podman run ")
	assert.NotEmpty(t, runCmd)
	assert.Contains(t, runCmd, " -u root ")
	assert.NotContains(t, runCmd, " -u dev ")
}

func TestDevcontainerRunEnsuresConfiguredUsersBeforePostCreate(t *testing.T) {
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.19")
	config.Set(containerconf.KContainerUser, "dev")
	config.Set(containerconf.KRemoteUser, "dev")
	config.Set(containerconf.KPostCreateCommand, "echo ready")

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()

	assert.NoError(t, err)
	runIndex := findCommandIndexWithPrefix(cmds, "podman run ")
	ensureIndex := findCommandIndexWithDescription(cmds, "Ensuring container user 'dev' exists")
	postCreateIndex := findCommandIndexWithDescription(cmds, "Running devcontainer postCreateCommand")
	assert.NotEqual(t, -1, runIndex)
	assert.NotEqual(t, -1, ensureIndex)
	assert.NotEqual(t, -1, postCreateIndex)
	assert.Less(t, runIndex, ensureIndex)
	assert.Less(t, ensureIndex, postCreateIndex)
	assert.Contains(t, cmds[ensureIndex].Cmd(), `useradd -m -s /bin/sh $gopt "$user"`)
	assert.Contains(t, cmds[ensureIndex].Cmd(), `adduser -D -s /bin/sh ${group:+-G $group} "$user"`)
	assert.Contains(t, cmds[ensureIndex].Cmd(), "chown \"$user\" \"$home_dir\"")
	// the spec is split into account and optional group before creation
	assert.Contains(t, cmds[ensureIndex].Cmd(), "user=${spec%%:*}")

	ensureCount := 0
	for _, cmd := range cmds {
		if cmd.Description() == "Ensuring container user 'dev' exists" {
			ensureCount++
		}
	}
	assert.Equal(t, 1, ensureCount)
}

func TestPreServerSkipsMissingAuthArtifact(t *testing.T) {
	ce := NewContainerEngine(WithResourceProvider(testResourceProvider{files: map[string][]byte{}}))

	err := ce.preServer()

	assert.NoError(t, err)
}

func TestSSHKeyCommandInstallsKeyAsRootForRemoteUser(t *testing.T) {
	identifier, err := containerconf.SingletonIdentifier(containerconf.KindPubKey)
	require.NoError(t, err)
	// a hostile key must reach the container as a single, uninterpreted argument
	keys := "ssh-ed25519 AAAATEST developer@example.com\nssh-rsa AAAA'$(touch /tmp/pwned)'\n"
	config := containerconf.NewConfig()
	config.Set(containerconf.KRemoteUser, "developer")
	ce := NewContainerEngine(
		WithContainerConfig(config),
		WithResourceProvider(testResourceProvider{files: map[string][]byte{identifier: []byte(keys)}}),
	)
	ce.SetAction(K_ACTION_EXE)
	ce.SetExecuteCmd(K_EXEC_CMD_SSH)
	ce.SetContainerName("container-a")
	ce.SetRemoteUser("developer")

	cmds, err := ce.BuildCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	argv := runWithFakePodman(t, cmds[0].Cmd())
	assert.Equal(t, []string{
		"exec", "-u", "root", "-i", "container-a",
		"/bin/sh", "-c", kInstallAuthorizedKeysScript, "sh", "developer", strings.TrimSpace(keys),
	}, argv)
}

func TestInstallAuthorizedKeysScriptIsIdempotent(t *testing.T) {
	current, err := user.Current()
	require.NoError(t, err)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	passwd := filepath.Join(dir, "passwd")
	require.NoError(t, os.WriteFile(passwd, []byte(fmt.Sprintf("%s:x:1:1::%s:/bin/sh\n", current.Username, home)), 0600))
	script := strings.ReplaceAll(kInstallAuthorizedKeysScript, "/etc/passwd", passwd)

	install := func(keys string) {
		t.Helper()
		out, err := exec.Command("/bin/sh", "-c", script, "sh", current.Username, keys).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	install("ssh-ed25519 AAAA1 a@b\nssh-ed25519 AAAA2 c@d")
	install("ssh-ed25519 AAAA2 c@d")

	authorizedKeys := filepath.Join(home, ".ssh", "authorized_keys")
	content, err := os.ReadFile(authorizedKeys)
	require.NoError(t, err)
	assert.Equal(t, "ssh-ed25519 AAAA1 a@b\nssh-ed25519 AAAA2 c@d\n", string(content))
	info, err := os.Stat(authorizedKeys)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestInstallAuthorizedKeysScriptHandlesMissingTrailingNewline(t *testing.T) {
	// Reviewer P2: an existing key without a trailing newline must not get the
	// new key appended onto its line; both must end up as separate entries.
	current, err := user.Current()
	require.NoError(t, err)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	passwd := filepath.Join(dir, "passwd")
	require.NoError(t, os.WriteFile(passwd, []byte(fmt.Sprintf("%s:x:1:1::%s:/bin/sh\n", current.Username, home)), 0600))
	// seed an authorized_keys whose single key has no trailing newline
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0700))
	authorizedKeys := filepath.Join(home, ".ssh", "authorized_keys")
	require.NoError(t, os.WriteFile(authorizedKeys, []byte("ssh-ed25519 AAAA1 a@b"), 0600))

	script := strings.ReplaceAll(kInstallAuthorizedKeysScript, "/etc/passwd", passwd)
	out, err := exec.Command("/bin/sh", "-c", script, "sh", current.Username, "ssh-ed25519 AAAA2 c@d").CombinedOutput()
	require.NoError(t, err, string(out))

	content, err := os.ReadFile(authorizedKeys)
	require.NoError(t, err)
	assert.Equal(t, "ssh-ed25519 AAAA1 a@b\nssh-ed25519 AAAA2 c@d\n", string(content),
		"the new key is a separate entry, not appended to the unterminated line")
}

func TestEnsureContainerUserScriptParsesUserGroupSpec(t *testing.T) {
	// Reviewer P2: a "name:group" (or uid:gid) spec must be split so the whole
	// spec is not handed to id/useradd as a single account name.
	script := ensureContainerUserScript("nobody:nogroup")
	assert.Contains(t, script, "spec='nobody:nogroup'")
	assert.Contains(t, script, "user=${spec%%:*}", "the account is the part before ':'")
	assert.Contains(t, script, "group=${spec#*:}", "the group is the part after ':'")
	// an existing account is detected and not recreated
	assert.Contains(t, script, "if ! account_exists; then")
}

func TestUniqueNonRootUsersSkipsRootAndGroupSpecs(t *testing.T) {
	assert.Equal(t, []string{"developer"}, uniqueNonRootUsers("developer", "developer"))
	assert.Empty(t, uniqueNonRootUsers("root", "0", "root:root", "0:0"))
	assert.Equal(t, []string{"nobody:nogroup"}, uniqueNonRootUsers("nobody:nogroup", "root"))
}

func TestContainerRunToleratesNonStringRunArgsAndPublishesNumericPorts(t *testing.T) {
	// a numeric runArg must be skipped (not panic) and a numeric appPort must be
	// published like its string form
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.20")
	config.Set(containerconf.KRunArgs, []interface{}{1, "-t"})
	config.Set(containerconf.KAppPort, []interface{}{18080, "9090:90"})

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")

	cmds, err := ce.BuildCommands()
	require.NoError(t, err)
	runIndex := findCommandIndexWithPrefix(cmds, "podman run ")
	require.NotEqual(t, -1, runIndex)
	run := cmds[runIndex].Cmd()
	assert.Contains(t, run, "-t", "the valid string runArg is kept")
	assert.Contains(t, run, "-p 18080:18080", "the numeric appPort maps to the same host port")
	assert.Contains(t, run, "-p 9090:90", "the string appPort is published")
}

func TestNormalizeAppPorts(t *testing.T) {
	mk := func(v interface{}) *containerconf.Config {
		c := containerconf.NewConfig()
		c.Set(containerconf.KAppPort, v)
		return c
	}
	assert.Equal(t, []string{"18080:18080"}, normalizeAppPorts(mk([]interface{}{18080})))
	assert.Equal(t, []string{"8080:80"}, normalizeAppPorts(mk([]interface{}{"8080:80"})))
	assert.Equal(t, []string{"18080:18080"}, normalizeAppPorts(mk(18080)))          // scalar number -> same host port
	assert.Equal(t, []string{"18080"}, normalizeAppPorts(mk("18080")))              // scalar string unchanged
	assert.Empty(t, normalizeAppPorts(mk([]interface{}{map[string]interface{}{}}))) // unsupported skipped
	assert.Nil(t, normalizeAppPorts(containerconf.NewConfig()))                     // unset
}

func TestPostCreateCommandIsQuotedAsSingleArgument(t *testing.T) {
	// P2: `echo 'hello world'` must reach the container intact, not be re-split
	config := containerconf.NewConfig()
	config.Set(containerconf.KImage, "alpine:3.20")
	config.Set(containerconf.KPostCreateCommand, "echo 'hello world'")

	ce := NewContainerEngine(WithContainerConfig(config))
	ce.SetAction(K_ACTION_RUN)
	ce.SetRunType(K_RUN_DEV_CONTAINER)
	ce.SetContainerName("sample-container")
	cmds, err := ce.BuildCommands()
	require.NoError(t, err)

	idx := findCommandIndexWithDescription(cmds, "Running devcontainer postCreateCommand")
	require.NotEqual(t, -1, idx)
	cmd := cmds[idx].Cmd()
	// the whole command is a single-quoted argument; inner quotes are escaped
	assert.Contains(t, cmd, cg.ShellSingleQuote("echo 'hello world'"))
	// run it through a fake podman to prove the container receives one argument
	argv := runWithFakePodman(t, cmd)
	// last argv element is the full command, unchanged
	assert.Equal(t, "echo 'hello world'", argv[len(argv)-1])
}

func TestCustomCommandIsPassedAsSingleArgument(t *testing.T) {
	ce := NewContainerEngine()
	ce.SetAction(K_ACTION_EXE)
	ce.SetExecuteCmd(K_EXEC_CUSTOM_CMD)
	ce.SetContainerName("container-a")
	ce.SetRemoteUser("developer")
	ce.SetCustomCommand(`echo 'quoted' "$HOME"; exit 2`)

	cmds, err := ce.BuildCommands()
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	argv := runWithFakePodman(t, cmds[0].Cmd())
	assert.Equal(t, []string{"exec", "-u", "developer", "-i", "container-a", "/bin/sh", "-c", `echo 'quoted' "$HOME"; exit 2`}, argv)
}

// runWithFakePodman runs cmd with bash, using a fake podman recording its argv.
func runWithFakePodman(t *testing.T, cmd string) []string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv")
	script := fmt.Sprintf("#!/bin/sh\nfor arg in \"$@\"; do printf '%%s\\0' \"$arg\"; done > %q\n", logPath)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "podman"), []byte(script), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	require.NoError(t, err, string(out))
	logged, err := os.ReadFile(logPath)
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(logged), "\x00"), "\x00")
}

func TestDeleteContainerPassesContainerNameOnce(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "podman.log")
	podmanPath := filepath.Join(dir, "podman")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
count=0
for arg in "$@"; do
  if [ "$arg" = "sample-container" ]; then
    count=$((count + 1))
  fi
done
if [ "$count" -ne 1 ]; then
  echo "container name passed $count times" >&2
  exit 1
fi
`, logPath)
	assert.NoError(t, os.WriteFile(podmanPath, []byte(script), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := DeleteContainer("sample-container")

	assert.NoError(t, err)
	logged, readErr := os.ReadFile(logPath)
	assert.NoError(t, readErr)
	assert.Equal(t, "rm -f sample-container\n", string(logged))
}

func findCommandWithPrefix(cmds []shexec.ExecuteEvent, prefix string) string {
	for _, cmd := range cmds {
		if strings.HasPrefix(cmd.Cmd(), prefix) {
			return cmd.Cmd()
		}
	}
	return ""
}

func findCommandIndexWithPrefix(cmds []shexec.ExecuteEvent, prefix string) int {
	for index, cmd := range cmds {
		if strings.HasPrefix(cmd.Cmd(), prefix) {
			return index
		}
	}
	return -1
}

func findCommandIndexWithDescription(cmds []shexec.ExecuteEvent, description string) int {
	for index, cmd := range cmds {
		if cmd.Description() == description {
			return index
		}
	}
	return -1
}

func TestRemoveAuthorizedKeysCommand(t *testing.T) {
	current, err := user.Current()
	require.NoError(t, err)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	passwd := filepath.Join(dir, "passwd")
	require.NoError(t, os.WriteFile(passwd, []byte(fmt.Sprintf("%s:x:1:1::%s:/bin/sh\n", current.Username, home)), 0600))
	run := func(install bool, keys string) {
		t.Helper()
		cmd := strings.ReplaceAll(AuthorizedKeysCommand(install, current.Username, keys), "/etc/passwd", passwd)
		out, err := exec.Command("/bin/sh", "-c", cmd).CombinedOutput()
		require.NoError(t, err, string(out))
	}

	run(false, "ssh-ed25519 AAAA1 a@b") // nothing installed yet
	run(true, "ssh-ed25519 AAAA1 a@b\nssh-ed25519 AAAA2 c@d\nssh-ed25519 AAAA3 e@f")
	run(false, "ssh-ed25519 AAAA2 c@d\nssh-ed25519 AAAA3 e@f")

	content, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	require.NoError(t, err)
	assert.Equal(t, "ssh-ed25519 AAAA1 a@b\n", string(content))
}
