package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

// containerOps abstracts the container engine so that the gRPC layer can be
// tested without running podman/docker.
type containerOps interface {
	ListContainers() (bo.Containers, error)
	GetContainer(name string) (bo.Container, error)
	StartContainer(name string) error
	StopContainer(name string) error
	DeleteContainer(name string) error
	RenameContainer(name, newName string) error
	ExecuteInContainer(ctx context.Context, containerName, command, user string) (output string, exitCode int, err error)
	DeployContainer(ctx context.Context, spec deployContainerSpec, report deployProgressReporter) (deployContainerResult, error)
}

type deployResourceProvider interface {
	FetchFile(containerconf.ResourceType, string) ([]byte, error)
	FileExists(containerconf.ResourceType, string) bool
}

type deployContainerSpec struct {
	projectName      string
	containerName    string
	labels           map[string]string
	config           *containerconf.Config
	resourceProvider deployResourceProvider
	// orchestration is the resolved orchestration configuration of the
	// devcontainer. When Requested, the container joins the kind network and is
	// decorated with the cluster kubeconfig and registry trust.
	orchestration containerconf.KindOrchestrationConfig
	// kubeconfig is the cluster kubeconfig installed into the container when
	// orchestration is requested (resolved by the service before deploy).
	kubeconfig string
	// registryCA is the registry CA PEM installed into the container's trust
	// store when a registry is requested.
	registryCA string
}

type deployProgress struct {
	phase   string
	message string
}

type deployProgressReporter func(deployProgress) error

type deployContainerResult struct {
	containerName string
	output        string
}

// defaultContainerOps runs the container engine on the agent host.
type defaultContainerOps struct{}

var _ containerOps = defaultContainerOps{}

// ListContainers lists the containers managed by cds only.
func (defaultContainerOps) ListContainers() (bo.Containers, error) {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_PS)
	es.AddArg("--filter", fmt.Sprintf("label=%s=%s", engine.KCDSContainerBasicLabel, engine.KCDSContainerBasicLabelValue))
	es.SetFormat(engine.K_FCONTAINER_ID_STATUS)

	output, err := engine.ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return nil, cerr.AppendError("failed to list containers", err)
	}

	containers, err := engine.ParseContainersInfo(output, engine.K_FCONTAINER_ID_STATUS)
	if err != nil {
		return nil, cerr.AppendError("failed to parse containers info", err)
	}
	return containers, nil
}

func (defaultContainerOps) GetContainer(name string) (bo.Container, error) {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_INSPECT)
	es.SetContainerName(name)
	es.SetFormat(engine.K_FPORT)

	output, err := engine.ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return bo.Container{}, cerr.AppendErrorFmt("failed to inspect container %s", err, name)
	}

	container := bo.Container{Name: bo.ContainerName(name)}
	if err := engine.ParseContainerInfo(output, &container, engine.K_FPORT); err != nil {
		return bo.Container{}, cerr.AppendErrorFmt("failed to parse container info for %s", err, name)
	}

	engineStatus, err := engine.GetContainerStatus(name, cg.EmptyStr)
	if err != nil {
		return bo.Container{}, cerr.AppendErrorFmt("failed to get container status for %s", err, name)
	}
	container.Status = engineStatus
	return container, nil
}

// StartContainer starts a stopped container. The podman start is run in a
// transient systemd scope, like the initial run, so the helpers it respawns
// (conmon, rootlessport) do not land in cds.service and get killed on the next
// agent restart. Without this, a stop/start reintroduces the restart failure.
func (defaultContainerOps) StartContainer(name string) error {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_START)
	es.AddArg(name)

	cmds, err := es.BuildCommands()
	if err != nil {
		return cerr.AppendErrorFmt("failed to build start command for %s", err, name)
	}
	if len(cmds) != 1 {
		return cerr.NewError(fmt.Sprintf("expected 1 start command, got %d", len(cmds)))
	}
	if _, err := shexec.RunLocalCmdWithOutput([]shexec.ExecuteEvent{scopeWrap(cmds[0])}); err != nil {
		return cerr.AppendErrorFmt("failed to start container %s", err, name)
	}
	return nil
}

func (defaultContainerOps) StopContainer(name string) error {
	return engine.StopContainer(name)
}

func (defaultContainerOps) DeleteContainer(name string) error {
	return engine.DeleteContainer(name)
}

