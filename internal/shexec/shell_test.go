package shexec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLocalCmdWithOutput(t *testing.T) {
	stdout, err := RunLocalCmdWithOutput([]ExecuteEvent{&DefaultShEvent{ExeCmd: "echo hello"}})

	if err != nil && stdout != "hello" {
		t.Errorf("com.RunLocalCmdWithOutput: Failed to run hello cmd: stdout %s, err: %s", stdout, err)
	}

	stdout, err = RunLocalCmdWithOutput([]ExecuteEvent{&DefaultShEvent{ExeCmd: "echoerror"}})
	commandNotFound := strings.Contains(stdout, "echoerror: command not found") || strings.Contains(stdout, "'echoerror' is not recognized as an internal or external command")
	if err == nil || (!commandNotFound) {
		t.Errorf("com.RunLocalCmdWithOutput: Failed crash on wrong cmd: stdout %s, err: %s", stdout, err)
	}
}

func TestRunLocalCmdWithExitCode(t *testing.T) {
	for name, tc := range map[string]struct {
		cmd        string
		wantOutput string
		wantCode   int
	}{
		"success":                       {cmd: "echo hello", wantOutput: "hello\n", wantCode: 0},
		"non-zero exit is not an error": {cmd: "echo out; echo err >&2; exit 3", wantOutput: "out\nerr\n", wantCode: 3},
	} {
		t.Run(name, func(t *testing.T) {
			output, code, err := RunLocalCmdWithExitCode(&DefaultShEvent{ExeCmd: tc.cmd})

			require.NoError(t, err)
			assert.Equal(t, tc.wantOutput, output)
			assert.Equal(t, tc.wantCode, code)
		})
	}
}

func TestRunLocalCmdWithStdinFeedsSecretOffCommandLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	const secret = "pw-on-stdin"
	in := func(cmd string) ExecuteEvent {
		return &StdinShEvent{ExecuteEvent: &DefaultShEvent{ExeCmd: cmd}, Input: []byte(secret)}
	}
	out, err := RunLocalCmdWithOutput([]ExecuteEvent{in("cat")})
	require.NoError(t, err)
	assert.Equal(t, secret, out)

	out, code, err := RunLocalCmdWithExitCode(in("cat; exit 3"))
	require.NoError(t, err)
	assert.Equal(t, secret, out)
	assert.Equal(t, 3, code)

	// a failing command's error names the command, not the stdin content
	_, err = RunLocalCmdWithOutput([]ExecuteEvent{in("cat >/dev/null; exit 2")})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
}

// TestRunLocalCmdsFeedsStdin: RunLocalCmds feeds the input of a stdin event
// too, so `sh -s` never runs an empty script. Real bash.
func TestRunLocalCmdsFeedsStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	out := filepath.Join(t.TempDir(), "out")
	wrapped := KeepStdin(
		&StdinShEvent{ExecuteEvent: &DefaultShEvent{ExeCmd: "unused"}, Input: []byte("echo fed > " + out)},
		&DefaultShEvent{ExeCmd: "sh -s"},
	)
	require.NoError(t, RunLocalCmds([]ExecuteEvent{wrapped}))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "fed\n", string(got))
}

// TestKeepStdin: a wrapper that rebuilds the command keeps the input, and
// only an event that has input gets one.
func TestKeepStdin(t *testing.T) {
	to := &DefaultShEvent{ExeCmd: "wrapped"}
	in, ok := StdinOf(KeepStdin(&StdinShEvent{ExecuteEvent: &DefaultShEvent{}, Input: []byte("x")}, to))
	assert.True(t, ok)
	assert.Equal(t, []byte("x"), in)
	assert.Same(t, to, KeepStdin(&DefaultShEvent{}, to))
}

// TestRemoteExecutorsRejectStdin: the ssh executors cannot feed stdin, so a
// stdin event fails before any connection instead of running an empty script
// and reporting success.
func TestRemoteExecutorsRejectStdin(t *testing.T) {
	cmd := &StdinShEvent{ExecuteEvent: &DefaultShEvent{ExeCmd: "sh -s"}, Input: []byte("echo hi")}
	noConnect := func() (target, error) {
		t.Fatal("must fail before connecting")
		return nil, nil
	}
	for name, method := range map[string]RemoteExecute{
		"key":      keyExecuteCallback(noConnect),
		"password": passwordExecuteCallback(noConnect),
	} {
		t.Run(name, func(t *testing.T) {
			out, errs := method.execute([]any{&DefaultShEvent{ExeCmd: "true"}, cmd}, nil)
			assert.Nil(t, out)
			require.Len(t, errs, 1)
			assert.Contains(t, errs[0].Error(), "cannot feed stdin")
			assert.Contains(t, errs[0].Error(), "sh -s")
			assert.NotContains(t, errs[0].Error(), "echo hi")
		})
	}
}

// TestIsProbe: only a ProbeShEvent is a probe, so only its failure is quiet
// (no spinner, debug log); every other command still warns.
func TestIsProbe(t *testing.T) {
	assert.True(t, isProbe(&ProbeShEvent{DefaultShEvent: DefaultShEvent{ExeCmd: "systemctl --user is-active cds.service"}}))
	assert.False(t, isProbe(&DefaultShEvent{ExeCmd: "systemctl --user daemon-reload"}))
}
