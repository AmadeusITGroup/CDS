// Package containerruntime detects and validates the container runtime (Podman)
// that the agent relies on to manage DevContainers on a host.
//
// On macOS, Podman runs inside a virtual machine, so a usable runtime requires
// more than the binary being on PATH: the Podman machine must also be running
// and answering. Detect performs those checks and reports an actionable error
// for each failure mode. On Linux, Podman runs natively and the machine check
// is skipped.
package containerruntime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	stdruntime "runtime"
	"strings"

	"github.com/amadeusitgroup/cds/internal/clog"
)

// EnginePodman is the only container engine supported for local hosts today.
const EnginePodman = "podman"

// Info describes the detected container runtime.
type Info struct {
	Engine  string // e.g. "podman"
	Version string // e.g. "5.1.1"
}

// Detection failures. Errors are wrapped with %w around these sentinels so
// callers can branch with errors.Is, while the wrapped message carries an
// actionable hint suitable for display to the developer.
var (
	// ErrRuntimeNotFound indicates the Podman binary is not installed / not on PATH.
	ErrRuntimeNotFound = errors.New("container runtime not found")
	// ErrMachineNotRunning indicates Podman is installed but its machine (VM) is not running (macOS).
	ErrMachineNotRunning = errors.New("container runtime machine is not running")
	// ErrRuntimeUnavailable indicates Podman is installed and reachable, but is not responding.
	ErrRuntimeUnavailable = errors.New("container runtime is not responding")
)

// Injection points so detection can be unit-tested without a real Podman and
// for either operating system.
var (
	lookPath   = exec.LookPath
	runCommand = defaultRunCommand
	goos       = stdruntime.GOOS
)

// defaultRunCommand runs a binary directly (no shell) and returns its stdout.
// stderr is folded into the returned error so callers get useful context.
func defaultRunCommand(name string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

type podmanMachine struct {
	Name    string `json:"Name"`
	Running bool   `json:"Running"`
}

type podmanInfo struct {
	Version struct {
		Version string `json:"Version"`
	} `json:"version"`
}

// Detect locates the container runtime and validates that it is usable.
//
// It returns ErrRuntimeNotFound, ErrMachineNotRunning, or ErrRuntimeUnavailable
// (wrapped with an actionable message) when the runtime cannot be used.
func Detect() (Info, error) {
	clog.Debug("Detecting container runtime (podman)")

	if _, err := lookPath(EnginePodman); err != nil {
		return Info{}, fmt.Errorf(
			"%w: install Podman (e.g. `brew install podman`) and run `podman machine init`",
			ErrRuntimeNotFound)
	}

	// On VM-backed platforms (macOS, and Windows via WSL2) Podman runs inside a
	// machine that must be started before it can serve requests. On Linux it runs
	// natively, so there is no machine to check.
	if podmanUsesMachine(goos) {
		running, err := podmanMachineRunning()
		if err != nil {
			return Info{}, err
		}
		if !running {
			return Info{}, fmt.Errorf(
				"%w: start it with `podman machine start` (initialise first with `podman machine init` if needed)",
				ErrMachineNotRunning)
		}
	}

	version, err := podmanVersion()
	if err != nil {
		return Info{}, fmt.Errorf(
			"%w: `podman info` failed (%v)", ErrRuntimeUnavailable, err)
	}

	info := Info{Engine: EnginePodman, Version: version}
	clog.Debug(fmt.Sprintf("Detected %s %s", info.Engine, info.Version))
	return info, nil
}

// podmanUsesMachine reports whether Podman runs inside a managed VM ("machine")
// on the given OS rather than natively. macOS (applehv) and Windows (WSL2) are
// VM-backed and require `podman machine start`; Linux runs natively.
//
// Windows is listed here for correctness, but Windows host registration is not
// yet wired up — see the local-host-registration design doc (out of scope).
func podmanUsesMachine(os string) bool {
	return os == "darwin" || os == "windows"
}

// podmanMachineRunning reports whether at least one Podman machine is running.
func podmanMachineRunning() (bool, error) {
	out, err := runCommand(EnginePodman, "machine", "list", "--format", "json")
	if err != nil {
		return false, fmt.Errorf("%w: `podman machine list` failed (%v)", ErrMachineNotRunning, err)
	}

	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return false, nil
	}

	var machines []podmanMachine
	if err := json.Unmarshal([]byte(out), &machines); err != nil {
		return false, fmt.Errorf("%w: failed to parse `podman machine list` output (%v)", ErrMachineNotRunning, err)
	}

	for _, m := range machines {
		if m.Running {
			return true, nil
		}
	}
	return false, nil
}

// podmanVersion returns the Podman version reported by `podman info`.
func podmanVersion() (string, error) {
	out, err := runCommand(EnginePodman, "info", "--format", "json")
	if err != nil {
		return "", err
	}

	var info podmanInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return "", fmt.Errorf("failed to parse `podman info` output: %w", err)
	}
	return info.Version.Version, nil
}
