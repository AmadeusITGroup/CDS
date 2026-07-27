package containerruntime

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// withStubs swaps the injected lookPath/runCommand/goos for the duration of a
// test and restores them afterwards.
func withStubs(t *testing.T, os string, lp func(string) (string, error), rc func(string, ...string) (string, error)) {
	t.Helper()
	origLookPath, origRun, origGoos := lookPath, runCommand, goos
	lookPath, runCommand, goos = lp, rc, os
	t.Cleanup(func() {
		lookPath, runCommand, goos = origLookPath, origRun, origGoos
	})
}

func TestDetect_RuntimeNotFound(t *testing.T) {
	withStubs(t, "darwin",
		func(string) (string, error) { return "", errors.New("not found") },
		func(string, ...string) (string, error) {
			t.Fatal("runCommand should not be called when the binary is missing")
			return "", nil
		},
	)

	_, err := Detect()
	assert.ErrorIs(t, err, ErrRuntimeNotFound)
}

func TestDetect_MachineNotRunning_Darwin(t *testing.T) {
	tests := []struct {
		name       string
		listOutput string
	}{
		{name: "empty output", listOutput: ""},
		{name: "empty array", listOutput: "[]"},
		{name: "machine stopped", listOutput: `[{"Name":"podman-machine-default","Running":false}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withStubs(t, "darwin",
				func(string) (string, error) { return "/usr/bin/podman", nil },
				func(_ string, args ...string) (string, error) {
					assert.Equal(t, []string{"machine", "list", "--format", "json"}, args)
					return tt.listOutput, nil
				},
			)

			_, err := Detect()
			assert.ErrorIs(t, err, ErrMachineNotRunning)
		})
	}
}

func TestDetect_MachineListFails_Darwin(t *testing.T) {
	withStubs(t, "darwin",
		func(string) (string, error) { return "/usr/bin/podman", nil },
		func(string, ...string) (string, error) { return "", errors.New("connection refused") },
	)

	_, err := Detect()
	assert.ErrorIs(t, err, ErrMachineNotRunning)
}

func TestDetect_RuntimeUnavailable_Darwin(t *testing.T) {
	withStubs(t, "darwin",
		func(string) (string, error) { return "/usr/bin/podman", nil },
		func(_ string, args ...string) (string, error) {
			if args[0] == "machine" {
				return `[{"Name":"podman-machine-default","Running":true}]`, nil
			}
			return "", errors.New("cannot connect to Podman socket")
		},
	)

	_, err := Detect()
	assert.ErrorIs(t, err, ErrRuntimeUnavailable)
}

func TestDetect_Healthy_Darwin(t *testing.T) {
	withStubs(t, "darwin",
		func(string) (string, error) { return "/usr/bin/podman", nil },
		func(_ string, args ...string) (string, error) {
			switch args[0] {
			case "machine":
				return `[{"Name":"podman-machine-default","Running":true}]`, nil
			case "info":
				return `{"version":{"Version":"5.1.1"}}`, nil
			default:
				t.Fatalf("unexpected podman subcommand: %v", args)
				return "", nil
			}
		},
	)

	info, err := Detect()
	assert.NoError(t, err)
	assert.Equal(t, EnginePodman, info.Engine)
	assert.Equal(t, "5.1.1", info.Version)
}

func TestDetect_Healthy_DarwinMultipleMachines(t *testing.T) {
	withStubs(t, "darwin",
		func(string) (string, error) { return "/usr/bin/podman", nil },
		func(_ string, args ...string) (string, error) {
			switch args[0] {
			case "machine":
				return `[{"Name":"stopped","Running":false},{"Name":"default","Running":true}]`, nil
			case "info":
				return `{"version":{"Version":"5.2.0"}}`, nil
			default:
				return "", nil
			}
		},
	)

	info, err := Detect()
	assert.NoError(t, err)
	assert.Equal(t, "5.2.0", info.Version)
}

// On Linux, Podman runs natively: there is no machine, so `podman machine list`
// must never be called and detection succeeds straight from `podman info`.
func TestDetect_Healthy_LinuxSkipsMachineCheck(t *testing.T) {
	withStubs(t, "linux",
		func(string) (string, error) { return "/usr/bin/podman", nil },
		func(_ string, args ...string) (string, error) {
			if args[0] == "machine" {
				t.Fatal("podman machine list must not be called on Linux")
			}
			return `{"version":{"Version":"4.9.0"}}`, nil
		},
	)

	info, err := Detect()
	assert.NoError(t, err)
	assert.Equal(t, "4.9.0", info.Version)
}

func TestDetect_RuntimeUnavailable_Linux(t *testing.T) {
	withStubs(t, "linux",
		func(string) (string, error) { return "/usr/bin/podman", nil },
		func(string, ...string) (string, error) { return "", errors.New("cannot connect to Podman socket") },
	)

	_, err := Detect()
	assert.ErrorIs(t, err, ErrRuntimeUnavailable)
}

// Windows, like macOS, runs Podman in a VM, so the machine check must apply: a
// stopped machine reports ErrMachineNotRunning, not ErrRuntimeUnavailable.
func TestDetect_MachineNotRunning_Windows(t *testing.T) {
	withStubs(t, "windows",
		func(string) (string, error) { return `C:\podman\podman.exe`, nil },
		func(_ string, args ...string) (string, error) {
			assert.Equal(t, "machine", args[0])
			return "[]", nil
		},
	)

	_, err := Detect()
	assert.ErrorIs(t, err, ErrMachineNotRunning)
}

func TestPodmanUsesMachine(t *testing.T) {
	assert.True(t, podmanUsesMachine("darwin"))
	assert.True(t, podmanUsesMachine("windows"))
	assert.False(t, podmanUsesMachine("linux"))
}
