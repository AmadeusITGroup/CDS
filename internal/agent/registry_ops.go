package agent

import (
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
// without running podman. It mirrors clusterOps.
type registryOps interface {
	BuildRegistry(secure bool, report registryProgressReporter) error
	StartRegistry() error
	StopRegistry() error
	DeleteRegistry(force bool) error
	GetRegistry() (registry.Registry, error)
}

// defaultRegistryOps drives the registry package on the agent host.
type defaultRegistryOps struct{}

var _ registryOps = defaultRegistryOps{}

func (defaultRegistryOps) BuildRegistry(secure bool, report registryProgressReporter) error {
	return registry.New().Build(secure, func(phase, message string) error {
		if report == nil {
			return nil
		}
		return report(registryProgress{phase: phase, message: message})
	})
}

func (defaultRegistryOps) StartRegistry() error        { return registry.New().Start() }
func (defaultRegistryOps) StopRegistry() error         { return registry.New().Stop() }
func (defaultRegistryOps) DeleteRegistry(f bool) error { return registry.New().Delete(f) }
func (defaultRegistryOps) GetRegistry() (registry.Registry, error) {
	return registry.New().Status()
}
