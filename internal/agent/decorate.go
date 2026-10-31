package agent

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/registry"
	"golang.org/x/sync/errgroup"
)

// ensureOrchestration makes sure the cluster and (when requested) the registry
// are built before a devcontainer that needs them is deployed. Per the owner
// decision the cluster and registry are brought up CONCURRENTLY, not
// sequentially, matching the enterprise behaviour. Progress from both is
// streamed through report with a per-resource phase prefix.
//
// It is a no-op when the devcontainer did not request orchestration.
func ensureOrchestration(
	ctx context.Context,
	cluster clusterOps,
	reg registryOps,
	oc containerconf.KindOrchestrationConfig,
	report deployProgressReporter,
) error {
	if !oc.Requested {
		return nil
	}

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		err := cluster.BuildCluster(oc.Engine, oc.ConfigFile, func(p clusterProgress) error {
			return report(deployProgress{phase: "cluster", message: p.message})
		})
		if err != nil {
			return cerr.AppendError("failed to ensure cluster", err)
		}
		return nil
	})

	if oc.Registry.Requested {
		g.Go(func() error {
			err := reg.BuildRegistry(oc.Registry.Secure, func(p registryProgress) error {
				return report(deployProgress{phase: "registry", message: p.message})
			})
			if err != nil {
				return cerr.AppendError("failed to ensure registry", err)
			}
			return nil
		})
	}

	return g.Wait()
}

// containerDecorationCommands builds the exec commands that decorate a running
// devcontainer so it can use the cluster and registry: the cluster kubeconfig
// is installed at the container's default kubeconfig path, and the registry CA
// is installed in the container's per-registry trust directory. The commands
// run as root inside the container.
//
// kubeconfig is the cluster kubeconfig (may be empty to skip), caCert is the
// registry CA PEM (may be empty to skip).
func containerDecorationCommands(engineName, containerName, kubeconfig, caCert string) []decorationCommand {
	var cmds []decorationCommand
	if kubeconfig != "" {
		cmds = append(cmds, decorationCommand{
			info:   "Installing cluster kubeconfig in container",
			script: installKubeconfigScript(kubeconfig),
		})
	}
	if caCert != "" {
		cmds = append(cmds, decorationCommand{
			info:   "Installing registry trust in container",
			script: installRegistryTrustScript(caCert),
		})
	}
	return cmds
}

// decorationCommand is a shell snippet run as root inside the container with a
// human-readable description.
type decorationCommand struct {
	info   string
	script string
}

func installKubeconfigScript(kubeconfig string) string {
	// write the kubeconfig to root's default path; a here-doc keeps the content
	// intact regardless of quoting.
	return fmt.Sprintf(`set -eu
mkdir -p /root/.kube
cat > /root/.kube/config <<'CDS_EOF'
%s
CDS_EOF
chmod 600 /root/.kube/config`, kubeconfig)
}

func installRegistryTrustScript(caCert string) string {
	dir := fmt.Sprintf("/etc/containers/certs.d/%s:%d", registry.Hostname, registry.DefaultPort)
	return fmt.Sprintf(`set -eu
mkdir -p %q
cat > %q/ca.crt <<'CDS_EOF'
%s
CDS_EOF`, dir, dir, caCert)
}

// ensureAndResolveDecoration ensures the cluster and registry are up
// (concurrently) and resolves the material to install into the container (the
// cluster kubeconfig and, when a registry is requested, its CA). It returns the
// spec enriched with that material.
func (s *containerServiceServer) ensureAndResolveDecoration(
	ctx context.Context,
	spec deployContainerSpec,
	report deployProgressReporter,
) (deployContainerSpec, error) {
	if err := ensureOrchestration(ctx, s.core.clusterOps(), s.core.registryOps(), spec.orchestration, report); err != nil {
		return spec, err
	}

	kubeconfig, err := s.core.clusterOps().GetKubeconfig()
	if err != nil {
		return spec, cerr.AppendError("failed to resolve cluster kubeconfig for decoration", err)
	}
	spec.kubeconfig = kubeconfig

	if spec.orchestration.Registry.Requested {
		ca, err := s.core.registryOps().RegistryCA()
		if err != nil {
			return spec, cerr.AppendError("failed to resolve registry CA for decoration", err)
		}
		spec.registryCA = ca
	}
	return spec, nil
}