func (defaultContainerOps) RenameContainer(name, newName string) error {
	return engine.RenameContainer(name, newName)
}

// ExecuteInContainer runs command in the container and returns its output and
// exit code. A non-zero exit code is a command result, not an error. When ctx
// is cancelled (e.g. the RPC deadline is reached) the in-container process tree
// is terminated, not only the host-side podman client: the command is tagged
// with a unique token and, on cancellation, a `pkill` inside the container
// matches that token and kills the workload so it cannot complete its effect
// after the deadline.
func (defaultContainerOps) ExecuteInContainer(ctx context.Context, containerName, command, user string) (string, int, error) {
	token := execToken()
	tagged := execWrapper(token, command)

	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_EXE)
	es.SetExecuteCmd(engine.K_EXEC_CUSTOM_CMD)
	es.SetContainerName(containerName)
	es.SetCustomCommand(tagged)
	if user != cg.EmptyStr {
		es.SetRemoteUser(user)
	}

	cmds, err := es.BuildCommands()
	if err != nil {
		return cg.EmptyStr, -1, cerr.AppendError("failed to build execute command", err)
	}
	if len(cmds) != 1 {
		return cg.EmptyStr, -1, cerr.NewError(fmt.Sprintf("expected 1 execute command, got %d", len(cmds)))
	}

	// terminate the in-container process tree when ctx is cancelled
	stop := watchAndKillInContainer(ctx, containerName, token)
	defer stop()

	return shexec.RunLocalCmdWithExitCodeContext(ctx, cmds[0])
}

// execWrapper builds the shell snippet run inside the container for an
// execution: it runs the user command as a single quoted argument of a shell
// whose environment carries the token, so:
//   - the exact command (including compound/multiline forms) runs unchanged
//     and the shell waits for it, returning its real exit status and output;
//   - the token is inherited by the command and every child, surviving a shell
//     that replaces itself (`sh -c '...'`), so the cancellation killer can find
//     the whole tree via /proc/<pid>/environ.
//
// setsid is deliberately not used: util-linux setsid forks and returns
// immediately when the caller already leads its process group (as a
// `podman exec` process does), which would make every command report exit 0
// with no output, and busybox setsid lacks the --wait needed to avoid that.
func execWrapper(token, command string) string {
	return fmt.Sprintf("export CDS_EXEC_TOKEN=%s\nexec sh -c %s", token, cg.ShellSingleQuote(command))
}

// execToken returns a unique marker embedded in an execution's in-container
// command so the process tree can be found and killed on cancellation.
func execToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// a weaker but still unique-enough fallback; collisions only risk killing
		// the wrong cds exec, which is already an exceptional cancellation path
		return fmt.Sprintf("cds-exec-%d", time.Now().UnixNano())
	}
	return "cds-exec-" + hex.EncodeToString(b[:])
}

// watchAndKillInContainer starts a watcher that, when ctx is cancelled, kills
// the in-container process tree tagged with token (TERM then KILL). It returns
// a stop function that must be called once the command returns. Killing happens
// through a fresh engine exec that does not share the cancelled context.
func watchAndKillInContainer(ctx context.Context, containerName, token string) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		killInContainer(containerName, token)
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// killInContainer terminates every in-container process whose environment
// carries CDS_EXEC_TOKEN=<token> (the exec'd command and all its children),
// TERM first then KILL after a short grace period. It scans /proc/<pid>/environ
// with a plain shell loop rather than pkill (absent on slim images), matches on
// the NUL-delimited environment (not the command line), and skips its own
// process so the killer does not terminate itself. It uses its own bounded
// context since the caller's context is already cancelled.
func killInContainer(containerName, token string) {
	killCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	script := killByEnvTokenScript(token)

	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_EXE)
	es.SetExecuteCmd(engine.K_EXEC_CUSTOM_CMD)
	es.SetContainerName(containerName)
	es.SetRemoteUser(engine.KRootUsr)
	es.SetCustomCommand(script)
	cmds, err := es.BuildCommands()
	if err != nil || len(cmds) != 1 {
		clog.Warn(fmt.Sprintf("Failed to build in-container kill command for %s", containerName), err)
		return
	}
	if _, _, err := shexec.RunLocalCmdWithExitCodeContext(killCtx, cmds[0]); err != nil {
		clog.Warn(fmt.Sprintf("Failed to terminate in-container process for %s after cancellation", containerName), err)
	}
}

