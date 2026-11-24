package agent

import (
	"context"

	"github.com/amadeusitgroup/cds/internal/registry"
)

// registryProgress is a progress signal emitted during a registry build.
type registryProgress struct {
	phase   string
	message string
}

// registryProgressReporter receives registry build progress.
type registryProgressReporter func(registryProgress) error

// registryOps abstracts the registry engine so the gRPC layer can be tested
// without running podman. It mirrors clusterOps: the operations that change the
// registry take the request context and are serialized on one lock (CDS-029).
type registryOps interface {
	BuildRegistry(ctx context.Context, report registryProgressReporter) error
	StartRegistry(ctx context.Context) error
	StopRegistry(ctx context.Context) error
	DeleteRegistry(ctx context.Context, force bool) error
	GetRegistry() (registry.Registry, error)
	RegistryCA() (string, error)
	RegistryCredentials() (username, password string, err error)
}

// defaultRegistryOps drives the registry package on the agent host.
type defaultRegistryOps struct{}

var _ registryOps = defaultRegistryOps{}

func (defaultRegistryOps) BuildRegistry(ctx context.Context, report registryProgressReporter) error {
	return locked(ctx, sRegistryLock, "registry", func() error {
		return registry.New().Build(func(phase, message string) error {
			if report == nil {
				return nil
			}
			return report(registryProgress{phase: phase, message: message})
		})
	})
}

func (defaultRegistryOps) StartRegistry(ctx context.Context) error {
	return locked(ctx, sRegistryLock, "registry", func() error { return registry.New().Start() })
}

func (defaultRegistryOps) StopRegistry(ctx context.Context) error {
	return locked(ctx, sRegistryLock, "registry", func() error { return registry.New().Stop() })
}

func (defaultRegistryOps) DeleteRegistry(ctx context.Context, f bool) error {
	return locked(ctx, sRegistryLock, "registry", func() error {
		if err := guardClusterDelete("registry", f); err != nil {
			return err
		}
		return registry.New().Delete(f)
	})
}

func (defaultRegistryOps) GetRegistry() (registry.Registry, error) {
	return registry.New().Status()
}

func (defaultRegistryOps) RegistryCA() (string, error) {
	return registry.New().CACertificate()
}

func (defaultRegistryOps) RegistryCredentials() (string, string, error) {
	return registry.New().Credentials()
}
