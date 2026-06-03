package systemd

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
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

type hostOps interface {
	Defined() bool
	Build() error
	Execute(name string, args ...string) (string, error)
	Copy(localPath, remotePath string) error
}

func userUnitDir() string {
	return filepath.Join(os.Getenv("HOME"), ".config/systemd/user")
}

func unitPath(unitName string) string {
	return filepath.Join(userUnitDir(), unitName)
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

// StartService starts the service on the target host.
func (s *sysD) StartService() error {
	if !s.isUnitReady() {
		if err := s.createUnit(); err != nil {
			return err
		}
	}
	return s.startUnit()
}

// isUnitReady checks if the systemd unit files exist and are enabled.
func (s *sysD) isUnitReady() bool {
	if _, err := s.h.Execute("test", "-f", unitPath(socketUnitName)); err != nil {
		if !s.h.Defined() {
			if berr := s.h.Build(); berr != nil {
				clog.Error("failed to configure host", berr)
			}
		}
		return false
	}
	if _, err := s.h.Execute("test", "-f", unitPath(serviceUnitName)); err != nil {
		if !s.h.Defined() {
			if berr := s.h.Build(); berr != nil {
				clog.Error("failed to configure host", berr)
			}
		}
		return false
	}

	socketOut, err := s.h.Execute("systemctl", "--user", "is-enabled", socketUnitName)
	if err != nil {
		clog.Error("failed to check socket unit state", err)
		return false
	}
	serviceOut, err := s.h.Execute("systemctl", "--user", "is-enabled", serviceUnitName)
	if err != nil {
		clog.Error("failed to check service unit state", err)
		return false
	}
	return strings.Contains(socketOut, "enabled") &&
		strings.Contains(serviceOut, "enabled")
}

// createUnit creates the systemd unit files on the target host.
func (s *sysD) createUnit() error {
	port := s.port
	if port == 0 {
		port = 8087
	}

	unitsBytes := s.buildUnits(port)
	for unitFileName, unitByte := range unitsBytes {
		if err := s.createUnitFileOnTarget(unitFileName, unitByte); err != nil {
			return cerr.AppendErrorFmt("failed to build unit file %s", err, unitFileName)
		}
	}
	return nil
}

// buildUnits builds the systemd unit files for the given port.
func (s *sysD) buildUnits(port int) map[string][]byte {
	binary := s.binary
	if binary == "" {
		binary = "cdssrv"
	}

	unitsBytes := make(map[string][]byte)

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

	var errByte error
	socketUnitBytes, errByte := io.ReadAll(unit.Serialize(socketUnitOptions))
	if errByte != nil {
		return nil
	}
	unitsBytes[socketUnitName] = socketUnitBytes

	serviceUnitBytes, errByte := io.ReadAll(unit.Serialize(serviceUnitOptions))
	if errByte != nil {
		return nil
	}
	unitsBytes[serviceUnitName] = serviceUnitBytes

	return unitsBytes
}

// createUnitFileOnTarget creates a unit file on the target host.
func (s *sysD) createUnitFileOnTarget(fileName string, data []byte) error {
	tmpDir, err := os.MkdirTemp("", "cds-units")
	if err != nil {
		return cerr.AppendError("failed to create temp dir for unit files", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpFile := filepath.Join(tmpDir, fileName)
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return cerr.AppendError("failed to write temp unit file", err)
	}

	return s.h.Copy(tmpFile, userUnitDir())
}

// startUnit enables and starts the systemd units on the target host.
func (s *sysD) startUnit() error {
	if _, err := s.h.Execute("systemctl", "--user", "daemon-reload"); err != nil {
		return cerr.AppendError("failed to reload systemd daemon", err)
	}
	if _, err := s.h.Execute("systemctl", "--user", "enable", "now", socketUnitName, serviceUnitName); err != nil {
		return cerr.AppendError("failed to enable systemd units", err)
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
