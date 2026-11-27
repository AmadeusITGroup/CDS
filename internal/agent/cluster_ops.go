package agent

import (
	"context"

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
//
// The operations that change the cluster take the request context and are
// serialized on one lock (CDS-029); the read-only ones are not.
type clusterOps interface {
	BuildCluster(ctx context.Context, engineName, configFile string, report clusterProgressReporter) error
	StartCluster(ctx context.Context) error
	StopCluster(ctx context.Context) error
	DeleteCluster(ctx context.Context, force bool) error
	GetCluster() (orchestration.Cluster, error)
	GetKubeconfig() (string, error)
	ApplyMirrors(ctx context.Context, mirrors []orchestration.Mirror) error
	PruneMirrors(ctx context.Context, keep []orchestration.Mirror) error
	DeployIngress(ctx context.Context) error
	CreateRegistrySecret(ctx context.Context, registryHost string, port int, username, password string) error
	TrustRegistryOnNodes(ctx context.Context, registryHost string, port int, caPEM string) error
}

// defaultClusterOps drives the orchestration package on the agent host.
type defaultClusterOps struct{}

var _ clusterOps = defaultClusterOps{}

// withCluster runs op on the provider for engineName while holding the cluster
// lock.
func withCluster(ctx context.Context, engineName string, op func(orchestration.ClusterProvider) error) error {
	return locked(ctx, sClusterLock, "cluster", func() error {
		provider, err := orchestration.Provider(engineName)
		if err != nil {
			return err
		}
		return op(provider)
	})
}

func (defaultClusterOps) BuildCluster(ctx context.Context, engineName, configFile string, report clusterProgressReporter) error {
	return withCluster(ctx, engineName, func(provider orchestration.ClusterProvider) error {
		return provider.Build(ctx, configFile, func(phase, message string) error {
			if report == nil {
				return nil
			}
			return report(clusterProgress{phase: phase, message: message})
		})
	})
}

func (defaultClusterOps) StartCluster(ctx context.Context) error {
	return withCluster(ctx, "", orchestration.ClusterProvider.Start)
}

func (defaultClusterOps) StopCluster(ctx context.Context) error {
	return withCluster(ctx, "", orchestration.ClusterProvider.Stop)
}

func (defaultClusterOps) DeleteCluster(ctx context.Context, force bool) error {
	return withCluster(ctx, "", func(provider orchestration.ClusterProvider) error {
		if err := guardClusterDelete("cluster", force); err != nil {
			return err
		}
		return provider.Delete(force)
	})
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

func (defaultClusterOps) ApplyMirrors(ctx context.Context, mirrors []orchestration.Mirror) error {
	return withCluster(ctx, "", func(provider orchestration.ClusterProvider) error {
		return provider.ApplyMirrors(mirrors)
	})
}

func (defaultClusterOps) PruneMirrors(ctx context.Context, keep []orchestration.Mirror) error {
	return withCluster(ctx, "", func(provider orchestration.ClusterProvider) error {
		return provider.PruneMirrors(keep)
	})
}

func (defaultClusterOps) DeployIngress(ctx context.Context) error {
	return withCluster(ctx, "", func(provider orchestration.ClusterProvider) error {
		return provider.DeployIngress(ctx)
	})
}

func (defaultClusterOps) CreateRegistrySecret(ctx context.Context, registryHost string, port int, username, password string) error {
	return withCluster(ctx, "", func(provider orchestration.ClusterProvider) error {
		return provider.CreateRegistrySecret(ctx, registryHost, port, username, password)
	})
}

func (defaultClusterOps) TrustRegistryOnNodes(ctx context.Context, registryHost string, port int, caPEM string) error {
	return withCluster(ctx, "", func(provider orchestration.ClusterProvider) error {
		return provider.TrustRegistryOnNodes(registryHost, port, caPEM)
	})
}
