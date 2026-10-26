package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePodman installs a podman stub on PATH running script, and returns the
// file where its arguments are logged (one invocation per line).
func fakePodman(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "podman.log")
	stub := "#!/bin/sh\necho \"$*\" >> '" + logPath + "'\n" + script + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "podman"), []byte(stub), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestDefaultContainerOpsListsCDSContainersOnly(t *testing.T) {
	logPath := fakePodman(t, `echo 'ID=abc,Name=dev-1,Status=Up 2 hours;ID=def,Name=dev-2,Status=Exited (0);'`)

	containers, err := defaultContainerOps{}.ListContainers()

	require.NoError(t, err)
	assert.Equal(t, bo.Containers{
		{Id: "abc", Name: "dev-1", Status: bo.KContainerStatusRunning},
		{Id: "def", Name: "dev-2", Status: bo.KContainerStatusExited},
	}, containers)
	assert.Contains(t, readLog(t, logPath), "ps -a --filter label=io.cds=cds --format")
}

func TestDefaultContainerOpsExecuteReturnsExitCode(t *testing.T) {
	for name, tc := range map[string]struct {
		script     string
		wantOutput string
		wantCode   int
	}{
		"success":           {script: "echo done", wantOutput: "done\n", wantCode: 0},
		"command exit code": {script: "echo failed >&2; exit 42", wantOutput: "failed\n", wantCode: 42},
		"engine error code": {script: "echo 'no such container' >&2; exit 125", wantOutput: "no such container\n", wantCode: 125},
	} {
		t.Run(name, func(t *testing.T) {
			logPath := fakePodman(t, tc.script)

			output, code, err := defaultContainerOps{}.ExecuteInContainer(context.Background(), "dev-1", "echo 'hi'", "developer")

			require.NoError(t, err)
			assert.Equal(t, tc.wantOutput, output)
			assert.Equal(t, tc.wantCode, code)
			assert.Equal(t, "exec -u developer -i dev-1 /bin/sh -c echo 'hi'\n", readLog(t, logPath))
		})
	}
}

func TestDefaultContainerOpsExecuteCancelsOnContext(t *testing.T) {
	// a slow command is aborted when the context deadline is reached, well before
	// it would finish naturally (the WaitDelay bounds any grandchild-held pipe)
	fakePodman(t, "sleep 5")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := defaultContainerOps{}.ExecuteInContainer(ctx, "dev-1", "sleep 5", "root")

	require.Error(t, err)
	assert.Less(t, time.Since(start), 4*time.Second, "the command is killed well before its 5s runtime")
}

