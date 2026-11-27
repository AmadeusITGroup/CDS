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
		// fail before building anything: only a secured registry exists
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
// The registry always listens on registry.DefaultPort.
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
	// only that user can read it.
	owner string
}

func (r registryEnv) set() bool { return r.name != "" && r.user != "" }

// newRegistryEnv is the registry env for a deployment, written for the
// container user owner (empty means root). The registry always listens on
// registry.DefaultPort. It is unset when user is empty.
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
// is installed at root's and the container user's default kubeconfig path, the
// registry CA in the container's per-registry trust directory, and the
// CDS_REGISTRY_* env in a 0600 file in the container user's home.
//
// Root writes only root-owned paths (/root/.kube, /etc/containers/certs.d,
// /etc/profile.d). Every write into the container user's home runs AS that
// user: root following a link the user planted in a dir the user owns would
// let the user take any root file.
//
// kubeconfig is the cluster kubeconfig, caCert the registry CA PEM and owner
// the container user (remoteUser, empty means root); each step is skipped when
// its material is empty (or reg is unset).
func containerDecorationCommands(kubeconfig, caCert, owner string, reg registryEnv) []decorationCommand {
	var cmds []decorationCommand
	if kubeconfig != "" {
		cmds = append(cmds, decorationCommand{
			info:   "Installing cluster kubeconfig in container",
			script: installKubeconfigScript(kubeconfig),
		})
		if !isRootUser(owner) {
			cmds = append(cmds, decorationCommand{
				info:   "Installing cluster kubeconfig for the container user",
				script: installUserKubeconfigScript(kubeconfig, owner),
				user:   owner,
			})
		}
	}
	if caCert != "" {
		cmds = append(cmds, decorationCommand{
			info:   "Installing registry trust in container",
			script: installRegistryTrustScript(caCert),
		})
	}
	if reg.set() {
		cmds = append(cmds, decorationCommand{
			info:   "Removing legacy registry environment from container",
			script: kRemoveLegacyRegistryEnvScript,
		}, decorationCommand{
			info:   "Exporting registry environment in container",
			script: installRegistryEnvScript(reg),
			user:   userOrRoot(reg.owner),
		})
	}
	return cmds
}

// isRootUser reports whether the container user owner is root (empty means root).
func isRootUser(owner string) bool {
	return owner == cg.EmptyStr || owner == engine.KRootUsr
}

// userOrRoot is owner, or root when owner is empty.
func userOrRoot(owner string) string {
	if owner == cg.EmptyStr {
		return engine.KRootUsr
	}
	return owner
}

// kRemoveLegacyRegistryEnvScript removes the world-readable /etc/profile.d
// script that earlier versions wrote the registry password to.
const kRemoveLegacyRegistryEnvScript = `rm -f /etc/profile.d/cds-registry.sh`

// registryEnvFile is the env file, relative to the container user's home, that
// holds the CDS_REGISTRY_* variables.
const registryEnvFile = ".config/cds/registry.env"

