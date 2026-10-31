// Package registry deploys and supervises a secure local container registry on
// the agent host. Like internal/orchestration it runs entirely agent-side and
// depends only on the container engine, shell execution and systemd. It must
// not import internal/db, internal/config or internal/command.
//
// The registry serves TLS with a self-signed certificate for a fixed hostname
// and requires basic auth. All material (certificate, key, htpasswd) is
// generated in Go with random per-host credentials; the private key never
// leaves the host. The registry runs as a Quadlet container unit so podman
// supervises it (per the owner decision to use Quadlet on Linux).
package registry

const (
	// Name is the fixed registry container name CDS manages on a host.
	Name = "cds-sec-registry"
	// Hostname is the TLS server name the registry certificate is issued for
	// and the name containers use to reach it.
	Hostname = "cds.sec.io"
	// DefaultPort is the host port the registry publishes.
	DefaultPort = 5000
	// Image is the registry server image.
	Image = "docker.io/library/registry:2"
)

// ProgressReporter receives human-readable progress during a long-running
// registry operation (e.g. BuildRegistry).
type ProgressReporter func(phase, message string) error

// Registry is a point-in-time view of the managed registry on a host.
type Registry struct {
	// Name is the registry container name (always Name for now).
	Name string
	// Status is the live status of the registry container, derived from the
	// engine (podman inspect).
	Status string
	// Port is the host port the registry publishes.
	Port int
	// Hostname is the TLS server name of the registry.
	Hostname string
}

// Provider abstracts the registry engine behind the lifecycle. There is a
// single implementation today, but the seam mirrors the orchestration
// ClusterProvider so the service layer stays provider-agnostic.
type Provider interface {
	// Build generates the registry material if needed and (re)creates the
	// registry, streaming progress. secure reports whether auth+TLS are
	// required (an insecure registry is not supported yet; secure must be
	// true). Build is idempotent.
	Build(secure bool, report ProgressReporter) error
	// Start starts an existing, stopped registry.
	Start() error
	// Stop stops a running registry without deleting it.
	Stop() error
	// Delete removes the registry. force tears it down even when in use.
	Delete(force bool) error
	// Status returns the live registry view.
	Status() (Registry, error)
	// CACertificate returns the registry CA certificate PEM, so it can be
	// installed as trust inside devcontainers. It is empty when no material has
	// been generated yet.
	CACertificate() (string, error)
}
