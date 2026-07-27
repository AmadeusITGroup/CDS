package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

var (
	processName   = defaultProcessName
	signalProcess = defaultSignalProcess
)

// StopAgent stops the local agent process registered for hostname. Remote
// agents are not stopped by local unregister today.
func StopAgent(hostname string) error {
	if hostname != cg.KLocalhost {
		return nil
	}
	if runtime.GOOS == "windows" {
		return nil
	}

	ownership := db.GetHostAgentOwnership(hostname)
	if !isProcessManagedAgent(ownership) {
		return nil
	}
	return StopManagedAgent(ownership)
}

// StopManagedAgent stops an agent process when the ownership record proves CDS
// started and owns that process. Non-process-managed ownership records are left
// untouched.
func StopManagedAgent(ownership bo.AgentOwnership) error {
	if !isProcessManagedAgent(ownership) {
		return nil
	}
	name, err := processName(ownership.PID)
	if err != nil {
		clog.Debug(fmt.Sprintf("Local agent process %d is no longer running", ownership.PID))
		return nil
	}
	if !matchesOwnedAgentProcess(name, ownership) {
		clog.Debug(fmt.Sprintf("Skipping process %d because it does not match stored CDS agent ownership", ownership.PID))
		return nil
	}
	if err := signalProcess(ownership.PID); err != nil {
		return cerr.AppendErrorFmt("failed to stop local agent process %d", err, ownership.PID)
	}
	clog.Debug(fmt.Sprintf("Stopped local agent process %d", ownership.PID))
	return nil
}

func defaultProcessName(pid int) (string, error) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return cg.EmptyStr, err
	}
	return strings.TrimSpace(string(out)), nil
}

func isProcessManagedAgent(ownership bo.AgentOwnership) bool {
	return ownership.Manager == "process" && ownership.PID > 0 && ownership.Binary != cg.EmptyStr
}

func matchesOwnedAgentProcess(name string, ownership bo.AgentOwnership) bool {
	base := filepath.Base(strings.TrimSpace(name))
	return base == filepath.Base(strings.TrimSpace(ownership.Binary))
}

func defaultSignalProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}