// killByEnvTokenScript returns a POSIX sh script that terminates every process
// whose environment contains CDS_EXEC_TOKEN=<token> (the command and, since the
// token is inherited, any shell it re-exec'd into and all its children),
// excluding the script's own process. It relies only on /proc and the shell, so
// it works on minimal images without pkill.
//
// It freezes the whole tree with SIGSTOP before killing: a plain TERM pass walks
// /proc in lexical order with a gap between kills, so if a child's pid sorts
// before its parent's the child is signalled first and the (still-alive) parent
// shell sees it exit and runs its next command before its own signal arrives.
// A stopped process cannot exit, so no parent can advance while the tree is
// frozen. The STOP scan repeats until it finds no new pids (children spawned
// between scans), then every match is KILLed.
func killByEnvTokenScript(token string) string {
	return fmt.Sprintf(`self=$$
match() {
	for d in /proc/[0-9]*; do
		pid=${d#/proc/}
		[ "$pid" = "$self" ] && continue
		if tr '\0' '\n' < "$d/environ" 2>/dev/null | grep -qx 'CDS_EXEC_TOKEN=%s'; then
			echo "$pid"
		fi
	done
}
# freeze the tree: STOP every match, repeating until no new pid appears, so a
# child cannot exit and let its parent run the next command while we work
prev=""
i=0
while [ "$i" -lt 100 ]; do
	cur=$(match)
	for p in $cur; do kill -STOP "$p" 2>/dev/null || true; done
	[ "$cur" = "$prev" ] && break
	prev="$cur"
	i=$((i + 1))
done
# now kill the frozen processes
for p in $(match); do kill -KILL "$p" 2>/dev/null || true; done
true`, token)
}

func (defaultContainerOps) DeployContainer(ctx context.Context, spec deployContainerSpec, report deployProgressReporter) (deployContainerResult, error) {
	result := deployContainerResult{containerName: spec.containerName}

	opts := []engine.ContainerEngineOption{
		engine.WithContainerConfig(spec.config),
		engine.WithResourceProvider(spec.resourceProvider),
	}
	if spec.orchestration.Requested {
		// attach the devcontainer to the kind cluster network so it can reach the
		// in-cluster API server and the registry.
		opts = append(opts, engine.WithNetwork(engine.KindNetworkName))
	}
	ce := engine.NewContainerEngine(opts...)
	ce.SetAction(engine.K_ACTION_RUN)
	ce.SetRunType(engine.K_RUN_DEV_CONTAINER)
	ce.SetContainerName(spec.containerName)
	ce.AddContainerLabels(spec.labels)

	cmds, err := ce.BuildCommands()
	if err != nil {
		return result, cerr.AppendError("failed to build deploy commands", err)
	}
	sshKeyCommands, err := buildSSHKeyInstallCommands(spec)
	if err != nil {
		return result, cerr.AppendError("failed to build SSH key install commands", err)
	}
	cmds = append(cmds, sshKeyCommands...)

	// decorate the devcontainer with cluster/registry access when orchestration
	// is requested (kubeconfig + registry trust installed inside the container).
	if spec.orchestration.Requested {
		cmds = append(cmds, buildDecorationCommands(spec)...)
	}

	// retain only a bounded tail of the combined command output; the full output
	// is streamed as progress, so the accumulator need not grow with the command
	output := &boundedTail{limit: kMaxDeployOutput}
	created := false
	// id of a container that already has this name before we try to create one,
	// so a create failure never removes a pre-existing container (only the one
	// this run actually created, which has a different id)
	preexistingID := containerIDByName(spec.containerName)
	for _, cmd := range cmds {
		if ctxErr := ctx.Err(); ctxErr != nil {
			result.output = boundedOutput(output.String())
			compensateFailedDeploy(created, spec.containerName)
			return result, cerr.AppendError("deployment cancelled", ctxErr)
		}
		if err := report(deployProgress{phase: "execute", message: deployCommandDescription(cmd)}); err != nil {
			result.output = boundedOutput(output.String())
			compensateFailedDeploy(created, spec.containerName)
			return result, err
		}

		stdout, runErr := shexec.RunLocalCmdWithOutputContext(ctx, []shexec.ExecuteEvent{detachContainerHelpers(cmd)})
		output.WriteString(stdout)
		// Establish ownership of a successfully created container immediately,
		// before anything that could fail (reporting its output, the next
		// command). Podman prints the new container id on the run step; if the
		// output send then failed we would otherwise leave the container running
		// without recording that this deployment created it, so compensation
		// could not clean it up.
		if runErr == nil && commandCreatesContainer(cmd) {
			created = true
		}
		if runErr != nil {
			result.output = boundedOutput(output.String())
			if commandCreatesContainer(cmd) {
				// Podman can fail after creating the container (e.g. a published
				// port already in use leaves it in Created state). Remove only the
				// container this run created — identified by a container id that
				// differs from any that already had this name — so a name
				// collision with a pre-existing container never deletes it.
				compensateOrphanedCreate(spec.containerName, preexistingID)
				return result, cerr.AppendError("failed to create container", runErr)
			}
			compensateFailedDeploy(created, spec.containerName)
			return result, cerr.AppendError("failed to execute deploy command", runErr)
		}
		// Stream the command output as progress rather than only returning it in
		// the final response: the response carries a bounded tail, while the full
		// output still reaches the client as it is produced.
		if reportErr := reportCommandOutput(report, stdout); reportErr != nil {
			result.output = boundedOutput(output.String())
			compensateFailedDeploy(created, spec.containerName)
			return result, reportErr
		}
	}

	result.output = boundedOutput(output.String())
	return result, nil
}

