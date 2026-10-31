package systemd

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// hostOps is the target host, prepared by the caller (e.g. SSH access
// configured). Targets are Linux hosts, hence POSIX path joining throughout
// this package.
type hostOps interface {
	Execute(name string, args ...string) (string, error)
	Copy(localPath, remotePath string) error
}

// Unit is a single systemd user unit to reconcile on the target: its file name
// (e.g. "cds.service", "cds-sec-registry.container") and the desired file
// content. A unit manager reconciles the installed file with Content and, for
// enabling, uses Name.
type Unit struct {
	Name    string
	Content []byte
}

// unitManager reconciles and controls systemd user units on a target host by
// content. It is the generic primitive the agent reuses for the cds socket/
// service, the secure registry Quadlet, the kind supervision unit and the
// devcontainer Quadlets: each is just a set of Units driven through the same
// reconcile / daemon-reload / enable / start / stop / remove / is-active steps.
//
// It is intentionally OS-neutral at the call site (the client may run on macOS
// while bootstrapping a remote Linux host): all systemd interaction goes
// through the target's hostOps, and `systemctl --user` on the target decides
// availability.
type unitManager struct {
	h hostOps
}

func newUnitManager(h hostOps) *unitManager {
	return &unitManager{h: h}
}

// available reports whether a systemd user session is reachable on the target.
func (m *unitManager) available() bool {
	_, err := m.h.Execute("systemctl", "--user", "--no-pager")
	return err == nil
}

// isActive reports whether the given unit is active on the target.
func (m *unitManager) isActive(unitName string) bool {
	out, err := m.h.Execute("systemctl", "--user", "is-active", unitName)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "active"
}

// userUnitDir returns the systemd user unit directory of the target host. It is
// resolved on the target itself since its home directory may differ from the
// local one.
func (m *unitManager) userUnitDir() (string, error) {
	out, err := m.h.Execute("systemd-path", "user-configuration")
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("failed to resolve systemd user configuration directory", err)
	}
	configDir := strings.TrimSpace(out)
	if configDir == cg.EmptyStr {
		return cg.EmptyStr, cerr.NewError("systemd-path returned an empty user configuration directory")
	}
	return path.Join(configDir, "systemd", "user"), nil
}

// reconcile writes the desired unit files into unitDir when they are missing or
// differ from what is installed on the target, reloading the systemd daemon
// once when it changed anything. It returns whether any unit file was written.
//
// unitDir is passed in (rather than resolved here) so callers that install
// several unit families into different directories — e.g. Quadlet units under
// ~/.config/containers/systemd — can reuse the same reconciliation.
func (m *unitManager) reconcile(unitDir string, units ...Unit) (bool, error) {
	if _, err := m.h.Execute("mkdir", "-p", unitDir); err != nil {
		return false, cerr.AppendErrorFmt("failed to create unit directory %s", err, unitDir)
	}

	changed := false
	for _, u := range units {
		if m.unitUpToDate(unitDir, u.Name, u.Content) {
			continue
		}
		if err := m.writeUnitFile(unitDir, u.Name, u.Content); err != nil {
			return false, cerr.AppendErrorFmt("failed to install unit file %s", err, u.Name)
		}
		changed = true
	}

	if changed {
		if err := m.daemonReload(); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// unitUpToDate reports whether the unit file installed on the target has
// exactly the desired content, so that it does not need to be rewritten.
func (m *unitManager) unitUpToDate(unitDir, unitName string, desired []byte) bool {
	out, err := m.h.Execute("cat", path.Join(unitDir, unitName))
	if err != nil {
		return false
	}
	return out == string(desired)
}

// writeUnitFile writes a unit file on the target host via a local temp file
// copied to unitDir.
func (m *unitManager) writeUnitFile(unitDir, fileName string, data []byte) error {
	tmpDir, err := os.MkdirTemp("", "cds-units")
	if err != nil {
		return cerr.AppendError("failed to create temp dir for unit files", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpFile := filepath.Join(tmpDir, fileName)
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return cerr.AppendError("failed to write temp unit file", err)
	}
	return m.h.Copy(tmpFile, unitDir)
}

// daemonReload reloads the systemd user daemon on the target.
func (m *unitManager) daemonReload() error {
	if _, err := m.h.Execute("systemctl", "--user", "daemon-reload"); err != nil {
		return cerr.AppendError("failed to reload systemd daemon", err)
	}
	return nil
}

// enableNow enables and starts the given units on the target.
func (m *unitManager) enableNow(unitNames ...string) error {
	args := append([]string{"--user", "enable", "--now"}, unitNames...)
	if _, err := m.h.Execute("systemctl", args...); err != nil {
		return cerr.AppendError("failed to enable systemd units", err)
	}
	return nil
}

// start starts the given units on the target (without enabling them at boot).
func (m *unitManager) start(unitNames ...string) error {
	args := append([]string{"--user", "start"}, unitNames...)
	if _, err := m.h.Execute("systemctl", args...); err != nil {
		return cerr.AppendError("failed to start systemd units", err)
	}
	return nil
}

// restart restarts the given units on the target.
func (m *unitManager) restart(unitNames ...string) error {
	args := append([]string{"--user", "restart"}, unitNames...)
	if _, err := m.h.Execute("systemctl", args...); err != nil {
		return cerr.AppendError("failed to restart systemd units", err)
	}
	return nil
}

// stop stops the given units on the target.
func (m *unitManager) stop(unitNames ...string) error {
	args := append([]string{"--user", "stop"}, unitNames...)
	if _, err := m.h.Execute("systemctl", args...); err != nil {
		return cerr.AppendError("failed to stop systemd units", err)
	}
	return nil
}

// remove disables the given units and deletes their files from unitDir,
// reloading the daemon afterwards. Disable failures are tolerated (a unit that
// was never enabled, or is already gone, must not fail teardown); file removal
// and the reload are the operations that must succeed.
func (m *unitManager) remove(unitDir string, unitNames ...string) error {
	disableArgs := append([]string{"--user", "disable", "--now"}, unitNames...)
	if _, err := m.h.Execute("systemctl", disableArgs...); err != nil {
		// best effort: the unit may not have been enabled
		_ = err
	}
	for _, name := range unitNames {
		if _, err := m.h.Execute("rm", "-f", path.Join(unitDir, name)); err != nil {
			return cerr.AppendErrorFmt("failed to remove unit file %s", err, name)
		}
	}
	return m.daemonReload()
}
