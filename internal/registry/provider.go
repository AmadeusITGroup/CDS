package registry

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
	"github.com/amadeusitgroup/cds/internal/systemd"
)

const (
	// quadletUnitFile is the Quadlet .container unit file name. podman-generator
	// turns it into cds-sec-registry.service.
	quadletUnitFile = Name + ".container"
	// quadletService is the systemd service podman generates from the Quadlet
	// unit.
	quadletService = Name + ".service"
	// materialDirName is the agent config subdir holding registry material.
	materialDirName = "registry"
)

// podmanProvider runs the secure registry as a Quadlet container on the agent
// host.
type podmanProvider struct {
	engineName string
	port       int
	// runner executes a local shell command; injected for tests.
	runner func(command string) (string, error)
	// units manages the Quadlet unit; injected for tests.
	units *systemd.UnitManager
	// materialRoot is the registry material directory; injected for tests.
	materialRoot string
	// quadletDir is where the Quadlet unit file is installed; injected for tests.
	quadletDir string
	// certsDBase is the podman certs.d base dir; injected for tests.
	certsDBase string
}

var _ Provider = (*podmanProvider)(nil)

// NewProvider returns the secure registry provider for the agent host.
func NewProvider() *podmanProvider {
	return &podmanProvider{
		engineName:   cg.EmptyStr,
		port:         DefaultPort,
		runner:       runLocal,
		units:        systemd.NewUnitManager(systemd.LocalHost()),
		materialRoot: cenv.AgentConfigDir(materialDirName),
		quadletDir:   filepath.Join(cenv.GetUserHomeDir(), ".config", "containers", "systemd"),
		certsDBase:   filepath.Join(cenv.GetUserHomeDir(), ".config", "containers", "certs.d"),
	}
}

func runLocal(command string) (string, error) {
	return shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{&shexec.DefaultShEvent{ExeCmd: command}})
}

// Build generates the registry material and reconciles the Quadlet unit, then
// starts the registry and installs host trust. The registry is always secured
// (auth+TLS).
func (p *podmanProvider) Build(report ProgressReporter) error {
	if err := reportf(report, "material", "Generating registry material"); err != nil {
		return err
	}
	m, materialChanged, err := ensureMaterial(p.materialRoot)
	if err != nil {
		return err
	}

	if err := reportf(report, "network", "Ensuring kind network for registry"); err != nil {
		return err
	}
	if err := engine.EnsureKindNetwork(p.engineName); err != nil {
		return cerr.AppendError("failed to ensure kind network for registry", err)
	}

	if err := reportf(report, "unit", "Installing registry Quadlet unit"); err != nil {
		return err
	}
	if err := os.MkdirAll(p.quadletDir, 0755); err != nil {
		return cerr.AppendErrorFmt("failed to create Quadlet dir %s", err, p.quadletDir)
	}
	content := p.quadletUnit(m)
	changed, err := p.units.Reconcile(p.quadletDir, systemd.Unit{Name: quadletUnitFile, Content: content})
	if err != nil {
		return cerr.AppendError("failed to install registry Quadlet unit", err)
	}

	if err := reportf(report, "registry", fmt.Sprintf("Starting registry %s", Name)); err != nil {
		return err
	}
	// Quadlet-generated services are transient from systemd's view; a daemon
	// reload makes the generated unit visible, then start it.
	// A changed unit (e.g. the new NetworkAlias after the registry rename) or
	// changed material (a reissued certificate) is restarted, since a
	// plain start keeps the old container serving the old files.
	start := p.units.Start
	if changed || materialChanged {
		start = p.units.Restart
	}
	if err := start(quadletService); err != nil {
		return cerr.AppendError("failed to start registry", err)
	}

	// the host port is picked by podman at each start, so trust follows the start
	if err := reportf(report, "trust", "Installing registry host trust"); err != nil {
		return err
	}
	if err := p.installHostTrust(m); err != nil {
		return err
	}
	return reportf(report, "registry", fmt.Sprintf("Registry %s started", Name))
}

