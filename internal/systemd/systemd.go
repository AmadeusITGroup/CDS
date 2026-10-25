package systemd

import (
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
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
	unitDir, err := s.userUnitDir()
	if err != nil {
		clog.Error("failed to check unit files", err)
		return false
	}

	for _, unitName := range []string{socketUnitName, serviceUnitName} {
		if _, err := s.h.Execute("test", "-f", path.Join(unitDir, unitName)); err != nil {
			return false
		}
	}

	for _, unitName := range []string{socketUnitName, serviceUnitName} {
		// is-enabled exits with a non-zero code when the unit is not enabled
		out, err := s.h.Execute("systemctl", "--user", "is-enabled", unitName)
		if err != nil {
			clog.Debug(fmt.Sprintf("%s unit is not enabled: %v", unitName, err))
			return false
		}
		if strings.TrimSpace(out) != "enabled" {
			return false
		}
	}
	return true
}

// createUnit creates the systemd unit files on the target host.
func (s *sysD) createUnit() error {
	port := s.port
	if port == 0 {
		port = 8087
	}

	unitsBytes, err := s.buildUnits(port)
	if err != nil {
		return err
	}

	unitDir, err := s.userUnitDir()
	if err != nil {
		return err
	}
	if _, err := s.h.Execute("mkdir", "-p", unitDir); err != nil {
		return cerr.AppendErrorFmt("failed to create unit directory %s", err, unitDir)
	}

	for unitFileName, unitByte := range unitsBytes {
		if err := s.createUnitFileOnTarget(unitDir, unitFileName, unitByte); err != nil {
			return cerr.AppendErrorFmt("failed to build unit file %s", err, unitFileName)
		}
	}
	return nil
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