// installRegistryEnvScript writes the CDS_REGISTRY_* variables to a 0600 env
// file in the container user's home and sources it from the user's shell rc
// files, so the user's shells can podman login and push to the local registry
// while other container users cannot read the password.
//
// The script runs AS the container user, so a link or a file the
// user does not own in the home can only make a write fail, never redirect
// it: a dir or env file it cannot write is skipped with a warning, and so is
// an rc file it cannot append to. The "cds: " warnings go to stdout: a
// successful first-deploy step keeps only stdout, and the agent sends those
// lines to the client.
func installRegistryEnvScript(reg registryEnv) string {
	return fmt.Sprintf(`set -eu
%s
umask 077
d="$home/.config/cds"
if ! mkdir -p "$d" 2>/dev/null || ! tmp=$(mktemp "$d/.registry.env.XXXXXX" 2>/dev/null); then
  echo "cds: cannot write to $d, $home/%s not written"; exit 0
fi
cat > "$tmp" <<'CDS_EOF'
export CDS_REGISTRY_NAME=%s
export CDS_REGISTRY_PORT=%d
export CDS_REGISTRY_PATH_TO_CA=%s
export CDS_REGISTRY_USR=%s
export CDS_REGISTRY_PWD=%s
CDS_EOF
mv -f "$tmp" "$home/%s"
line='if [ -r "$HOME/%s" ]; then . "$HOME/%s"; fi # cds registry env'
for rc in .profile .bashrc .bash_profile .zshrc; do
  f="$home/$rc"
  # .profile is created when missing (sh/dash/bash login shells read it);
  # the others are only appended to when the user already has them.
  if [ "$rc" != .profile ] && [ ! -e "$f" ]; then continue; fi
  if [ -e "$f" ] && grep -qF "$line" "$f" 2>/dev/null; then continue; fi
  if ! { printf '%%s\n' "$line" >> "$f"; } 2>/dev/null; then
    echo "cds: cannot write to $f, not changed; add to it: $line"
  fi
done`,
		containerUserPrelude(reg.owner),
		registryEnvFile,
		reg.name, reg.port, reg.caPath, reg.user, reg.password,
		registryEnvFile, registryEnvFile, registryEnvFile)
}

// decorationCommand is a shell snippet run inside the container, as user
// (empty means root), with a human-readable description.
type decorationCommand struct {
	info   string
	script string
	user   string
}

// runAs is the container user the command runs as.
func (dc decorationCommand) runAs() string { return userOrRoot(dc.user) }

// containerUserPrelude is the shell prelude of a decoration script that runs
// as container user owner and writes into its home: it sets u and home and
// exits 1 when the user has no home. The home comes from the passwd entry,
// not $HOME, which an image ENV can override.
func containerUserPrelude(owner string) string {
	return fmt.Sprintf(`u=%s
home=$(getent passwd "$u" 2>/dev/null | cut -d: -f6) || home=
[ -n "$home" ] || home=$(awk -F: -v u="$u" '$1 == u { print $6 }' /etc/passwd)
[ -n "$home" ] || { echo "container user $u has no home directory" >&2; exit 1; }`, cg.ShellSingleQuote(userOrRoot(owner)))
}

// installKubeconfigScript writes the cluster kubeconfig to root's default
// path. It runs as root.
func installKubeconfigScript(kubeconfig string) string {
	// a here-doc keeps the content intact regardless of quoting.
	return fmt.Sprintf(`set -eu
mkdir -p /root/.kube
cat > /root/.kube/config <<'CDS_EOF'
%s
CDS_EOF
chmod 600 /root/.kube/config`, kubeconfig)
}

// installUserKubeconfigScript writes the cluster kubeconfig to the ~/.kube/config
// (600) of the non-root container user owner (remoteUser), so the shell
// `cds project ssh` opens can run kubectl and helm. It runs AS that
// user: a temp file renamed over the target, so a reader never sees
// a partial file; a ~/.kube it cannot write is skipped with a warning.
func installUserKubeconfigScript(kubeconfig, owner string) string {
	return "set -eu\n" + containerUserPrelude(owner) + fmt.Sprintf(`
umask 077
d="$home/.kube"
if ! mkdir -p "$d" 2>/dev/null || ! tmp=$(mktemp "$d/.config.XXXXXX" 2>/dev/null); then
  echo "cds: cannot write to $d, $d/config not written"; exit 0
fi
cat > "$tmp" <<'CDS_EOF'
%s
CDS_EOF
mv -f "$tmp" "$d/config"`, kubeconfig)
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
	// apply the mirrors of every recorded deployment, not only this one, so the
	// prune below keeps theirs
	oc := spec.orchestration
	others, known := s.core.otherDeployments(spec.containerName)
	mirrorSets := [][]containerconf.Mirror{oc.Mirrors}
	for _, d := range others {
		mirrorSets = append(mirrorSets, d.oc.Mirrors)
	}
	var conflicts []string
	oc.Mirrors, conflicts = mergeMirrors(mirrorSets...)
	warnMirrorConflicts(report, conflicts)
	created := s.core.clusterAbsent()
	if err := ensureOrchestration(ctx, s.core.clusterOps(), s.core.registryOps(), oc, report); err != nil {
		return spec, err
	}
	if created {
		s.core.restoreOtherDeployments(ctx, oc, others, report)
	}
	if known {
		// a stale mirror is not worth failing the deploy for
		if err := s.core.clusterOps().PruneMirrors(ctx, toOrchestrationMirrors(oc.Mirrors)); err != nil {
			msg := fmt.Sprintf("failed to remove unused registry mirrors: %v", err)
			clog.Warn(msg)
			_ = report(deployProgress{phase: kProgressPhaseWarning, message: msg})
		}
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
			} else {
				clog.Warn("failed to resolve registry CA for re-decoration", err)
			}
		}

		// a rebuilt registry has new credentials: refresh CDS_REGISTRY_* too
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
		s.redecorateOne(ctx, d.name, containerDecorationCommands(kubeconfig, ca, d.remoteUser, reg), report)
	}
}

