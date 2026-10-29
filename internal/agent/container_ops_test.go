package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
			// the command exports a cancellation token, then exec's the user command
			// (under setsid when available)
			logged := readLog(t, logPath)
			assert.True(t, strings.HasPrefix(logged, "exec -u developer -i dev-1 /bin/sh -c export CDS_EXEC_TOKEN=cds-exec-"), logged)
			assert.Contains(t, logged, "setsid echo 'hi'", logged)
		})
	}
}

func TestDefaultContainerOpsExecuteCancelsOnContext(t *testing.T) {
	// a slow command is aborted when the context deadline is reached, well before
	// it would finish naturally (the WaitDelay bounds any grandchild-held pipe).
	// the cleanup exec (the /proc-scan kill script) returns immediately so stop()
	// is not delayed.
	fakePodman(t, `case "$*" in *CDS_EXEC_TOKEN*) exit 0 ;; *) sleep 5 ;; esac`)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := defaultContainerOps{}.ExecuteInContainer(ctx, "dev-1", "sleep 5", "root")

	require.Error(t, err)
	assert.Less(t, time.Since(start), 4*time.Second, "the command is killed well before its 5s runtime")
}

// TestExecuteInContainerKillsInContainerOnCancel covers P2: on cancellation the
// agent issues a podman exec that kills every in-container process carrying the
// exec token in its environment, so the workload cannot keep running past the
// deadline — regardless of command shape.
func TestExecuteInContainerKillsInContainerOnCancel(t *testing.T) {
	// fake podman: the first exec (the actual command) sleeps so it is still
	// running when the deadline fires; the kill exec (carrying the token in its
	// script) returns immediately; every invocation is logged
	logPath := fakePodman(t, `case "$*" in *CDS_EXEC_TOKEN*) exit 0 ;; *) sleep 5 ;; esac`)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, _, err := defaultContainerOps{}.ExecuteInContainer(ctx, "dev-1", "sleep 5", "root")
	require.Error(t, err)

	// give the kill watcher a moment to issue the kill exec, which scans
	// /proc/<pid>/environ for the token
	assert.Eventually(t, func() bool {
		data, _ := os.ReadFile(logPath)
		s := string(data)
		return strings.Contains(s, "environ") && strings.Contains(s, "CDS_EXEC_TOKEN=cds-exec-")
	}, 3*time.Second, 50*time.Millisecond, "a kill scanning /proc for the exec token was issued into the container")
}

func TestKillByEnvTokenScriptMatchesEnvironNotCmdlineAndSkipsSelf(t *testing.T) {
	s := killByEnvTokenScript("cds-exec-abc")
	assert.Contains(t, s, "/environ")
	assert.Contains(t, s, "CDS_EXEC_TOKEN=cds-exec-abc")
	assert.Contains(t, s, `[ "$pid" = "$self" ] && continue`, "the killer skips its own pid")
	assert.NotContains(t, s, "pkill", "does not depend on pkill")
	// kills the whole process group (negative pgid) so trapped/children die too
	assert.Contains(t, s, `kill -TERM "-$g"`)
	assert.Contains(t, s, `kill -KILL "-$g"`)
}

