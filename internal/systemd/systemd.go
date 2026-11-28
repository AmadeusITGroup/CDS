package systemd

import (
	"io"
	"net"
	"strconv"

	"github.com/amadeusitgroup/cds/internal/cerr"
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

// sysD manages the cds gRPC socket/service user units on a target host. It is
// one consumer of the generic unitManager: the cds.socket/cds.service pair is
// just the set of units it knows how to build and reconcile.
type sysD struct {
	h      hostOps
	port   int
	binary string
}

// mgr returns a unit manager bound to this target.
func (s *sysD) mgr() *unitManager {
	return newUnitManager(s.h)
}

// In checks if systemd is available on the target host.
func (s *sysD) In() bool {
	return s.mgr().available()
}

// IsServiceUp checks if the service is running.
func (s *sysD) IsServiceUp() bool {
	return s.mgr().isActive(serviceUnitName)
}

// StartService makes sure the service runs the desired units on the target
// host. The unit files are reconciled with the desired binary and port: when
// they differ from what is installed (or are missing), they are rewritten and
// the daemon reloaded. The units are then enabled and started. StartService
// reports whether the unit files changed, so the caller can decide to restart
// the service to pick up a new binary (see RestartService).
func (s *sysD) StartService() (bool, error) {
	m := s.mgr()

	unitDir, err := m.userUnitDir()
	if err != nil {
		return false, err
	}

	units, err := s.desiredUnits()
	if err != nil {
		return false, err
	}

	changed, err := m.reconcile(unitDir, units...)
	if err != nil {
		return false, err
	}

	// A daemon-reload before enabling is kept even when reconcile already
	// reloaded (nothing changed): historically StartService always reloaded
	// before enabling, and the no-op reload is cheap and harmless.
	if err := m.daemonReload(); err != nil {
		return false, err
	}
	if err := m.enableNow(socketUnitName, serviceUnitName); err != nil {
		return false, err
	}
	return changed, nil
}

// desiredUnits builds the cds socket and service units for the configured
// port and binary, applying the defaults used when they are unset.
func (s *sysD) desiredUnits() ([]Unit, error) {
	port := s.port
	if port == 0 {
		port = 8087
	}
	built, err := s.buildUnits(port)
	if err != nil {
		return nil, err
	}
	// Order matters: the socket is reconciled (and later enabled) before the
	// service it backs.
	return []Unit{
		{Name: socketUnitName, Content: built[socketUnitName]},
		{Name: serviceUnitName, Content: built[serviceUnitName]},
	}, nil
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

// RestartService restarts the service, e.g. to run an updated binary. The
// socket is kept so that no connection is refused meanwhile.
func (s *sysD) RestartService() error {
	return s.mgr().restart(serviceUnitName)
}

// StopService stops the systemd service on the target host.
func (s *sysD) StopService() error {
	return s.mgr().stop(socketUnitName, serviceUnitName)
}

// Listeners returns the list of systemd activation listeners.
func Listeners() ([]net.Listener, error) {
	return activation.Listeners()
}