// orchestratedDeployment is a recorded deployment that requested orchestration.
type orchestratedDeployment struct {
	name string
	oc   containerconf.KindOrchestrationConfig
	// remoteUser is the container user the kubeconfig and registry env are written for.
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

// otherDeployments returns every recorded orchestrated deployment but name; ok
// is false when the record cannot be read.
func (s *grpcServer) otherDeployments(name string) (others []orchestratedDeployment, ok bool) {
	deployed, ok := s.orchestratedDeployments()
	for _, d := range deployed {
		if d.name != name {
			others = append(others, d)
		}
	}
	return others, ok
}

// clusterAbsent reports whether the cluster does not exist yet, so a build
// creates it. A status that cannot be read counts as absent: restoring the
// other deployments is best effort and safe to repeat.
func (s *grpcServer) clusterAbsent() bool {
	c, err := s.clusterOps().GetCluster()
	return err != nil || c.Status == bo.KContainerStatusDeleted
}

// restoreOtherDeployments gives a cluster a deploy just created what the other
// recorded deployments need, as BuildCluster does: their ingress and
// registry access, then a re-decoration of their running containers, which
// hold the old cluster's kubeconfig. Their mirrors are already merged into
// oc.Mirrors. Best effort: a failed step is reported, the deploy goes on.
func (s *grpcServer) restoreOtherDeployments(ctx context.Context, oc containerconf.KindOrchestrationConfig, others []orchestratedDeployment, report deployProgressReporter) {
	if len(others) == 0 {
		return
	}
	ingress, registryRequested := false, false
	for _, d := range others {
		ingress = ingress || d.oc.Ingress
		registryRequested = registryRequested || d.oc.Registry.Requested
	}
	if ingress && !oc.Ingress {
		warnReapply(report, deployIngress(ctx, s.clusterOps(), report))
	}
	if registryRequested && !oc.Registry.Requested {
		warnReapply(report, connectRegistryToCluster(ctx, s.clusterOps(), s.registryOps(), report))
	}
	s.redecorateContainers(ctx, report)
}

// warnReapply logs and reports a failed re-apply step as a warning; nil is a no-op.
func warnReapply(report deployProgressReporter, err error) {
	if err == nil {
		return
	}
	clog.Warn("failed to re-apply orchestration setting", err)
	_ = report(deployProgress{phase: kProgressPhaseWarning, message: fmt.Sprintf("failed to re-apply orchestration setting: %v", err)})
}

// warnMirrorConflicts logs and reports each resolved mirror conflict as a
// warning: the first mirror was applied, so nothing failed.
func warnMirrorConflicts(report deployProgressReporter, conflicts []string) {
	for _, c := range conflicts {
		clog.Warn(c)
		_ = report(deployProgress{phase: kProgressPhaseWarning, message: c})
	}
}

// mergeMirrors merges mirror sets into one mirror per registry. When two sets
// map a registry to different endpoints, the first one wins and a conflict
// message is returned for the other.
func mergeMirrors(sets ...[]containerconf.Mirror) (merged []containerconf.Mirror, conflicts []string) {
	endpoint := map[string]string{}
	for _, set := range sets {
		for _, m := range set {
			kept, seen := endpoint[m.Registry]
			if !seen {
				endpoint[m.Registry] = m.Endpoint
				merged = append(merged, m)
			} else if kept != m.Endpoint {
				conflicts = append(conflicts, fmt.Sprintf("registry mirror conflict for %s: using %s, ignoring %s set by another deployment", m.Registry, kept, m.Endpoint))
			}
		}
	}
	return merged, conflicts
}

// reapplyOrchestration re-runs the post-build steps of ensureOrchestration for
// every orchestration setting found in the recorded deployments: a
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

	var mirrorSets [][]containerconf.Mirror
	ingress, registryRequested := false, false
	for _, d := range deployed {
		mirrorSets = append(mirrorSets, d.oc.Mirrors)
		ingress = ingress || d.oc.Ingress
		registryRequested = registryRequested || d.oc.Registry.Requested
	}
	mirrors, conflicts := mergeMirrors(mirrorSets...)

	warn := func(err error) { warnReapply(report, err) }
	if clusterRebuilt {
		warnMirrorConflicts(report, conflicts)
		warn(applyMirrors(ctx, s.clusterOps(), mirrors, report))
		if ingress {
			warn(deployIngress(ctx, s.clusterOps(), report))
		}
	}
	if registryRequested {
		warn(connectRegistryToCluster(ctx, s.clusterOps(), s.registryOps(), report))
	}
}

