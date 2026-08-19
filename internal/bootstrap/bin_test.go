package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveAgentBinaryPathUsesEnvOverride(t *testing.T) {
	agentPath := writeExecutable(t, t.TempDir(), "custom-agent")
	t.Setenv(kAPIAgentPathEnvVar, agentPath)
	t.Setenv("PATH", t.TempDir())

	resolvedPath, err := resolveAgentBinaryPath()

	require.NoError(t, err)
	require.Equal(t, agentPath, resolvedPath)
}

func TestResolveAgentBinaryPathFindsRepoRootBinary(t *testing.T) {
	repositoryRoot := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(repositoryRoot, "go.mod"),
		[]byte("module github.com/amadeusitgroup/cds\n"),
		0o644,
	))
	agentPath := writeExecutable(t, repositoryRoot, kAPIAgentBinaryName)
	nestedDir := filepath.Join(repositoryRoot, "internal", "bootstrap")
	require.NoError(t, os.MkdirAll(nestedDir, 0o755))
	t.Chdir(nestedDir)
	t.Setenv(kAPIAgentPathEnvVar, "")
	t.Setenv("PATH", t.TempDir())

	resolvedPath, err := resolveAgentBinaryPath()

	require.NoError(t, err)
	require.Equal(t, agentPath, resolvedPath)
}

func TestResolveAgentBinaryPathRejectsInvalidEnvOverride(t *testing.T) {
	t.Setenv(kAPIAgentPathEnvVar, filepath.Join(t.TempDir(), "missing-agent"))
	t.Setenv("PATH", t.TempDir())

	_, err := resolveAgentBinaryPath()

	require.Error(t, err)
	require.Contains(t, err.Error(), kAPIAgentPathEnvVar)
}

func writeExecutable(t *testing.T, dir string, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))
	return path
}