func TestDeployContainerStopsOnCancelledContext(t *testing.T) {
	fakePodman(t, "exit 0")
	spec := deploySpecForTest(t, "dev-cancel", `{"image":"ubuntu:24.04"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the first command runs

	_, err := defaultContainerOps{}.DeployContainer(ctx, spec, func(deployProgress) error { return nil })

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

func TestBoundedOutput(t *testing.T) {
	assert.Equal(t, "short", boundedOutput("short"))

	big := strings.Repeat("x", kMaxDeployOutput+100)
	got := boundedOutput(big)
	assert.LessOrEqual(t, len(got), kMaxDeployOutput+64, "output is bounded (plus a short marker)")
	assert.Contains(t, got, "truncated")
	assert.True(t, strings.HasSuffix(got, strings.Repeat("x", 10)), "the tail is kept")
}

func TestReportCommandOutputChunksBoundedly(t *testing.T) {
	var chunks []string
	report := func(p deployProgress) error {
		if p.phase == "output" {
			chunks = append(chunks, p.message)
		}
		return nil
	}
	require.NoError(t, reportCommandOutput(report, strings.Repeat("y", kMaxDeployOutputChunk*2+5)))

	require.Len(t, chunks, 3)
	assert.Len(t, chunks[0], kMaxDeployOutputChunk)
	assert.Len(t, chunks[1], kMaxDeployOutputChunk)
	assert.Len(t, chunks[2], 5)
}

func TestReportCommandOutputSkipsEmpty(t *testing.T) {
	called := false
	require.NoError(t, reportCommandOutput(func(deployProgress) error { called = true; return nil }, ""))
	assert.False(t, called, "no event is sent for empty output")
}

func TestDefaultContainerOpsLifecycle(t *testing.T) {
	logPath := fakePodman(t, "")
	ops := defaultContainerOps{}

	require.NoError(t, ops.StartContainer("dev-1"))
	require.NoError(t, ops.StopContainer("dev-1"))
	require.NoError(t, ops.RenameContainer("dev-1", "dev-2"))
	require.NoError(t, ops.DeleteContainer("dev-2"))

	lines := strings.Split(strings.TrimSpace(readLog(t, logPath)), "\n")
	require.Len(t, lines, 4)
	assert.True(t, strings.HasPrefix(lines[0], "start"), lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "stop"), lines[1])
	assert.Equal(t, "rename dev-1 dev-2", lines[2])
	assert.Equal(t, "rm -f dev-2", lines[3])
}

// TestDetachContainerHelpersWrapsOnlyUnderSystemd covers P1: the
// container-creating command is wrapped in a transient systemd scope so the
// container runtime helpers survive an agent service restart. The wrapping only
// applies to the creating command, and only when the agent runs under systemd
// with systemd-run available.
func TestDetachContainerHelpersWrapsOnlyUnderSystemd(t *testing.T) {
	creating := fakeCreatingEvent{cmd: "podman run --name dev", creates: true}
	nonCreating := fakeCreatingEvent{cmd: "podman exec dev true", creates: false}

	t.Run("not under systemd: command is returned unchanged", func(t *testing.T) {
		t.Setenv("INVOCATION_ID", "")
		assert.Equal(t, creating.cmd, detachContainerHelpers(creating).Cmd())
	})

	t.Run("under systemd: non-creating command is unchanged", func(t *testing.T) {
		t.Setenv("INVOCATION_ID", "abc")
		assert.Equal(t, nonCreating.cmd, detachContainerHelpers(nonCreating).Cmd())
	})

	t.Run("under systemd with systemd-run: creating command is scoped", func(t *testing.T) {
		t.Setenv("INVOCATION_ID", "abc")
		fakeSystemdRun(t)
		wrapped := detachContainerHelpers(creating)
		assert.NotEqual(t, creating.cmd, wrapped.Cmd())
		assert.Contains(t, wrapped.Cmd(), "systemd-run --user --scope --collect")
		assert.Contains(t, wrapped.Cmd(), creating.cmd)
	})
}

// fakeCreatingEvent is a minimal ExecuteEvent exposing CreatesContainer for the
// detach logic, standing in for an engine run event.
type fakeCreatingEvent struct {
	cmd     string
	creates bool
}

func (e fakeCreatingEvent) Cmd() string            { return e.cmd }
func (e fakeCreatingEvent) Description() string    { return "" }
func (e fakeCreatingEvent) Recoverable() bool      { return false }
func (e fakeCreatingEvent) CarryOn() bool          { return false }
func (e fakeCreatingEvent) Recover() string        { return "" }
func (e fakeCreatingEvent) CreatesContainer() bool { return e.creates }

// fakeSystemdRun puts a no-op systemd-run on PATH so systemdRunAvailable is true.
func fakeSystemdRun(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "systemd-run"), []byte("#!/bin/sh\n"), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// configuration, parsed the same way the agent does. It isolates HOME so that
// engine side effects (e.g. the default ~/.devbox mount directory) land in a
// throwaway directory rather than the real home.
func deploySpecForTest(t *testing.T, name, config string) deployContainerSpec {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := containerconf.ParseBytes(strings.NewReader(config))
	require.NoError(t, err)
	return deployContainerSpec{
		projectName:   "proj",
		containerName: name,
		config:        cfg,
	}
}

// TestDeployContainerCompensatesAfterCreation covers P1: when a command fails
// after the container was created, the created container is removed so it does
// not linger unmanaged (the client only records it on success).
func TestDeployContainerCompensatesAfterCreation(t *testing.T) {
	// Fail the post-create exec, succeed everything else. The container is
	// created (run) first, so the failure must trigger its removal.
	logPath := fakePodman(t, `case "$*" in
  *"postCreateCommand"*|*"exit 42"*) echo "boom" >&2; exit 42 ;;
  *) exit 0 ;;
esac`)

	spec := deploySpecForTest(t, "dev-fail", `{"image":"ubuntu:24.04","postCreateCommand":"exit 42"}`)
	_, err := defaultContainerOps{}.DeployContainer(context.Background(), spec, func(deployProgress) error { return nil })

	require.Error(t, err)
	assert.Contains(t, readLog(t, logPath), "rm -f dev-fail", "the created container is removed after the failure")
}

// TestDeployContainerDoesNotRemoveOnCreationFailure covers P1: a creation
// failure (e.g. a name collision with a pre-existing container) must not remove
// that other container.
func TestDeployContainerDoesNotRemoveOnCreationFailure(t *testing.T) {
	// The very first command (run/create) fails; nothing was created by us.
	logPath := fakePodman(t, `case "$*" in
  run*) echo "name already in use" >&2; exit 125 ;;
  *) exit 0 ;;
esac`)

	spec := deploySpecForTest(t, "dev-collision", `{"image":"ubuntu:24.04"}`)
	_, err := defaultContainerOps{}.DeployContainer(context.Background(), spec, func(deployProgress) error { return nil })

	require.Error(t, err)
	assert.NotContains(t, readLog(t, logPath), "rm -f dev-collision", "a pre-existing container must not be removed on a creation failure")
}
