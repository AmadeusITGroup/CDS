package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
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
// starts the registry and installs host trust. secure must be true: an insecure
// registry is not supported.
func (p *podmanProvider) Build(secure bool, report ProgressReporter) error {
	if !secure {
		return cerr.NewError("insecure registry is not supported; secure must be true")
	}
	if err := reportf(report, "material", "Generating registry material"); err != nil {
		return err
	}
	m, err := ensureMaterial(p.materialRoot)
	if err != nil {
		return err
	}

	if err := reportf(report, "unit", "Installing registry Quadlet unit"); err != nil {
		return err
	}
	if err := os.MkdirAll(p.quadletDir, 0755); err != nil {
		return cerr.AppendErrorFmt("failed to create Quadlet dir %s", err, p.quadletDir)
	}
	content := p.quadletUnit(m)
	if _, err := p.units.Reconcile(p.quadletDir, systemd.Unit{Name: quadletUnitFile, Content: content}); err != nil {
		return cerr.AppendError("failed to install registry Quadlet unit", err)
	}

	if err := reportf(report, "trust", "Installing registry host trust"); err != nil {
		return err
	}
	if err := p.installHostTrust(m); err != nil {
		return err
	}

	if err := reportf(report, "registry", fmt.Sprintf("Starting registry %s", Name)); err != nil {
		return err
	}
	// Quadlet-generated services are transient from systemd's view; a daemon
	// reload makes the generated unit visible, then start it.
	if err := p.units.Start(quadletService); err != nil {
		return cerr.AppendError("failed to start registry", err)
	}
	return reportf(report, "registry", fmt.Sprintf("Registry %s started", Name))
}

// quadletUnit builds the Quadlet .container unit. Data, certs and auth are
// mounted as volumes so state survives container recreation (Quadlet recreates
// the container on each start).
func (p *podmanProvider) quadletUnit(m material) []byte {
	// Quadlet .container is an INI-like file read by podman's systemd generator.
	return []byte(fmt.Sprintf(`[Unit]
Description=CDS secure registry (User)

[Container]
ContainerName=%s
Image=%s
PublishPort=%d:5000
Volume=%s:/certs:Z
Volume=%s:/auth:Z
Volume=%s:/var/lib/registry:Z
Environment=REGISTRY_HTTP_TLS_CERTIFICATE=/certs/domain.crt
Environment=REGISTRY_HTTP_TLS_KEY=/certs/domain.key
Environment=REGISTRY_AUTH=htpasswd
Environment=REGISTRY_AUTH_HTPASSWD_REALM=cds
Environment=REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd

[Install]
WantedBy=default.target
`,
		Name,
		Image,
		p.port,
		m.certsDir(),
		m.authDir(),
		m.dataDir(),
	))
}

// installHostTrust copies the registry certificate into the podman per-registry
// trust directory so the host (and kind nodes mounting it) trust the registry's
// self-signed certificate: ~/.config/containers/certs.d/<host>:<port>/ca.crt.
func (p *podmanProvider) installHostTrust(m material) error {
	certsD := filepath.Join(
		p.certsDBase,
		fmt.Sprintf("%s:%d", Hostname, p.port),
	)
	if err := os.MkdirAll(certsD, 0755); err != nil {
		return cerr.AppendErrorFmt("failed to create certs.d dir %s", err, certsD)
	}
	cert, err := os.ReadFile(m.certFile())
	if err != nil {
		return cerr.AppendError("failed to read registry certificate for host trust", err)
	}
	if err := os.WriteFile(filepath.Join(certsD, "ca.crt"), cert, 0644); err != nil {
		return cerr.AppendError("failed to write registry host trust", err)
	}
	return nil
}

func (p *podmanProvider) Start() error {
	if err := p.units.Start(quadletService); err != nil {
		return cerr.AppendError("failed to start registry", err)
	}
	return nil
}

func (p *podmanProvider) Stop() error {
	if p.units.IsActive(quadletService) {
		if err := p.units.Stop(quadletService); err != nil {
			return cerr.AppendError("failed to stop registry", err)
		}
		return nil
	}
	if out, err := p.runner(fmt.Sprintf("podman stop %s", Name)); err != nil {
		return cerr.AppendErrorFmt("failed to stop registry %s: %s", err, Name, out)
	}
	return nil
}

// Delete removes the Quadlet unit and the registry container. force is accepted
// for interface parity.
func (p *podmanProvider) Delete(force bool) error {
	if err := p.units.Remove(p.quadletDir, quadletUnitFile); err != nil {
		clog.Warn("failed to remove registry Quadlet unit", err)
	}
	// the Quadlet-managed container may still exist; remove it directly.
	if out, err := p.runner(fmt.Sprintf("podman rm -f %s", Name)); err != nil {
		clog.Debug(fmt.Sprintf("registry container removal returned: %s (%v)", out, err))
	}
	return nil
}

func (p *podmanProvider) Status() (Registry, error) {
	status, err := engine.GetContainerStatus(Name, p.engineName)
	if err != nil {
		return Registry{}, cerr.AppendErrorFmt("failed to get registry status %s", err, Name)
	}
	return Registry{
		Name:     Name,
		Status:   bo.FContainerStatus(status),
		Port:     p.port,
		Hostname: Hostname,
	}, nil
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

func reportf(report ProgressReporter, phase, message string) error {
	if report == nil {
		return nil
	}
	return report(phase, message)
}