// quadletUnit builds the Quadlet .container unit. Data, certs and auth are
// mounted as volumes so state survives container recreation (Quadlet recreates
// the container on each start).
func (p *podmanProvider) quadletUnit(m material) []byte {
	// Quadlet .container is an INI-like file read by podman's systemd generator.
	// The host port is bound to loopback only: kind nodes and
	// devcontainers reach the registry on the kind network through its alias, so
	// nothing needs it on the host's external interfaces. podman picks a free
	// host port, so each user's registry on a shared host gets its own (CDS-075).
	return []byte(fmt.Sprintf(`[Unit]
Description=CDS secure registry (User)

[Container]
ContainerName=%s
Image=%s
PublishPort=127.0.0.1::%d
Network=%s
NetworkAlias=%s
Volume=%s:/certs:Z
Volume=%s:/auth:Z
Volume=%s:/var/lib/registry:Z
Environment=REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt
Environment=REGISTRY_HTTP_TLS_KEY=/certs/domain.key
Environment=REGISTRY_AUTH=htpasswd
Environment=REGISTRY_AUTH_HTPASSWD_REALM=cds
Environment=REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd

[Service]
# A requested stop kills the container, so the main process exits 2 (or 137/143).
# Treat those as success so a clean 'registry stop' leaves the unit inactive, not failed.
# shortcut: a real crash with exit 2 is also hidden; Restart= is not set, so upgrade if auto-restart is added.
SuccessExitStatus=2 137 143

[Install]
WantedBy=default.target
`,
		Name,
		Image,
		p.port,
		engine.KindNetworkName,
		Hostname,
		m.certsDir(),
		m.authDir(),
		m.dataDir(),
	))
}

// installHostTrust copies the registry certificate into the podman per-registry
// trust directories for the published loopback port, so host podman/skopeo
// trust the self-signed certificate: ~/.config/containers/certs.d/<addr>:<port>/ca.crt
// for 127.0.0.1 and localhost. Kind nodes get it through TrustRegistryOnNodes.
// The port changes at each start, so trust left for an earlier port is removed.
func (p *podmanProvider) installHostTrust(m material) error {
	port, err := p.hostPort()
	if err != nil {
		return err
	}
	cert, err := os.ReadFile(m.certFile())
	if err != nil {
		return cerr.AppendError("failed to read registry certificate for host trust", err)
	}
	p.removeStaleHostTrust()
	for _, addr := range hostTrustAddrs {
		certsD := filepath.Join(p.certsDBase, fmt.Sprintf("%s:%d", addr, port))
		if err := os.MkdirAll(certsD, 0755); err != nil {
			return cerr.AppendErrorFmt("failed to create certs.d dir %s", err, certsD)
		}
		if err := os.WriteFile(filepath.Join(certsD, "ca.crt"), cert, 0644); err != nil {
			return cerr.AppendError("failed to write registry host trust", err)
		}
	}
	return nil
}

// hostTrustAddrs are the loopback names the registry certificate covers and
// host trust is installed for.
var hostTrustAddrs = []string{"127.0.0.1", "localhost"}

// removeStaleHostTrust removes loopback certs.d entries holding a CDS registry
// certificate (issued for Hostname), so trust for old ports does not pile up.
// Entries of other registries are left alone. Best effort.
func (p *podmanProvider) removeStaleHostTrust() {
	entries, err := os.ReadDir(p.certsDBase)
	if err != nil {
		return
	}
	for _, e := range entries {
		addr, _, ok := strings.Cut(e.Name(), ":")
		if !ok || !slices.Contains(hostTrustAddrs, addr) {
			continue
		}
		dir := filepath.Join(p.certsDBase, e.Name())
		if isRegistryCert(filepath.Join(dir, "ca.crt")) {
			_ = os.RemoveAll(dir)
		}
	}
}

// isRegistryCert reports whether path holds a certificate issued for Hostname.
func isRegistryCert(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	return err == nil && slices.Contains(cert.DNSNames, Hostname)
}

// hostPort returns the loopback host port podman published for the
// registry's container port.
func (p *podmanProvider) hostPort() (int, error) {
	out, err := p.runner(fmt.Sprintf("podman port %s %d/tcp", Name, p.port))
	if err != nil {
		return 0, cerr.AppendErrorFmt("failed to read registry host port: %s", err, out)
	}
	return parseHostPort(out)
}

// parseHostPort reads the port from `podman port` output such as
// "127.0.0.1:41234".
func parseHostPort(out string) (int, error) {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	i := strings.LastIndex(line, ":")
	port, err := strconv.Atoi(strings.TrimSpace(line[i+1:]))
	if err != nil || port <= 0 {
		return 0, cerr.NewError(fmt.Sprintf("registry %s publishes no host port: %q", Name, out))
	}
	return port, nil
}

func (p *podmanProvider) Start() error {
	// without its Quadlet file the unit does not exist and systemctl fails
	// with a bare "exit status 5": say what to run instead (CDS-100).
	if !fileExists(filepath.Join(p.quadletDir, quadletUnitFile)) {
		return cerr.NewError(fmt.Sprintf("registry %s does not exist: run `cds space registry run` to build it", Name))
	}
	// The Quadlet unit runs `podman run --network kind`, so the kind network
	// must exist. A `podman system reset` (host maintenance) wipes it, which
	// made a plain restart fail with the container unable to attach its network.
	// Re-ensure it here so start/restart after a reset is self-healing. The
	// material bind mounts under ~/.cds-agent/registry survive a reset.
	if err := engine.EnsureKindNetwork(p.engineName); err != nil {
		return cerr.AppendError("failed to ensure kind network for registry", err)
	}
	if err := p.units.Start(quadletService); err != nil {
		return cerr.AppendError("failed to start registry", err)
	}
	// a new start may publish a new host port: move host trust to it
	return p.installHostTrust(material{root: p.materialRoot})
}

