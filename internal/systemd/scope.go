package systemd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	cg "github.com/amadeusitgroup/cds/internal/global"
)

// TransientScope wraps command so it runs in its own transient systemd user
// scope (a sibling of cds.service), detaching any long-lived container helpers
// it spawns (conmon, rootlessport) from the agent's service cgroup. Without
// this, stopping or restarting cds.service under the default
// KillMode=control-group kills those helpers (or waits 90 s on them, then
// SIGKILLs them). props are extra scope properties, e.g. "Delegate=yes".
//
// It returns the command unchanged (changed=false) when the agent is not under
// systemd or systemd-run is unavailable.
func TransientScope(command string, props ...string) (string, bool) {
	return NamedTransientScope(cg.EmptyStr, command, props...)
}

// NamedTransientScope is TransientScope with a fixed scope unit name (e.g.
// "cds-kind-create.scope"; empty lets systemd pick one), so the caller can stop
// the whole scope, every process in it, rather than only the command
// (CDS-076).
func NamedTransientScope(unit, command string, props ...string) (string, bool) {
	if !runningUnderSystemd() || !systemdRunAvailable() {
		return command, false
	}
	var p strings.Builder
	if unit != cg.EmptyStr {
		p.WriteString(" --unit=" + unit)
	}
	for _, prop := range props {
		p.WriteString(" -p " + prop)
	}
	// --scope runs the command in a transient scope unit; --collect reclaims it
	// once it exits. The scope is a sibling of cds.service, so a restart of the
	// service no longer tears down the container helpers placed in this scope.
	return fmt.Sprintf("systemd-run --user --scope --collect --quiet%s -- bash -c %s", p.String(), cg.ShellSingleQuote(command)), true
}

// runningUnderSystemd reports whether this process runs inside a systemd unit.
// systemd exports INVOCATION_ID to the processes of the units it starts.
func runningUnderSystemd() bool {
	return os.Getenv("INVOCATION_ID") != cg.EmptyStr
}

func systemdRunAvailable() bool {
	_, err := exec.LookPath("systemd-run")
	return err == nil
}
