package systemd

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
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
	"gopkg.in/ini.v1"
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

type sysD struct {
	h hostOps
}
type hostOps interface {
	FQDN() string
	Defined() bool
	Build() error
}

// In checks if systemd is used on the specified hostname.
func (s *sysD) In() bool {
	// Infering that systemd is used based on distros (rhel and fedora)

	if s.h.FQDN() != cg.KLocalhost {
		// TODO: add remote command to check if systemd is used
		return false
	}
	cfg, err := ini.Load("/etc/os-release")
	if err != nil {
		return false
	}
	distro := strings.ToLower(cfg.Section("").Key("ID").String())
	return strings.Compare(distro, "rhel") == 0 || strings.Compare(distro, "fedora") == 0
}

// IsServiceUp checks if the service is running on the specified host.
func (s *sysD) IsServiceUp() bool {
	return false
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
		socketPath := filepath.Join(os.Getenv("HOME"), ".config/systemd/user/mysrv.socket")
		servicePath := filepath.Join(os.Getenv("HOME"), ".config/systemd/user/mysrv.service")
		if _, err := os.Stat(socketPath); os.IsNotExist(err) {
			return false
		}

		if _, err := os.Stat(servicePath); os.IsNotExist(err) {
			return false
		}
		socketEnabled := shexec.Execcmd{Name: "sh", Args: []string{"-c", "systemctl", "--user", "is-enabled", "mysrv.socket"}}
		serviceEnabled := shexec.Execcmd{Name: "sh", Args: []string{"-c", "systemctl", "--user", "is-enabled", "mysrv.service"}}

		socketOut, err := shexec.ExecuteCmd(socketEnabled, cg.EmptyStr)
		if err != nil {
			clog.Error("failed to start socket unit", err)
			return false
		}
		serviceOut, err := shexec.ExecuteCmd(serviceEnabled, cg.EmptyStr)
		if err != nil {
			clog.Error("failed to start service unit", err)
			return false
		}

		return strings.Contains(string(socketOut), "enabled") && strings.Contains(string(serviceOut), "enabled")
	}
	// TODO remote use case
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
	// get free port on hostname
	var port int // TODO

	// create socket unit
	unitsBytes := s.buildUnits(port)
	for unitFileName, unitByte := range unitsBytes {
		if err := s.createUnitFileOnTarget(unitFileName, unitByte); err != nil {
			return cerr.AppendErrorFmt("faile to build unit file %s", err, unitFileName)
		}
	}
	return nil
}

// buildUnits builds the systemd unit files for the given port.
func (s *sysD) buildUnits(port int) map[string][]byte {

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
		{Section: "Unit", Name: "Requires", Value: "myserver.socket"},
		{Section: "Service", Name: "Type", Value: "simple"},
		{Section: "Service", Name: "ExecStart", Value: "%h/bin/cds-agent"}, // TODO fix!
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

	// TODO scp file to destination
	if err := s.h.Build(); err != nil {
		return err
	}
	return nil
}

// startUnit starts the systemd unit on the specified hostname.
func (s *sysD) startUnit() error {
	return nil
}

// Listeners returns the list of systemd activation listeners.
func Listeners() ([]net.Listener, error) {
	return activation.Listeners()
}