func (p *podmanProvider) Stop() error {
	if p.units.IsActive(quadletService) {
		if err := p.units.Stop(quadletService); err != nil {
			return cerr.AppendError("failed to stop registry", err)
		}
		return nil
	}
	// the Quadlet container runs with --rm, so after a clean stop it is gone:
	// a missing container is already stopped, as in Delete.
	if out, err := p.runner(fmt.Sprintf("podman stop %s", Name)); err != nil && !isNoSuchContainer(out) {
		return cerr.AppendErrorFmt("failed to stop registry %s: %s", err, Name, out)
	}
	return nil
}

// Delete removes the Quadlet unit and the registry container. It deliberately
// leaves the material directory (~/.cds-agent/registry) in place, so the
// pushed images under data/ and the TLS material survive: a later Build reuses
// them (material generation is idempotent). Deleting the registry thus removes
// the service, not the stored images. force is accepted for interface parity.
//
// Every step is attempted, and any failure is returned, so delete
// never reports success while the unit or container is still there. What is
// already gone is not a failure: Stop only runs on an active unit, Remove
// tolerates a unit that is not loaded, and a missing container is ignored.
func (p *podmanProvider) Delete(force bool) error {
	var errs []error
	// stop the service before its unit file goes away: a running unit that
	// loses its file ends up failed.
	if p.units.IsActive(quadletService) {
		if err := p.units.Stop(quadletService); err != nil {
			errs = append(errs, cerr.AppendError("failed to stop registry service", err))
		}
	}
	if err := p.units.Remove(p.quadletDir, quadletUnitFile); err != nil {
		errs = append(errs, cerr.AppendError("failed to remove registry Quadlet unit", err))
	}
	// the Quadlet-managed container may still exist; remove it directly.
	if out, err := p.runner(fmt.Sprintf("podman rm -f %s", Name)); err != nil && !isNoSuchContainer(out) {
		errs = append(errs, cerr.AppendErrorFmt("failed to remove registry container %s: %s", err, Name, out))
	}
	if len(errs) > 0 {
		return cerr.AppendMultipleErrors("failed to delete registry", errs)
	}
	return nil
}

// isNoSuchContainer tells a podman "container already gone" reply apart from a
// real removal failure.
func isNoSuchContainer(out string) bool {
	return strings.Contains(strings.ToLower(out), "no such container")
}

func (p *podmanProvider) Status() (Registry, error) {
	status, err := engine.GetContainerStatus(Name, p.engineName)
	if err != nil {
		return Registry{}, cerr.AppendErrorFmt("failed to get registry status %s", err, Name)
	}
	r := Registry{
		Name:     Name,
		Status:   bo.FContainerStatus(p.unitAwareStatus(status)),
		Port:     p.port,
		Hostname: Hostname,
	}
	if status == bo.KContainerStatusRunning {
		// best effort: a status read never fails on the port lookup
		r.HostPort, _ = p.hostPort()
	}
	return r, nil
}

// unitAwareStatus reports a registry that was stopped as exited, not deleted.
// The Quadlet container runs with --rm, so stop removes the container; the
// installed unit file is what tells stop (unit kept) from delete (unit removed).
func (p *podmanProvider) unitAwareStatus(status bo.ContainerStatus) bo.ContainerStatus {
	if status == bo.KContainerStatusDeleted && fileExists(filepath.Join(p.quadletDir, quadletUnitFile)) {
		return bo.KContainerStatusExited
	}
	return status
}

// CACertificate returns the registry CA certificate PEM read from the on-host
// material. It is empty when the material has not been generated.
func (p *podmanProvider) CACertificate() (string, error) {
	m := material{root: p.materialRoot}
	if !fileExists(m.certFile()) {
		return cg.EmptyStr, nil
	}
	data, err := os.ReadFile(m.certFile())
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("failed to read registry CA certificate", err)
	}
	return string(data), nil
}

// Credentials returns the generated registry credentials from the on-host
// material. Empty when no material has been generated.
func (p *podmanProvider) Credentials() (string, string, error) {
	m := material{root: p.materialRoot}
	if !fileExists(m.credsFile()) {
		return cg.EmptyStr, cg.EmptyStr, nil
	}
	return readPersistedCredentials(m.credsFile())
}

func reportf(report ProgressReporter, phase, message string) error {
	if report == nil {
		return nil
	}
	return report(phase, message)
}
