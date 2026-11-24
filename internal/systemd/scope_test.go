package systemd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransientScope: outside systemd the command is unchanged; under it, the
// command runs in a quoted transient scope with the given properties (CDS-058).
func TestTransientScope(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX systemd-run stub")
	}
	t.Setenv("INVOCATION_ID", "")
	cmd, changed := TransientScope("podman start x")
	assert.False(t, changed)
	assert.Equal(t, "podman start x", cmd)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "systemd-run"), []byte("#!/bin/sh\nexit 0\n"), 0755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("INVOCATION_ID", "test")

	cmd, changed = TransientScope("echo 'a'")
	assert.True(t, changed)
	assert.Equal(t, `systemd-run --user --scope --collect --quiet -- bash -c 'echo '\''a'\'''`, cmd)

	cmd, _ = TransientScope("kind create cluster", "Delegate=yes")
	assert.Equal(t, "systemd-run --user --scope --collect --quiet -p Delegate=yes -- bash -c 'kind create cluster'", cmd)
}
