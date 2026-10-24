package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
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

			output, code, err := defaultContainerOps{}.ExecuteInContainer("dev-1", "echo 'hi'", "developer")

			require.NoError(t, err)
			assert.Equal(t, tc.wantOutput, output)
			assert.Equal(t, tc.wantCode, code)
			assert.Equal(t, "exec -u developer -i dev-1 /bin/sh -c echo 'hi'\n", readLog(t, logPath))
		})
	}
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
