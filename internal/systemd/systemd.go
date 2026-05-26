package systemd

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/unit"
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
	FQDN() string
	Defined() bool
	Build() error
}

// In checks if systemd is available on the specified hostname.
func (s *sysD) In() bool {
	if s.h.FQDN() != cg.KLocalhost {
		return false
	}
	cmd := exec.Command("systemctl", "--user", "--no-pager")
	return cmd.Run() == nil
}

// IsServiceUp checks if the service is running on the specified host.
func (s *sysD) IsServiceUp() bool {
	if s.h.FQDN() != cg.KLocalhost {
		return false
	}
	out, err := shexec.ExecuteCmd(shexec.Execcmd{
		Name: "systemctl",
		Args: []string{"--user", "is-active", "cds.service"},
	}, cg.EmptyStr)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "active"
}

// StartService starts the service on the specified host.
func (s *sysD) StartService() error {
	if !s.isUnitReady() {
		if err := s.createUnit(); err != nil {
			return err
		}
	}
	return s.startUnit()
}

// isUnitReady checks if the systemd unit is ready on the specified hostname.
func (s *sysD) isUnitReady() bool {
	if s.h.FQDN() == cg.KLocalhost {
		socketPath := filepath.Join(os.Getenv("HOME"), ".config/systemd/user/cds.socket")
		servicePath := filepath.Join(os.Getenv("HOME"), ".config/systemd/user/cds.service")
		if _, err := os.Stat(socketPath); os.IsNotExist(err) {
			return false
		}
		if _, err := os.Stat(servicePath); os.IsNotExist(err) {
			return false
		}
		socketEnabled := shexec.Execcmd{
			Name: "systemctl",
			Args: []string{"--user", "is-enabled", "cds.socket"},
		}
		serviceEnabled := shexec.Execcmd{
			Name: "systemctl",
			Args: []string{"--user", "is-enabled", "cds.service"},
		}
		socketOut, err := shexec.ExecuteCmd(socketEnabled, cg.EmptyStr)
		if err != nil {
			clog.Error("failed to check socket unit state", err)
			return false
		}
		serviceOut, err := shexec.ExecuteCmd(serviceEnabled, cg.EmptyStr)
		if err != nil {
			clog.Error("failed to check service unit state", err)
			return false
		}
		return strings.Contains(string(socketOut), "enabled") &&
			strings.Contains(string(serviceOut), "enabled")
	}
	if !s.h.Defined() {
		if err := s.h.Build(); err != nil {
			clog.Error("failed to configure host", err)
		}
		return false
	}
	return false
}

// createUnit creates the systemd unit files on the specified hostname.
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
		{Section: "Unit", Name: "PartOf", Value: "cds.service"},
		{Section: "Socket", Name: "ListenStream", Value: strconv.Itoa(port)},
		{Section: "Socket", Name: "Accept", Value: "No"},
		{Section: "Socket", Name: "FileDescriptorName", Value: "cds"},
		{Section: "Install", Name: "WantedBy", Value: "sockets.target"},
	}
	serviceUnitOptions := []*unit.UnitOption{
		{Section: "Unit", Name: "Description", Value: "cds gRPC Service (User)"},
		{Section: "Unit", Name: "After", Value: "network.target"},
		{Section: "Unit", Name: "Requires", Value: "cds.socket"},
		{Section: "Service", Name: "Type", Value: "simple"},
		{Section: "Service", Name: "ExecStart", Value: binary},
		{Section: "Install", Name: "WantedBy", Value: "default.target"},
	}

	var socketUnitBytes, serviceUnitBytes []byte
	var errByte error

	socketUnitBytes, errByte = io.ReadAll(unit.Serialize(socketUnitOptions))
	if errByte != nil {
		return nil
	}
	unitsBytes["cds.socket"] = socketUnitBytes

	serviceUnitBytes, errByte = io.ReadAll(unit.Serialize(serviceUnitOptions))
	if errByte != nil {
		return nil
	}
	unitsBytes["cds.service"] = serviceUnitBytes

	return unitsBytes
}

// createUnitFileOnTarget creates a unit file on the target host with the specified file name and data.
func (s *sysD) createUnitFileOnTarget(fileName string, data []byte) error {
	if s.h.FQDN() == cg.KLocalhost {
		userUnitDir := filepath.Join(os.Getenv("HOME"), ".config/systemd/user")
		if err := os.MkdirAll(userUnitDir, fs.FileMode(0755)); err != nil {
			return cerr.AppendError("failed to create systemd user unit dir", err)
		}
		return cos.WriteFile(filepath.Join(userUnitDir, fileName), data, fs.FileMode(0644))
	}

	workDir, errTmpDir := os.MkdirTemp(cg.EmptyStr, "cds")
	if errTmpDir != nil {
		return cerr.AppendError("Failed to create temp dir", errTmpDir)
	}
	defer func() {
		_ = os.RemoveAll(workDir)
	}()

	clog.Debug(fmt.Sprintf("Working directory: %s", workDir))

	if err := cos.WriteFile(filepath.Join(workDir, fileName), data, fs.FileMode(0600)); err != nil {
		return err
	}

	if err := s.h.Build(); err != nil {
		return err
	}
	return nil
}

// startUnit enables and starts the systemd units on the specified hostname.
func (s *sysD) startUnit() error {
	if _, err := shexec.ExecuteCmd(shexec.Execcmd{
		Name: "systemctl",
		Args: []string{"--user", "daemon-reload"},
	}, cg.EmptyStr); err != nil {
		return cerr.AppendError("failed to reload systemd daemon", err)
	}
	if _, err := shexec.ExecuteCmd(shexec.Execcmd{
		Name: "systemctl",
		Args: []string{"--user", "enable", "cds.socket", "cds.service"},
	}, cg.EmptyStr); err != nil {
		return cerr.AppendError("failed to enable systemd units", err)
	}
	if _, err := shexec.ExecuteCmd(shexec.Execcmd{
		Name: "systemctl",
		Args: []string{"--user", "start", "cds.socket", "cds.service"},
	}, cg.EmptyStr); err != nil {
		return cerr.AppendError("failed to start systemd units", err)
	}
	return nil
}

// StopService stops the systemd service on the specified hostname.
func (s *sysD) StopService() error {
	if !s.In() {
		return fmt.Errorf("systemd not available on %s", s.h.FQDN())
	}
	if _, err := shexec.ExecuteCmd(shexec.Execcmd{
		Name: "systemctl",
		Args: []string{"--user", "stop", "cds.socket", "cds.service"},
	}, cg.EmptyStr); err != nil {
		return cerr.AppendError("failed to stop systemd units", err)
	}
	return nil
}

// Listeners returns the list of systemd activation listeners.
func Listeners() ([]net.Listener, error) {
	return activation.Listeners()
}
