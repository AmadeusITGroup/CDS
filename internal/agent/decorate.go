package agent

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/orchestration"
	"github.com/amadeusitgroup/cds/internal/registry"
	"golang.org/x/sync/errgroup"
)

// ensureOrchestration makes sure the cluster and (when requested) the registry
// are built before a devcontainer that needs them is deployed. The cluster
// and registry are brought up CONCURRENTLY, not sequentially, to keep deploys
// short. Progress from both is
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
	if oc.Registry.Requested && oc.Registry.InsecureRequested {
		// fail before building anything: only a secured registry exists (CDS-034)
		return cerr.NewError("insecure registry is not supported; remove orchestration.registry.secure or set it to true")
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
			err := reg.BuildRegistry(ctx, func(p registryProgress) error {
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
		return connectRegistryToCluster(ctx, cluster, reg, report)
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
// The registry always listens on registry.DefaultPort (CDS-033).
func connectRegistryToCluster(ctx context.Context, cluster clusterOps, reg registryOps, report deployProgressReporter) error {
	user, pass, err := reg.RegistryCredentials()
	if err != nil {
		return cerr.AppendError("failed to resolve registry credentials for pull secret", err)
	}
	// install the registry CA on the nodes so containerd trusts the registry
	// over TLS, otherwise pods fail with "certificate signed by unknown
	// authority" when pulling.
	if ca, caErr := reg.RegistryCA(); caErr == nil && ca != "" {
		if err := report(deployProgress{phase: "cluster", message: "Trusting registry CA on cluster nodes"}); err != nil {
			return err
		}
		if err := cluster.TrustRegistryOnNodes(ctx, registry.Hostname, registry.DefaultPort, ca); err != nil {
			return cerr.AppendError("failed to trust registry CA on cluster nodes", err)
		}
	}
	if err := report(deployProgress{phase: "cluster", message: "Creating in-cluster registry pull secret"}); err != nil {
		return err
	}
	if err := cluster.CreateRegistrySecret(ctx, registry.Hostname, registry.DefaultPort, user, pass); err != nil {
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
	// owner is the container user (remoteUser) the env file is written for;
	// only that user can read it (CDS-024).
	owner string
}

func (r registryEnv) set() bool { return r.name != "" && r.user != "" }

// newRegistryEnv is the registry env for a deployment, written for the
// container user owner (empty means root). The registry always listens on
// registry.DefaultPort (CDS-033). It is unset when user is empty.
func newRegistryEnv(user, password, owner string) registryEnv {
	if user == cg.EmptyStr {
		return registryEnv{}
	}
	if owner == cg.EmptyStr {
		owner = engine.KRootUsr
	}
	return registryEnv{
		name:     registry.Hostname,
		port:     registry.DefaultPort,
		caPath:   fmt.Sprintf("/etc/containers/certs.d/%s:%d/ca.crt", registry.Hostname, registry.DefaultPort),
		user:     user,
		password: password,
		owner:    owner,
	}
}

// containerDecorationCommands builds the exec commands that decorate a running
// devcontainer so it can use the cluster and registry: the cluster kubeconfig
// is installed at the container's default kubeconfig path, the registry CA in
// the container's per-registry trust directory, and the CDS_REGISTRY_* env in a
// 0600 file in the container user's home. The commands run as root inside the container.
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

// registryEnvFile is the env file, relative to the container user's home, that
// holds the CDS_REGISTRY_* variables.
const registryEnvFile = ".config/cds/registry.env"

// installRegistryEnvScript writes the CDS_REGISTRY_* variables to a 0600 env
// file owned by the container user, in that user's home, and sources it from
// the user's shell rc files, so the user's shells can podman login and push to
// the local registry while other container users cannot read the password
// (CDS-024). It also removes the world-readable /etc/profile.d script that
// earlier versions wrote.
//
// The script runs as root: the file is written to a fresh temp file and
// renamed, so a link planted at the target is replaced, not followed. For the
// same reason an rc file that is a link, not a regular file, or not owned by
// the user is skipped with a warning, never appended to (CDS-054), and a
// ~/.config or ~/.config/cds that is a link, not a dir, or not owned by the
// user makes the script skip the env file and rc files with a warning (CDS-055).
// Ownership is read with ls, not find, which minimal images may lack; an owner
// it cannot read is warned about as such, not as "not owned" (CDS-057).
func installRegistryEnvScript(reg registryEnv) string {
	return fmt.Sprintf(`set -eu
u=%s
home=$(getent passwd "$u" 2>/dev/null | cut -d: -f6) || home=
[ -n "$home" ] || home=$(awk -F: -v u="$u" '$1 == u { print $6 }' /etc/passwd)
[ -n "$home" ] || { echo "container user $u has no home directory" >&2; exit 1; }
g=$(id -g "$u")
uid=$(id -u "$u")
# owned: is $1 owned by the user? ls, not find: minimal images lack find (CDS-057).
owned() {
  o=$(ls -nd "$1" 2>/dev/null) || o=
  set -f; set -- "$1" $o; set +f
  [ -n "${4-}" ] || { echo "cds: cannot tell who owns $1" >&2; return 1; }
  [ "$4" = "$uid" ]
}
rm -f /etc/profile.d/cds-registry.sh
# root must not create dirs or write through a user-planted dir link (CDS-055).
# shortcut: check-then-write, so a user racing the script can still swap the
# dir; write as the user (runuser) if that matters.
for d in "$home/.config" "$home/.config/cds"; do
  if [ -L "$d" ] || { [ -e "$d" ] && { [ ! -d "$d" ] || ! owned "$d"; }; }; then
    echo "cds: $d is a link or not a directory owned by $u, $home/%s not written" >&2; exit 0
  fi
  if [ ! -d "$d" ]; then mkdir -m 700 "$d"; chown "$u:$g" "$d"; fi
done
tmp=$(mktemp "$home/.config/cds/.registry.env.XXXXXX")
cat > "$tmp" <<'CDS_EOF'
export CDS_REGISTRY_NAME=%s
export CDS_REGISTRY_PORT=%d
export CDS_REGISTRY_PATH_TO_CA=%s
export CDS_REGISTRY_USR=%s
export CDS_REGISTRY_PWD=%s
CDS_EOF
chmod 600 "$tmp"
chown "$u:$g" "$tmp"
mv -f "$tmp" "$home/%s"
line='if [ -r "$HOME/%s" ]; then . "$HOME/%s"; fi # cds registry env'
for rc in .profile .bashrc .bash_profile .zshrc; do
  f="$home/$rc"
  # root must not write through a user-planted link, nor into a file the user
  # does not own (a hard link to a root file) (CDS-054).
  # shortcut: check-then-append, so a user racing the script can still swap
  # the file; append as the user (runuser) if that matters.
  if [ -L "$f" ]; then
    echo "cds: $f is a link, not changed; add to it: $line" >&2; continue
  fi
  # .profile is created when missing (sh/dash/bash login shells read it);
  # the others are only appended to when the user already has them.
  if [ "$rc" != .profile ] && [ ! -e "$f" ]; then continue; fi
  if [ -e "$f" ]; then
    if [ ! -f "$f" ] || ! owned "$f"; then
      echo "cds: $f is not a regular file owned by $u, not changed; add to it: $line" >&2; continue
    fi
    if grep -qF "$line" "$f"; then continue; fi
  fi
  new=0; [ -e "$f" ] || new=1
  printf '%%s\n' "$line" >> "$f"
  if [ "$new" = 1 ]; then chown "$u:$g" "$f"; fi
done`,
		cg.ShellSingleQuote(reg.owner),
		registryEnvFile,
		reg.name, reg.port, reg.caPath, reg.user, reg.password,
		registryEnvFile, registryEnvFile, registryEnvFile)
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
			ca, reg = registryCA, newRegistryEnv(registryUser, registryPassword, d.remoteUser)
		}
		s.redecorateOne(ctx, d.name, containerDecorationCommands(kubeconfig, ca, reg), report)
	}
}

// orchestratedDeployment is a recorded deployment that requested orchestration.
type orchestratedDeployment struct {
	name string
	oc   containerconf.KindOrchestrationConfig
	// remoteUser is the container user the registry env is written for.
	remoteUser string
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
			out = append(out, orchestratedDeployment{name: req.GetContainerName(), oc: oc, remoteUser: engine.ResolveRemoteUserFromConfig(config)})
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
	ingress, registryRequested := false, false
	for _, d := range deployed {
		for _, m := range d.oc.Mirrors {
			if !seenMirror[m] {
				seenMirror[m] = true
				mirrors = append(mirrors, m)
			}
		}
		ingress = ingress || d.oc.Ingress
		registryRequested = registryRequested || d.oc.Registry.Requested
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
	if registryRequested {
		warn(connectRegistryToCluster(ctx, s.clusterOps(), s.registryOps(), report))
	}
}

// redecorateOne runs the decoration install scripts inside a single container
// as root, each fed on stdin (they carry the kubeconfig and registry secrets).
func (s *grpcServer) redecorateOne(ctx context.Context, name string, cmds []decorationCommand, report deployProgressReporter) {
	for _, dc := range cmds {
		if report != nil {
			_ = report(deployProgress{phase: "decorate", message: fmt.Sprintf("%s: %s", name, dc.info)})
		}
		out, _, err := s.containerOps().ExecuteScriptInContainer(ctx, name, dc.script, "root")
		if err != nil {
			clog.Warn(fmt.Sprintf("failed to re-decorate container %s (%s)", name, dc.info), err)
		}
		_ = reportDecorationWarnings(report, name, out)
	}
}

// kDecorationWarningPrefix starts each line a decoration script prints when it
// skips a write (a linked or unowned ~/.config, rc file, ...); the script still
// exits 0.
const kDecorationWarningPrefix = "cds: "

// reportDecorationWarnings logs each skip warning in a decoration script's
// output and sends it to the client as a warning, so the user knows what was
// not written and why (CDS-061). A failed report is returned.
func reportDecorationWarnings(report deployProgressReporter, container, out string) error {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, kDecorationWarningPrefix) {
			continue
		}
		msg := fmt.Sprintf("%s: %s", container, strings.TrimPrefix(line, kDecorationWarningPrefix))
		clog.Warn(msg)
		if report == nil {
			continue
		}
		if err := report(deployProgress{phase: kProgressPhaseWarning, message: msg}); err != nil {
			return err
		}
	}
	return nil
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