const (
	// kMaxDeployOutput bounds the output returned in a final response (deploy,
	// rebuild, execute, and error fields) so a verbose command cannot exceed the
	// client's gRPC receive limit (4 MiB by default) and make a successful result
	// look failed. The full deploy output is streamed as progress; only the
	// returned copy is bounded.
	kMaxDeployOutput = 256 * 1024
	// kMaxDeployOutputChunk bounds a single streamed output progress message.
	kMaxDeployOutputChunk = 64 * 1024
)

// boundedOutput returns at most the last kMaxDeployOutput bytes of output,
// prefixed with an explicit truncation marker when it had to drop content, so
// diagnostics are neither unbounded nor silently lost. The cut is moved to a
// UTF-8 boundary and the result is sanitized so it is always a valid protobuf
// string (invalid bytes, e.g. binary or Latin-1 output, become U+FFFD).
func boundedOutput(output string) string {
	if len(output) <= kMaxDeployOutput {
		return validUTF8(output)
	}
	tail := trimToValidUTF8Start(output[len(output)-kMaxDeployOutput:])
	return validUTF8(fmt.Sprintf("[... %d earlier bytes truncated ...]\n%s", len(output)-len(tail), tail))
}

// boundedHead returns at most kMaxDeployOutput bytes from the start of output,
// cut on a UTF-8 boundary and sanitized to valid UTF-8, with a trailing marker
// when truncated. It is used for execute and error payloads, where the start of
// the output is the most useful part.
func boundedHead(output string) string {
	if len(output) <= kMaxDeployOutput {
		return validUTF8(output)
	}
	head := trimToValidUTF8End(output[:kMaxDeployOutput])
	return validUTF8(fmt.Sprintf("%s\n[... %d later bytes truncated ...]", head, len(output)-len(head)))
}

// validUTF8 replaces invalid UTF-8 bytes with the replacement character so the
// string can be carried in a protobuf string field. Command output may be
// binary or non-UTF-8 (e.g. Latin-1), which would otherwise be rejected on send
// and lose the result (and, on the deploy progress path, trigger compensation).
func validUTF8(s string) string {
	return strings.ToValidUTF8(s, "\uFFFD")
}

// boundedTail accumulates written output but retains only the last limit bytes,
// so the deploy accumulator does not grow with a command's output (which is
// also streamed as progress). It tracks the total to report how much was
// dropped.
type boundedTail struct {
	limit int
	buf   []byte
	total int
}

func (b *boundedTail) WriteString(s string) {
	b.total += len(s)
	b.buf = append(b.buf, s...)
	if len(b.buf) > b.limit {
		b.buf = b.buf[len(b.buf)-b.limit:]
	}
}

func (b *boundedTail) String() string {
	if b.total <= len(b.buf) {
		return string(b.buf)
	}
	return fmt.Sprintf("[... %d earlier bytes truncated ...]\n%s", b.total-len(b.buf), trimToValidUTF8Start(string(b.buf)))
}

// trimToValidUTF8Start drops leading bytes that are a UTF-8 continuation so s
// starts on a rune boundary.
func trimToValidUTF8Start(s string) string {
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}

