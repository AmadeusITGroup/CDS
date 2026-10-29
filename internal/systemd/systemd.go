package systemd

import (
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/unit"
)

const (
	socketUnitName  = "cds.socket"
	serviceUnitName = "cds.service"
)

func New(ops ...func(*sysD)) *sysD {
	sysd := &sysD{}
	for _, op := range ops {
		op(sysd)
	}
	return sysd
}

func WithTarget(h hostOps) func(*sysD) {
	return func(sd *sysD) {
		sd.h = h
	}
}

func WithServicePort(port int) func(*sysD) {
	return func(sd *sysD) {
		sd.port = port
	}
}

func WithServiceBinary(binary string) func(*sysD) {
	return func(sd *sysD) {
		sd.binary = binary
	}
}

type sysD struct {
	h      hostOps
	port   int
	binary string
}

// hostOps is the target host, prepared by the caller (e.g. SSH access configured).
type hostOps interface {
	Execute(name string, args ...string) (string, error)
	Copy(localPath, remotePath string) error
}

// userUnitDir returns the systemd user unit directory of the target host.
// It is resolved on the target itself since its home directory may differ
// from the local one. Targets are Linux hosts, hence POSIX path joining.
func (s *sysD) userUnitDir() (string, error) {
	out, err := s.h.Execute("systemd-path", "user-configuration")
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("failed to resolve systemd user configuration directory", err)
	}
	configDir := strings.TrimSpace(out)
	if configDir == cg.EmptyStr {
		return cg.EmptyStr, cerr.NewError("systemd-path returned an empty user configuration directory")
	}
	return path.Join(configDir, "systemd", "user"), nil
}

// In checks if systemd is available on the target host.
func (s *sysD) In() bool {
	_, err := s.h.Execute("systemctl", "--user", "--no-pager")
	return err == nil
}

// IsServiceUp checks if the service is running.
func (s *sysD) IsServiceUp() bool {
	out, err := s.h.Execute("systemctl", "--user", "is-active", serviceUnitName)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "active"
}

// StartService makes sure the service runs the desired units on the target
// host. The unit files are reconciled with the desired binary and port: when
// they differ from what is installed (or are missing), they are rewritten and
// the daemon reloaded. The units are then enabled and started. StartService
// reports whether the unit files changed, so the caller can decide to restart
// the service to pick up a new binary (see RestartService).
func (s *sysD) StartService() (bool, error) {
	changed, err := s.reconcileUnits()
	if err != nil {
		return false, err
	}
	if err := s.startUnit(); err != nil {
		return false, err
	}
	return changed, nil
}

// reconcileUnits writes the desired unit files when they are missing or differ
// from what is installed on the target, reloading the systemd daemon when it
// changes anything. It returns whether any unit file was written.
func (s *sysD) reconcileUnits() (bool, error) {
	port := s.port
	if port == 0 {
		port = 8087
	}
	desired, err := s.buildUnits(port)
	if err != nil {
		return false, err
	}

	unitDir, err := s.userUnitDir()
	if err != nil {
		return false, err
	}
	if _, err := s.h.Execute("mkdir", "-p", unitDir); err != nil {
		return false, cerr.AppendErrorFmt("failed to create unit directory %s", err, unitDir)
	}

	changed := false
	for _, unitName := range []string{socketUnitName, serviceUnitName} {
		if s.unitUpToDate(unitDir, unitName, desired[unitName]) {
			continue
		}
		if err := s.createUnitFileOnTarget(unitDir, unitName, desired[unitName]); err != nil {
			return false, cerr.AppendErrorFmt("failed to install unit file %s", err, unitName)
		}
		changed = true
	}

	if changed {
		if _, err := s.h.Execute("systemctl", "--user", "daemon-reload"); err != nil {
			return false, cerr.AppendError("failed to reload systemd daemon", err)
		}
	}
	return changed, nil
}

// unitUpToDate reports whether the unit file installed on the target has
// exactly the desired content, so that it does not need to be rewritten.
func (s *sysD) unitUpToDate(unitDir, unitName string, desired []byte) bool {
	out, err := s.h.Execute("cat", path.Join(unitDir, unitName))
	if err != nil {
		return false
	}
	return out == string(desired)
}

// buildUnits builds the systemd unit files for the given port.
func (s *sysD) buildUnits(port int) (map[string][]byte, error) {
	binary := s.binary
	if binary == "" {
		binary = "cds-api-agent"
	}

	socketUnitOptions := []*unit.UnitOption{
		{Section: "Unit", Name: "Description", Value: "cds gRPC Socket (User)"},
		{Section: "Unit", Name: "PartOf", Value: serviceUnitName},
		{Section: "Socket", Name: "ListenStream", Value: strconv.Itoa(port)},
		{Section: "Socket", Name: "Accept", Value: "No"},
		{Section: "Socket", Name: "FileDescriptorName", Value: "cds"},
		{Section: "Install", Name: "WantedBy", Value: "sockets.target"},
	}
	serviceUnitOptions := []*unit.UnitOption{
		{Section: "Unit", Name: "Description", Value: "cds gRPC Service (User)"},
		{Section: "Unit", Name: "After", Value: "network.target"},
		{Section: "Unit", Name: "Requires", Value: socketUnitName},
		{Section: "Service", Name: "Type", Value: "simple"},
		{Section: "Service", Name: "ExecStart", Value: binary},
		{Section: "Install", Name: "WantedBy", Value: "default.target"},
	}

	unitsBytes := make(map[string][]byte)
	for unitName, options := range map[string][]*unit.UnitOption{
		socketUnitName:  socketUnitOptions,
		serviceUnitName: serviceUnitOptions,
	} {
		unitBytes, err := io.ReadAll(unit.Serialize(options))
		if err != nil {
			return nil, cerr.AppendErrorFmt("failed to serialize unit %s", err, unitName)
		}
		unitsBytes[unitName] = unitBytes
	}
	return unitsBytes, nil
}

// createUnitFileOnTarget creates a unit file on the target host.
func (s *sysD) createUnitFileOnTarget(unitDir, fileName string, data []byte) error {
	tmpDir, err := os.MkdirTemp("", "cds-units")
	if err != nil {
		return cerr.AppendError("failed to create temp dir for unit files", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpFile := filepath.Join(tmpDir, fileName)
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return cerr.AppendError("failed to write temp unit file", err)
	}

	return s.h.Copy(tmpFile, unitDir)
}

// startUnit enables and starts the systemd units on the target host.
func (s *sysD) startUnit() error {
	if _, err := s.h.Execute("systemctl", "--user", "daemon-reload"); err != nil {
		return cerr.AppendError("failed to reload systemd daemon", err)
	}
	if _, err := s.h.Execute("systemctl", "--user", "enable", "--now", socketUnitName, serviceUnitName); err != nil {
		return cerr.AppendError("failed to enable systemd units", err)
	}
	return nil
}

// RestartService restarts the service, e.g. to run an updated binary. The
// socket is kept so that no connection is refused meanwhile.
func (s *sysD) RestartService() error {
	if _, err := s.h.Execute("systemctl", "--user", "restart", serviceUnitName); err != nil {
		return cerr.AppendError("failed to restart systemd service", err)
	}
	return nil
}

// StopService stops the systemd service on the target host.
func (s *sysD) StopService() error {
	if _, err := s.h.Execute("systemctl", "--user", "stop", socketUnitName, serviceUnitName); err != nil {
		return cerr.AppendError("failed to stop systemd units", err)
	}
	return nil
}

// Listeners returns the list of systemd activation listeners.
func Listeners() ([]net.Listener, error) {
	return activation.Listeners()
}
