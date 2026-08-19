package bootstrap

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAgentCommandUsesExplicitEnvOverride(t *testing.T) {
	restoreAgentCommandTestHooks(t)
	t.Setenv(agentBinaryEnv, "/tmp/custom-agent")

	cmd, err := resolveAgentCommand()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/custom-agent", cmd.name)
	assert.Empty(t, cmd.args)
}

func TestResolveAgentCommandFindsSiblingBinaryForLocalDevelopment(t *testing.T) {
	restoreAgentCommandTestHooks(t)
	agentLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, agentBinaryName)
	require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0755))
	agentExecutable = func() (string, error) {
		return filepath.Join(dir, "cds"), nil
	}

	cmd, err := resolveAgentCommand()
	require.NoError(t, err)
	assert.Equal(t, binaryPath, cmd.name)
	assert.Empty(t, cmd.args)
}

func TestResolveAgentCommandFallsBackToGoRunFromRepository(t *testing.T) {
	restoreAgentCommandTestHooks(t)
	agentLookPath = func(name string) (string, error) {
		if name == "go" {
			return "/usr/bin/go", nil
		}
		return "", exec.ErrNotFound
	}
	agentExecutable = func() (string, error) {
		return "", errors.New("no executable")
	}

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module github.com/amadeusitgroup/cds\n"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "cmd", "api-agent"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "cmd", "api-agent", "cds-api-agent.go"), []byte("package main\n"), 0600))
	workDir := filepath.Join(repoDir, "internal", "bootstrap")
	require.NoError(t, os.MkdirAll(workDir, 0755))
	agentWorkingDir = func() (string, error) {
		return workDir, nil
	}

	cmd, err := resolveAgentCommand()
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/go", cmd.name)
	assert.Equal(t, []string{"run", "./cmd/api-agent/cds-api-agent.go"}, cmd.args)
	assert.Equal(t, repoDir, cmd.dir)
}

func TestAgentCommandWithPortAppendsPortFlag(t *testing.T) {
	cmd := agentCommand{name: "go", args: []string{"run", "./cmd/api-agent/cds-api-agent.go"}}

	withPort := cmd.withPort("9091")

	assert.Equal(t, []string{"run", "./cmd/api-agent/cds-api-agent.go"}, cmd.args)
	assert.Equal(t, []string{"run", "./cmd/api-agent/cds-api-agent.go", "-port", "9091"}, withPort.args)
}

func TestIsAgentRunningRejectsTCPOnlyListener(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", configDir)
	cenv.SetConfigDirForClient()
	t.Cleanup(cenv.SetConfigDirForClient)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
	}()

	missingCertDir := filepath.Join(configDir, "missing-certs")
	address := listener.Addr().String()
	require.NoError(t, config.UpsertAgentForHostInConfig("127.0.0.1", config.NewAgent(
		config.WithTargetAddress(address),
		config.WithAgentTLS(config.NewTlssecret(
			config.WithCA(filepath.Join(missingCertDir, "ca.pem")),
			config.WithCert(filepath.Join(missingCertDir, "client.pem")),
			config.WithKey(filepath.Join(missingCertDir, "client-key.pem")),
		)),
	)))

	running, resolvedAddress, err := isAgentRunning("127.0.0.1")

	require.NoError(t, err)
	assert.False(t, running)
	assert.Equal(t, address, resolvedAddress)
}

func TestIsLocalHostRecognizesLocalTargets(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{name: "empty", host: "", want: true},
		{name: "localhost", host: "localhost", want: true},
		{name: "port only", host: ":8087", want: true},
		{name: "localhost port", host: "localhost:8087", want: true},
		{name: "localhost URL", host: "https://localhost:8087", want: true},
		{name: "remote", host: "remote.example.com:8087", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isLocalHost(tt.host))
		})
	}
}

func restoreAgentCommandTestHooks(t *testing.T) {
	t.Helper()

	originalLookPath := agentLookPath
	originalExecutable := agentExecutable
	originalWorkingDir := agentWorkingDir
	originalStat := agentStat
	t.Cleanup(func() {
		agentLookPath = originalLookPath
		agentExecutable = originalExecutable
		agentWorkingDir = originalWorkingDir
		agentStat = originalStat
	})
}
