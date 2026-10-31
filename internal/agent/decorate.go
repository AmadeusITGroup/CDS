package agent

import (
	"bytes"
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/orchestration"
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
		if len(oc.Mirrors) > 0 {
			if err := report(deployProgress{phase: "cluster", message: "Applying registry mirrors"}); err != nil {
				return err
			}
			if err := cluster.ApplyMirrors(toOrchestrationMirrors(oc.Mirrors)); err != nil {
				return cerr.AppendError("failed to apply registry mirrors", err)
			}
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

// redecorateContainers re-installs the cluster kubeconfig and registry trust
// into every running CDS container whose recorded deployment requested
// orchestration. It is called after a cluster or registry (re)build so that
// containers deployed before the rebuild pick up the fresh kubeconfig and CA
// instead of stale material. Failures for a single container are logged and
// skipped so one bad container does not fail the whole rebuild.
//
// It is best effort and never returns an error that should fail the build: a
// rebuild succeeds even if re-decoration of some container does not.
func (s *grpcServer) redecorateContainers(ctx context.Context, report deployProgressReporter) {
	reqs, err := s.deployments().list()
	if err != nil {
		clog.Warn("failed to list deployments for re-decoration", err)
		return
	}

	var kubeconfig, registryCA string
	for _, req := range reqs {
		config, err := containerconf.ParseBytes(bytes.NewReader(req.GetDevcontainerConfig()))
		if err != nil {
			clog.Warn(fmt.Sprintf("skipping re-decoration of %s: invalid config", req.GetContainerName()), err)
			continue
		}
		oc := config.OrchestrationConfig()
		if !oc.Requested {
			continue
		}
		if !s.containerRunning(req.GetContainerName()) {
			continue
		}

		// resolve material once, lazily, only when there is a container to decorate
		if kubeconfig == "" {
			if kc, err := s.clusterOps().GetKubeconfig(); err == nil {
				kubeconfig = kc
			} else {
				clog.Warn("failed to resolve kubeconfig for re-decoration", err)
				return
			}
		}
		if registryCA == "" && oc.Registry.Requested {
			if ca, err := s.registryOps().RegistryCA(); err == nil {
				registryCA = ca
			}
		}

		ca := ""
		if oc.Registry.Requested {
			ca = registryCA
		}
		s.redecorateOne(ctx, req.GetContainerName(), kubeconfig, ca, report)
	}
}

// redecorateOne runs the decoration install scripts inside a single container
// via ExecuteInContainer as root.
func (s *grpcServer) redecorateOne(ctx context.Context, name, kubeconfig, ca string, report deployProgressReporter) {
	for _, dc := range containerDecorationCommands("", name, kubeconfig, ca) {
		if report != nil {
			_ = report(deployProgress{phase: "decorate", message: fmt.Sprintf("%s: %s", name, dc.info)})
		}
		_, _, err := s.containerOps().ExecuteInContainer(ctx, name, dc.script, "root")
		if err != nil {
			clog.Warn(fmt.Sprintf("failed to re-decorate container %s (%s)", name, dc.info), err)
		}
	}
}

// containerRunning reports whether the named container is currently running.
func (s *grpcServer) containerRunning(name string) bool {
	c, err := s.containerOps().GetContainer(name)
	if err != nil {
		return false
	}
	return c.Status == bo.KContainerStatusRunning
}

// toOrchestrationMirrors converts config mirrors to the orchestration type.
func toOrchestrationMirrors(mirrors []containerconf.Mirror) []orchestration.Mirror {
	out := make([]orchestration.Mirror, 0, len(mirrors))
	for _, m := range mirrors {
		out = append(out, orchestration.Mirror{Registry: m.Registry, Endpoint: m.Endpoint})
	}
	return out
}