func TestExecTokenIsUnique(t *testing.T) {
	assert.NotEqual(t, execToken(), execToken())
	assert.True(t, strings.HasPrefix(execToken(), "cds-exec-"))
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

// TestBoundedOutputKeepsValidUTF8 covers P2: the tail must start on a rune
// boundary so the bounded string is valid UTF-8 (a protobuf string rejects
// invalid UTF-8). Fill just over the limit with a multibyte rune.
func TestBoundedOutputKeepsValidUTF8(t *testing.T) {
	euro := strings.Repeat("€", kMaxDeployOutput) // 3 bytes each, well over the limit
	got := boundedOutput(euro)
	assert.True(t, utf8.ValidString(got), "bounded output must be valid UTF-8")
}

func TestBoundedHeadKeepsValidUTF8(t *testing.T) {
	assert.Equal(t, "short", boundedHead("short"))
	euro := strings.Repeat("€", kMaxDeployOutput)
	got := boundedHead(euro)
	assert.True(t, utf8.ValidString(got), "bounded head must be valid UTF-8")
	assert.Contains(t, got, "truncated")
}

// TestBoundingSanitizesInvalidUTF8 covers P2: binary / non-UTF-8 output must be
// sanitized to a valid protobuf string, not rejected on send.
func TestBoundingSanitizesInvalidUTF8(t *testing.T) {
	invalid := "ok\xff\xfeend"
	assert.True(t, utf8.ValidString(boundedOutput(invalid)))
	assert.True(t, utf8.ValidString(boundedHead(invalid)))
	assert.True(t, utf8.ValidString(validUTF8(invalid)))
	// a Latin-1 'é' (0xE9) is replaced, not dropped silently
	assert.True(t, utf8.ValidString(boundedHead("caf\xe9")))
}

func TestReportCommandOutputSanitizesInvalidUTF8(t *testing.T) {
	var chunks []string
	report := func(p deployProgress) error {
		if p.phase == "output" {
			chunks = append(chunks, p.message)
		}
		return nil
	}
	require.NoError(t, reportCommandOutput(report, "before\xffafter"))
	require.NotEmpty(t, chunks)
	for _, c := range chunks {
		assert.True(t, utf8.ValidString(c), "each streamed chunk is valid UTF-8")
	}
}

func TestReportCommandOutputChunksKeepValidUTF8(t *testing.T) {
	var chunks []string
	report := func(p deployProgress) error {
		if p.phase == "output" {
			chunks = append(chunks, p.message)
		}
		return nil
	}
	// a multibyte stream longer than one chunk must split on rune boundaries
	require.NoError(t, reportCommandOutput(report, strings.Repeat("€", kMaxDeployOutputChunk)))
	require.NotEmpty(t, chunks)
	for _, c := range chunks {
		assert.True(t, utf8.ValidString(c), "each streamed chunk is valid UTF-8")
	}
	assert.Equal(t, strings.Repeat("€", kMaxDeployOutputChunk), strings.Join(chunks, ""), "no bytes lost")
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

// TestStartContainerUsesTransientScopeUnderSystemd covers P1: podman start must
// run in a transient scope too, so the helpers it respawns do not land in
// cds.service and get killed on the next agent restart.
func TestStartContainerUsesTransientScopeUnderSystemd(t *testing.T) {
	logPath := fakePodman(t, "")
	t.Setenv("INVOCATION_ID", "abc")
	sdLog := fakeSystemdRun(t)

	require.NoError(t, defaultContainerOps{}.StartContainer("dev-1"))

	// start was handed to systemd-run --scope, carrying the podman start command
	require.FileExists(t, sdLog, "systemd-run was invoked")
	sd, err := os.ReadFile(sdLog)
	require.NoError(t, err)
	assert.Contains(t, string(sd), "--scope")
	assert.Contains(t, string(sd), "podman start dev-1")
	// podman start was not run directly (the fake systemd-run does not exec it)
	if data, err := os.ReadFile(logPath); err == nil {
		assert.NotContains(t, string(data), "start dev-1", "podman start was wrapped in a transient scope, not run directly")
	}
}

func TestStartContainerRunsDirectlyWithoutSystemd(t *testing.T) {
	logPath := fakePodman(t, "")
	t.Setenv("INVOCATION_ID", "") // not under systemd -> no scope wrapping

	require.NoError(t, defaultContainerOps{}.StartContainer("dev-1"))
	assert.Contains(t, readLog(t, logPath), "start dev-1")
}

// TestDeployContainerCompensatesWhenOutputSendFailsAfterCreation covers P1: if
// reporting the create command's output fails, the just-created container must
// still be cleaned up (ownership is established before the output is reported).
func TestDeployContainerCompensatesWhenOutputSendFailsAfterCreation(t *testing.T) {
	// podman prints a container id on create (non-empty stdout), everything succeeds
	logPath := fakePodman(t, `case "$*" in run*) echo "container-id-123" ;; *) : ;; esac`)
	spec := deploySpecForTest(t, "dev-reportfail", `{"image":"ubuntu:24.04"}`)

	// the reporter fails on the first "output" event (the create command's stdout)
	report := func(p deployProgress) error {
		if p.phase == "output" {
			return os.ErrClosed
		}
		return nil
	}
	_, err := defaultContainerOps{}.DeployContainer(context.Background(), spec, report)

	require.Error(t, err)
	assert.Contains(t, readLog(t, logPath), "rm -f dev-reportfail",
		"the created container is removed even though the failure was reporting its output")
}

// TestDeployContainerRemovesOrphanOnFailedCreate covers P2: when `podman run`
// fails after creating the container (e.g. port in use), the orphan is removed
// — but only when a cds-labelled container of that name is present.
func TestDeployContainerRemovesOrphanOnFailedCreate(t *testing.T) {
	// run fails; ps (ListContainers) reports the orphan exists with the name;
	// rm is expected. Everything is logged.
	logPath := fakePodman(t, `case "$*" in
  *"ps -a"*) echo 'ID=o1,Name=dev-orphan,Status=Created;' ;;
  run*) echo "bind: address already in use" >&2; exit 125 ;;
  *) exit 0 ;;
esac`)
	spec := deploySpecForTest(t, "dev-orphan", `{"image":"ubuntu:24.04"}`)
	_, err := defaultContainerOps{}.DeployContainer(context.Background(), spec, func(deployProgress) error { return nil })

	require.Error(t, err)
	assert.Contains(t, readLog(t, logPath), "rm -f dev-orphan", "the orphaned cds container is removed")
}

func TestDeployContainerDoesNotRemoveOnFailedCreateWhenNoOrphan(t *testing.T) {
	// run fails and ps reports no matching cds container -> nothing is removed
	// (a genuine name collision with a non-cds container must be left alone)
	logPath := fakePodman(t, `case "$*" in
  *"ps -a"*) echo '' ;;
  run*) echo "name already in use" >&2; exit 125 ;;
  *) exit 0 ;;
esac`)
	spec := deploySpecForTest(t, "dev-collision", `{"image":"ubuntu:24.04"}`)
	_, err := defaultContainerOps{}.DeployContainer(context.Background(), spec, func(deployProgress) error { return nil })

	require.Error(t, err)
	assert.NotContains(t, readLog(t, logPath), "rm -f dev-collision", "a pre-existing non-cds container is not removed")
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

// fakeSystemdRun puts a systemd-run stub on PATH (so systemdRunAvailable is
// true) that records its arguments and does NOT run them, and returns the log
// path. Recording-not-running lets a test assert the wrapped command without
// the inner podman being executed.
func fakeSystemdRun(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "systemd-run.log")
	stub := "#!/bin/sh\necho \"$*\" >> '" + logPath + "'\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "systemd-run"), []byte(stub), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// deploySpecForTest builds a minimal deploy spec with the given devcontainer
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