// trimToValidUTF8End drops a trailing partial rune so s ends on a rune boundary.
func trimToValidUTF8End(s string) string {
	for len(s) > 0 {
		if r, size := utf8.DecodeLastRuneInString(s); r == utf8.RuneError && size <= 1 {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

// reportCommandOutput streams non-empty command output as progress events,
// split into bounded chunks so no single event is unbounded. Each chunk is cut
// on a UTF-8 boundary and sanitized so it is a valid protobuf string.
func reportCommandOutput(report deployProgressReporter, stdout string) error {
	for len(stdout) > 0 {
		chunk := stdout
		if len(chunk) > kMaxDeployOutputChunk {
			chunk = trimToValidUTF8End(stdout[:kMaxDeployOutputChunk])
			// a single rune larger than the chunk budget should still make progress
			if chunk == cg.EmptyStr {
				chunk = stdout[:kMaxDeployOutputChunk]
			}
		}
		stdout = stdout[len(chunk):]
		if err := report(deployProgress{phase: "output", message: validUTF8(chunk)}); err != nil {
			return err
		}
	}
	return nil
}

// commandCreatesContainer reports whether running cmd brings the container into
// existence, i.e. the engine `run` step.
func commandCreatesContainer(cmd shexec.ExecuteEvent) bool {
	creator, ok := cmd.(interface{ CreatesContainer() bool })
	return ok && creator.CreatesContainer()
}

// detachContainerHelpers wraps the container-creating command so that the
// container runtime's long-lived helpers (conmon, rootlessport) start in their
// own transient systemd scope instead of the agent's service cgroup. Without
// this, restarting or stopping cds.service (e.g. during an agent upgrade) kills
// those helpers under the default KillMode=control-group, leaving a running
// container with no host port listener. The agent's lifetime is thus decoupled
// from the containers it deploys.
//
// Only the creating command is wrapped, and only when the agent itself runs
// under systemd (INVOCATION_ID is set for unit processes) and systemd-run is
// available. Otherwise the command is returned unchanged.
func detachContainerHelpers(cmd shexec.ExecuteEvent) shexec.ExecuteEvent {
	if !commandCreatesContainer(cmd) {
		return cmd
	}
	wrapped, changed := transientScopeCommand(cmd.Cmd())
	if !changed {
		return cmd
	}
	return &shexec.DefaultShEvent{ExeCmd: wrapped, DescriptionCmd: cmd.Description()}
}

// scopeWrap returns cmd wrapped to run in a transient systemd scope, for any
// command that spawns a container's long-lived helpers (e.g. `podman start`).
// Unlike detachContainerHelpers it does not require the CreatesContainer marker.
func scopeWrap(cmd shexec.ExecuteEvent) shexec.ExecuteEvent {
	wrapped, changed := transientScopeCommand(cmd.Cmd())
	if !changed {
		return cmd
	}
	return &shexec.DefaultShEvent{ExeCmd: wrapped, DescriptionCmd: cmd.Description()}
}

// transientScopeCommand wraps command so it runs in its own transient systemd
// user scope (a sibling of cds.service), detaching any long-lived container
// helpers it spawns from the agent's service cgroup. It returns the command
// unchanged (changed=false) when the agent is not under systemd or systemd-run
// is unavailable. This must wrap every operation that (re)spawns a container's
// helpers — both `podman run` (deploy) and `podman start` — otherwise a
// stop/start would place fresh helpers back in cds.service and an agent restart
// would kill them again.
func transientScopeCommand(command string) (string, bool) {
	if !runningUnderSystemd() || !systemdRunAvailable() {
		return command, false
	}
	// --scope runs the command in a transient scope unit; --collect reclaims it
	// once it exits. The scope is a sibling of cds.service, so a restart of the
	// service no longer tears down the container helpers placed in this scope.
	return fmt.Sprintf("systemd-run --user --scope --collect --quiet -- bash -c %s", cg.ShellSingleQuote(command)), true
}

// runningUnderSystemd reports whether this process runs inside a systemd unit.
// systemd exports INVOCATION_ID to the processes of the units it starts.
func runningUnderSystemd() bool {
	return os.Getenv("INVOCATION_ID") != cg.EmptyStr
}

func systemdRunAvailable() bool {
	_, err := exec.LookPath("systemd-run")
	return err == nil
}

// compensateFailedDeploy removes a container created earlier in a deployment
// that then failed, so that a partial deploy does not leave an unmanaged
// container behind. It is best effort: a container the client never recorded
// could otherwise only be removed by hand. Nothing is removed when the
// container was not created by this deployment.
func compensateFailedDeploy(created bool, containerName string) {
	if !created {
		return
	}
	if err := engine.DeleteContainer(containerName); err != nil {
		clog.Warn(fmt.Sprintf("Failed to remove container %s after a failed deployment, it may need manual cleanup", containerName), err)
	}
}

// containerIDByName returns the id of the container named name, or "" when none
// exists (or it cannot be determined). Used to tell a container this deployment
// created apart from a pre-existing one with the same name. It inspects by id
// directly so a missing container is simply an empty id, not an error.
func containerIDByName(name string) string {
	es := engine.NewContainerEngine()
	es.SetAction(engine.K_ACTION_INSPECT)
	es.SetContainerName(name)
	es.SetFormat(engine.K_FID)
	out, err := engine.ExecuteCommand(es, shexec.RunLocalCmdWithOutput)
	if err != nil {
		return cg.EmptyStr
	}
	return strings.TrimSpace(out)
}

// compensateOrphanedCreate removes a container left behind when `podman run`
// failed after creating it (e.g. a published port already in use leaves it in
// Created state). It removes the container only when the one now bearing the
// name is NOT the one that already existed before this run (different id), so a
// name collision with a pre-existing container never deletes it or its data.
func compensateOrphanedCreate(containerName, preexistingID string) {
	currentID := containerIDByName(containerName)
	if currentID == cg.EmptyStr || currentID == preexistingID {
		// nothing new was created (or a pre-existing container still owns the
		// name): do not remove it
		return
	}
	if err := engine.DeleteContainer(containerName); err != nil {
		clog.Warn(fmt.Sprintf("Failed to remove orphaned container %s after a failed create, it may need manual cleanup", containerName), err)
	}
}

// buildSSHKeyInstallCommands installs the staged public key, if any, for the
// remote user of the container.
// buildDecorationCommands builds the exec commands that decorate a running
// devcontainer with cluster/registry access: the cluster kubeconfig and the
// registry CA are installed inside the container. Each decoration step runs as
// root in the container via a custom command.
func buildDecorationCommands(spec deployContainerSpec) []shexec.ExecuteEvent {
	var events []shexec.ExecuteEvent
	for _, dc := range containerDecorationCommands(cg.EmptyStr, spec.containerName, spec.kubeconfig, spec.registryCA) {
		ce := engine.NewContainerEngine(engine.WithContainerConfig(spec.config))
		ce.SetAction(engine.K_ACTION_EXE)
		ce.SetExecuteCmd(engine.K_EXEC_CUSTOM_CMD)
		ce.SetContainerName(spec.containerName)
		ce.SetRemoteUser(engine.KRootUsr)
		ce.SetCustomCommand(dc.script)
		cmds, err := ce.BuildCommands()
		if err != nil {
			clog.Warn("failed to build container decoration command", err)
			continue
		}
		events = append(events, cmds...)
	}
	return events
}

func buildSSHKeyInstallCommands(spec deployContainerSpec) ([]shexec.ExecuteEvent, error) {
	identifier, err := containerconf.SingletonIdentifier(containerconf.KindPubKey)
	if err != nil {
		return nil, err
	}
	if spec.resourceProvider == nil || !spec.resourceProvider.FileExists(containerconf.ResourceTypeFile, identifier) {
		return nil, nil
	}

	opts := []engine.ContainerEngineOption{
		engine.WithContainerConfig(spec.config),
		engine.WithResourceProvider(spec.resourceProvider),
	}
	if spec.orchestration.Requested {
		// attach the devcontainer to the kind cluster network so it can reach the
		// in-cluster API server and the registry.
		opts = append(opts, engine.WithNetwork(engine.KindNetworkName))
	}
	ce := engine.NewContainerEngine(opts...)
	ce.SetAction(engine.K_ACTION_EXE)
	ce.SetExecuteCmd(engine.K_EXEC_CMD_SSH)
	ce.SetContainerName(spec.containerName)
	ce.SetRemoteUser(engine.ResolveRemoteUserFromConfig(spec.config))
	return ce.BuildCommands()
}

func deployCommandDescription(cmd shexec.ExecuteEvent) string {
	if description := strings.TrimSpace(cmd.Description()); description != cg.EmptyStr {
		return description
	}
	return "Running deploy command"
}
