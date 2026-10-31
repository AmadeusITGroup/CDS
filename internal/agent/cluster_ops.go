package agent

import (
	"github.com/amadeusitgroup/cds/internal/orchestration"
)

// clusterProgress is a progress signal emitted during a cluster build, mirrored
// to the gRPC stream by the service layer.
type clusterProgress struct {
	phase   string
	message string
}

// clusterProgressReporter receives cluster build progress.
type clusterProgressReporter func(clusterProgress) error

// clusterOps abstracts the orchestration engine so the gRPC layer can be tested
// without running kind/podman. It mirrors containerOps.
type clusterOps interface {
	BuildCluster(engineName, configFile string, report clusterProgressReporter) error
	StartCluster() error
	StopCluster() error
	DeleteCluster(force bool) error
	GetCluster() (orchestration.Cluster, error)
	GetKubeconfig() (string, error)
	ApplyMirrors(mirrors []orchestration.Mirror) error
	DeployIngress() error
	CreateRegistrySecret(registryHost string, port int, username, password string) error
	TrustRegistryOnNodes(registryHost string, port int, caPEM string) error
}

// defaultClusterOps drives the orchestration package on the agent host.
type defaultClusterOps struct{}

var _ clusterOps = defaultClusterOps{}

func (defaultClusterOps) BuildCluster(engineName, configFile string, report clusterProgressReporter) error {
	provider, err := orchestration.Provider(engineName)
	if err != nil {
		return err
	}
	return provider.Build(configFile, func(phase, message string) error {
		if report == nil {
			return nil
		}
		return report(clusterProgress{phase: phase, message: message})
	})
}

func (defaultClusterOps) StartCluster() error {
	provider, err := orchestration.Provider("")
	if err != nil {
		return err
	}
	return provider.Start()
}

func (defaultClusterOps) StopCluster() error {
	provider, err := orchestration.Provider("")
	if err != nil {
		return err
	}
	return provider.Stop()
}

func (defaultClusterOps) DeleteCluster(force bool) error {
	if err := guardClusterDelete("cluster", force); err != nil {
		return err
	}
	provider, err := orchestration.Provider("")
	if err != nil {
		return err
	}
	return provider.Delete(force)
}

func (defaultClusterOps) GetCluster() (orchestration.Cluster, error) {
	provider, err := orchestration.Provider("")
	if err != nil {
		return orchestration.Cluster{}, err
	}
	return provider.Status()
}

func (defaultClusterOps) GetKubeconfig() (string, error) {
	provider, err := orchestration.Provider("")
	if err != nil {
		return "", err
	}
	return provider.Kubeconfig()
}

func (defaultClusterOps) ApplyMirrors(mirrors []orchestration.Mirror) error {
	provider, err := orchestration.Provider("")
	if err != nil {
		return err
	}
	return provider.ApplyMirrors(mirrors)
}

func (defaultClusterOps) DeployIngress() error {
	provider, err := orchestration.Provider("")
	if err != nil {
		return err
	}
	return provider.DeployIngress()
}

func (defaultClusterOps) CreateRegistrySecret(registryHost string, port int, username, password string) error {
	provider, err := orchestration.Provider("")
	if err != nil {
		return err
	}
	return provider.CreateRegistrySecret(registryHost, port, username, password)
}

func (defaultClusterOps) TrustRegistryOnNodes(registryHost string, port int, caPEM string) error {
	provider, err := orchestration.Provider("")
	if err != nil {
		return err
	}
	return provider.TrustRegistryOnNodes(registryHost, port, caPEM)
}