// redecorateOne runs the decoration install scripts inside a single container,
// each as its user (root or the container user) and fed on stdin (they carry the kubeconfig and registry secrets).
func (s *grpcServer) redecorateOne(ctx context.Context, name string, cmds []decorationCommand, report deployProgressReporter) {
	for _, dc := range cmds {
		if report != nil {
			_ = report(deployProgress{phase: "decorate", message: fmt.Sprintf("%s: %s", name, dc.info)})
		}
		out, code, err := s.containerOps().ExecuteScriptInContainer(ctx, name, dc.script, dc.runAs())
		if err != nil {
			clog.Warn(fmt.Sprintf("failed to re-decorate container %s (%s)", name, dc.info), err)
		} else if code != 0 {
			// a non-zero exit (set -eu) is not an error from exec: log and report it
			msg := fmt.Sprintf("%s: %s failed (exit %d)", name, dc.info, code)
			if tail := decorationFailureTail(out); tail != "" {
				msg += ": " + tail
			}
			clog.Warn(msg)
			if report != nil {
				_ = report(deployProgress{phase: kProgressPhaseWarning, message: msg})
			}
		}
		_ = reportDecorationWarnings(report, name, out)
	}
}

// kMaxDecorationTail bounds the script output carried in a failure warning.
const kMaxDecorationTail = 512

// decorationFailureTail returns the trimmed last kMaxDecorationTail bytes of a
// failed decoration script's output, cut on a UTF-8 boundary.
func decorationFailureTail(out string) string {
	out = strings.TrimSpace(out)
	if len(out) <= kMaxDecorationTail {
		return validUTF8(out)
	}
	return "..." + validUTF8(trimToValidUTF8Start(out[len(out)-kMaxDecorationTail:]))
}

// kDecorationWarningPrefix starts each line a decoration script prints when it
// skips a write (a linked or unowned ~/.config, rc file, ...); the script still
// exits 0.
const kDecorationWarningPrefix = "cds: "

// reportDecorationWarnings logs each skip warning in a decoration script's
// output and sends it to the client as a warning, so the user knows what was
// not written and why. A failed report is returned.
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
