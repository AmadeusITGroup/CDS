package agent

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	cg "github.com/amadeusitgroup/cds/internal/global"
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

	// the cluster and registry goroutines share one gRPC stream; grpc-go forbids
	// concurrent SendMsg, so serialize every report call.
	var mu sync.Mutex
	rawReport := report
	report = func(p deployProgress) error {
		mu.Lock()
		defer mu.Unlock()
		return rawReport(p)
	}

	// the ops get the request ctx, not the group ctx: a failed registry build
	// must not kill a kind create that is half way through.
	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		err := cluster.BuildCluster(ctx, oc.Engine, oc.ConfigFile, func(p clusterProgress) error {
			return report(deployProgress{phase: "cluster", message: p.message})
		})
		if err != nil {
			return cerr.AppendError("failed to ensure cluster", err)
		}
		if err := applyMirrors(ctx, cluster, oc.Mirrors, report); err != nil {
			return err
		}
		if oc.Ingress {
			return deployIngress(ctx, cluster, report)
		}
		return nil
	})

	if oc.Registry.Requested {
		g.Go(func() error {
			err := reg.BuildRegistry(ctx, oc.Registry.Secure, func(p registryProgress) error {
				return report(deployProgress{phase: "registry", message: p.message})
			})
			if err != nil {
				return cerr.AppendError("failed to ensure registry", err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Once both the cluster and registry are up, make the cluster trust and pull
	// from the registry.
	if oc.Registry.Requested {
		return connectRegistryToCluster(ctx, cluster, reg, oc.Registry.Port, report)
	}
	return nil
}

// applyMirrors writes the registry mirrors on the cluster nodes.
func applyMirrors(ctx context.Context, cluster clusterOps, mirrors []containerconf.Mirror, report deployProgressReporter) error {
	if len(mirrors) == 0 {
		return nil
	}
	if err := report(deployProgress{phase: "cluster", message: "Applying registry mirrors"}); err != nil {
		return err
	}
	if err := cluster.ApplyMirrors(ctx, toOrchestrationMirrors(mirrors)); err != nil {
		return cerr.AppendError("failed to apply registry mirrors", err)
	}
	return nil
}

// deployIngress installs the ingress controller in the cluster.
func deployIngress(ctx context.Context, cluster clusterOps, report deployProgressReporter) error {
	if err := report(deployProgress{phase: "cluster", message: "Deploying ingress controller"}); err != nil {
		return err
	}
	if err := cluster.DeployIngress(ctx); err != nil {
		return cerr.AppendError("failed to deploy ingress", err)
	}
	return nil
}

// connectRegistryToCluster installs the registry CA on the cluster nodes and
// creates the in-cluster pull secret (the documented cdsregistrycred secret +
// default service account patch), so pods can pull from the local registry.
// port 0 means the default registry port.
func connectRegistryToCluster(ctx context.Context, cluster clusterOps, reg registryOps, port int, report deployProgressReporter) error {
	user, pass, err := reg.RegistryCredentials()
	if err != nil {
		return cerr.AppendError("failed to resolve registry credentials for pull secret", err)
	}
	if port == 0 {
		port = registry.DefaultPort
	}
	// install the registry CA on the nodes so containerd trusts the registry
	// over TLS, otherwise pods fail with "certificate signed by unknown
	// authority" when pulling.
	if ca, caErr := reg.RegistryCA(); caErr == nil && ca != "" {
		if err := report(deployProgress{phase: "cluster", message: "Trusting registry CA on cluster nodes"}); err != nil {
			return err
		}
		if err := cluster.TrustRegistryOnNodes(ctx, registry.Hostname, port, ca); err != nil {
			return cerr.AppendError("failed to trust registry CA on cluster nodes", err)
		}
	}
	if err := report(deployProgress{phase: "cluster", message: "Creating in-cluster registry pull secret"}); err != nil {
		return err
	}
	if err := cluster.CreateRegistrySecret(ctx, registry.Hostname, port, user, pass); err != nil {
		return cerr.AppendError("failed to create in-cluster registry pull secret", err)
	}
	return nil
}

// registryEnv carries the registry connection details injected into the
// devcontainer so a user can log in and push to the local registry.
type registryEnv struct {
	name     string
	port     int
	caPath   string
	user     string
	password string
}

func (r registryEnv) set() bool { return r.name != "" && r.user != "" }

// newRegistryEnv is the registry env for a deployment whose registry listens
// on port (0 means the default port). It is unset when user is empty.
func newRegistryEnv(port int, user, password string) registryEnv {
	if user == cg.EmptyStr {
		return registryEnv{}
	}
	if port == 0 {
		port = registry.DefaultPort
	}
	return registryEnv{
		name:     registry.Hostname,
		port:     port,
		caPath:   fmt.Sprintf("/etc/containers/certs.d/%s:%d/ca.crt", registry.Hostname, port),
		user:     user,
		password: password,
	}
}

// containerDecorationCommands builds the exec commands that decorate a running
// devcontainer so it can use the cluster and registry: the cluster kubeconfig
// is installed at the container's default kubeconfig path, the registry CA in
// the container's per-registry trust directory, and the CDS_REGISTRY_* env in a
// profile script. The commands run as root inside the container.
//
// kubeconfig is the cluster kubeconfig and caCert the registry CA PEM; each
// step is skipped when its material is empty (or reg is unset).
func containerDecorationCommands(kubeconfig, caCert string, reg registryEnv) []decorationCommand {
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
	if reg.set() {
		cmds = append(cmds, decorationCommand{
			info:   "Exporting registry environment in container",
			script: installRegistryEnvScript(reg),
		})
	}
	return cmds
}

// installRegistryEnvScript writes the CDS_REGISTRY_* variables to a profile
// script in the container so interactive shells can docker login and push to
// the local registry.
func installRegistryEnvScript(reg registryEnv) string {
	return fmt.Sprintf(`set -eu
mkdir -p /etc/profile.d
cat > /etc/profile.d/cds-registry.sh <<'CDS_EOF'
export CDS_REGISTRY_NAME=%s
export CDS_REGISTRY_PORT=%d
export CDS_REGISTRY_PATH_TO_CA=%s
export CDS_REGISTRY_USR=%s
export CDS_REGISTRY_PWD=%s
CDS_EOF
chmod 644 /etc/profile.d/cds-registry.sh`,
		reg.name, reg.port, reg.caPath, reg.user, reg.password)
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
		user, pass, err := s.core.registryOps().RegistryCredentials()
		if err != nil {
			return spec, cerr.AppendError("failed to resolve registry credentials for decoration", err)
		}
		spec.registryUser, spec.registryPassword = user, pass
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
	deployed, ok := s.orchestratedDeployments()
	if !ok {
		return
	}

	var kubeconfig, registryCA, registryUser, registryPassword string
	for _, d := range deployed {
		oc := d.oc
		if !s.containerRunning(d.name) {
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

		// a rebuilt registry has new credentials: refresh CDS_REGISTRY_* too (CDS-042)
		if registryUser == "" && oc.Registry.Requested {
			if user, pass, err := s.registryOps().RegistryCredentials(); err == nil {
				registryUser, registryPassword = user, pass
			} else {
				clog.Warn("failed to resolve registry credentials for re-decoration", err)
			}
		}

		ca, reg := "", registryEnv{}
		if oc.Registry.Requested {
			ca, reg = registryCA, newRegistryEnv(oc.Registry.Port, registryUser, registryPassword)
		}
		s.redecorateOne(ctx, d.name, containerDecorationCommands(kubeconfig, ca, reg), report)
	}
}

// orchestratedDeployment is a recorded deployment that requested orchestration.
type orchestratedDeployment struct {
	name string
	oc   containerconf.KindOrchestrationConfig
}

// orchestratedDeployments lists the recorded deployments that requested
// orchestration; ok is false when the record cannot be read.
func (s *grpcServer) orchestratedDeployments() (out []orchestratedDeployment, ok bool) {
	reqs, err := s.deployments().list()
	if err != nil {
		clog.Warn("failed to list deployments for re-decoration", err)
		return nil, false
	}
	for _, req := range reqs {
		config, err := containerconf.ParseBytes(bytes.NewReader(req.GetDevcontainerConfig()))
		if err != nil {
			clog.Warn(fmt.Sprintf("skipping re-decoration of %s: invalid config", req.GetContainerName()), err)
			continue
		}
		if oc := config.OrchestrationConfig(); oc.Requested {
			out = append(out, orchestratedDeployment{name: req.GetContainerName(), oc: oc})
		}
	}
	return out, true
}

// reapplyOrchestration re-runs the post-build steps of ensureOrchestration for
// every orchestration setting found in the recorded deployments (CDS-031): a
// deleted and rebuilt cluster has lost them, and a rebuilt registry has a new
// CA and credentials the nodes and pull secret must get.
//
// clusterRebuilt re-applies everything (mirrors, ingress, registry trust and
// pull secret); otherwise only the registry steps, and only on a running
// cluster. Like re-decoration it is best effort: a failed step is logged and
// reported, the other steps still run, and the build itself stays a success.
func (s *grpcServer) reapplyOrchestration(ctx context.Context, clusterRebuilt bool, report deployProgressReporter) {
	deployed, ok := s.orchestratedDeployments()
	if !ok || len(deployed) == 0 {
		return
	}
	if !clusterRebuilt {
		if c, err := s.clusterOps().GetCluster(); err != nil || c.Status != bo.KContainerStatusRunning {
			return
		}
	}

	var mirrors []containerconf.Mirror
	seenMirror := map[containerconf.Mirror]bool{}
	ingress := false
	var ports []int
	seenPort := map[int]bool{}
	for _, d := range deployed {
		for _, m := range d.oc.Mirrors {
			if !seenMirror[m] {
				seenMirror[m] = true
				mirrors = append(mirrors, m)
			}
		}
		ingress = ingress || d.oc.Ingress
		if p := d.oc.Registry.Port; d.oc.Registry.Requested && !seenPort[p] {
			seenPort[p] = true
			ports = append(ports, p)
		}
	}

	warn := func(err error) {
		if err == nil {
			return
		}
		clog.Warn("failed to re-apply orchestration setting", err)
		_ = report(deployProgress{phase: "cluster", message: fmt.Sprintf("Warning: %v", err)})
	}
	if clusterRebuilt {
		warn(applyMirrors(ctx, s.clusterOps(), mirrors, report))
		if ingress {
			warn(deployIngress(ctx, s.clusterOps(), report))
		}
	}
	for _, port := range ports {
		warn(connectRegistryToCluster(ctx, s.clusterOps(), s.registryOps(), port, report))
	}
}

// redecorateOne runs the decoration install scripts inside a single container
// as root, each fed on stdin (they carry the kubeconfig and registry secrets).
func (s *grpcServer) redecorateOne(ctx context.Context, name string, cmds []decorationCommand, report deployProgressReporter) {
	for _, dc := range cmds {
		if report != nil {
			_ = report(deployProgress{phase: "decorate", message: fmt.Sprintf("%s: %s", name, dc.info)})
		}
		_, _, err := s.containerOps().ExecuteScriptInContainer(ctx, name, dc.script, "root")
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
