package bootstrap

import (
	"crypto/sha256"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/host"
	"github.com/amadeusitgroup/cds/internal/systemd"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
)

const (
	kRemoteAgentDir    = ".xcds-agent"
	kRemoteUploadExt   = ".upload"
	kRemoteDirPerm     = "700"
	kRemoteBinaryPerm  = "700"
	kRemoteSecretsPerm = "600"
)

// remoteHost is a host reachable through SSH, see internal/host.
type remoteHost interface {
	Execute(name string, args ...string) (string, error)
	Copy(localPath, remotePath string) error
}

// remoteBootstrap installs and starts an agent on a remote host, in the
// spirit of the VS Code server: the agent binary matching the CLI version is
// uploaded under ~/.xcds-agent/bin/<version> when missing or different, along
// with a server certificate issued for the host, then run as a systemd user
// service.
type remoteBootstrap struct {
	hostname      string
	h             remoteHost
	port          int
	resolveBinary func(platform) (string, error)
	agentCertsDir func(hostname string) (string, error)
	startService  func(h remoteHost, binary string, port int) error
	waitHealthy   func(address, serverName string) error
	register      func(hostname, address string) error
}

func fireRemote(hostname string) error {
	h, err := prepareRemoteHost(hostname)
	if err != nil {
		return err
	}
	return remoteBootstrap{
		hostname:      hostname,
		h:             h,
		port:          config.AgentPort(),
		resolveBinary: resolveAgentBinary,
		agentCertsDir: cdstls.AgentCertsDir,
		startService:  startRemoteService,
		waitHealthy:   waitForHealthyAgent,
		register:      registerAgent,
	}.run()
}

// prepareRemoteHost returns the host with its SSH access, configuring it the
// first time (key pair generation and installation on the host).
func prepareRemoteHost(hostname string) (remoteHost, error) {
	h, found := host.Load(hostname)
	if found {
		return h, nil
	}
	clog.Info(fmt.Sprintf("Configuring SSH access to %s", hostname))
	if err := h.Build(); err != nil {
		return nil, cerr.AppendErrorFmt("Failed to configure host %s", err, hostname)
	}
	return h, nil
}

func (rb remoteBootstrap) run() error {
	p, err := rb.probePlatform()
	if err != nil {
		return err
	}
	home, err := rb.homeDir()
	if err != nil {
		return err
	}
	agentDir := path.Join(home, kRemoteAgentDir)
	if err := rb.ensureDir(agentDir); err != nil {
		return err
	}

	binary, err := rb.ensureBinary(p, path.Join(agentDir, "bin", cg.Version))
	if err != nil {
		return err
	}
	if err := rb.ensureCerts(path.Join(agentDir, "certs")); err != nil {
		return err
	}
	if err := rb.startService(rb.h, binary, rb.port); err != nil {
		return cerr.AppendErrorFmt("Failed to start agent on %s", err, rb.hostname)
	}

	address := agentAddress(rb.hostname, rb.port)
	if err := rb.waitHealthy(address, rb.hostname); err != nil {
		return err
	}
	return rb.register(rb.hostname, address)
}

// probePlatform maps `uname -sm` of the host to GOOS/GOARCH.
func (rb remoteBootstrap) probePlatform() (platform, error) {
	out, err := rb.h.Execute("uname", "-sm")
	if err != nil {
		return platform{}, cerr.AppendErrorFmt("Failed to probe platform of %s", err, rb.hostname)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return platform{}, cerr.NewError(fmt.Sprintf("unexpected platform %q for %s", strings.TrimSpace(out), rb.hostname))
	}

	p := platform{os: strings.ToLower(fields[0])}
	switch fields[1] {
	case "x86_64", "amd64":
		p.arch = "amd64"
	case "aarch64", "arm64":
		p.arch = "arm64"
	default:
		return platform{}, cerr.NewError(fmt.Sprintf("unsupported architecture %q for %s", fields[1], rb.hostname))
	}
	if p.os != cg.LINUX {
		return platform{}, cerr.NewError(fmt.Sprintf("unsupported OS %q for %s, remote agents run on linux", fields[0], rb.hostname))
	}
	return p, nil
}

// homeDir returns the home directory of the user on the host.
func (rb remoteBootstrap) homeDir() (string, error) {
	out, err := rb.h.Execute("systemd-path", "user")
	if err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("Failed to resolve home directory on %s", err, rb.hostname)
	}
	home := strings.TrimSpace(out)
	if !path.IsAbs(home) {
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("unexpected home directory %q on %s", home, rb.hostname))
	}
	return home, nil
}

func (rb remoteBootstrap) ensureDir(dir string) error {
	if _, err := rb.h.Execute("mkdir", "-p", dir); err != nil {
		return cerr.AppendErrorFmt("Failed to create %s on %s", err, dir, rb.hostname)
	}
	if _, err := rb.h.Execute("chmod", kRemoteDirPerm, dir); err != nil {
		return cerr.AppendErrorFmt("Failed to restrict access to %s on %s", err, dir, rb.hostname)
	}
	return nil
}

// ensureBinary makes sure binDir holds the agent binary resolved for p and
// returns its path. The binary is uploaded only when missing or different.
func (rb remoteBootstrap) ensureBinary(p platform, binDir string) (string, error) {
	local, err := rb.resolveBinary(p)
	if err != nil {
		return cg.EmptyStr, err
	}
	localSum, err := sha256File(local)
	if err != nil {
		return cg.EmptyStr, err
	}

	remote := path.Join(binDir, kAgentBinaryName)
	if out, err := rb.h.Execute("sh", "-c", sha256Probe(remote)); err == nil && strings.HasPrefix(strings.TrimSpace(out), localSum+" ") {
		clog.Debug(fmt.Sprintf("Agent binary %s is up to date on %s", remote, rb.hostname))
		return remote, nil
	}

	clog.Info(fmt.Sprintf("Installing %s %s on %s", kAgentBinaryName, cg.Version, rb.hostname))
	if err := rb.ensureDir(binDir); err != nil {
		return cg.EmptyStr, err
	}
	// upload aside then rename, the binary may be in use by a running agent
	if err := rb.upload(local, remote+kRemoteUploadExt, kRemoteBinaryPerm); err != nil {
		return cg.EmptyStr, err
	}
	if _, err := rb.h.Execute("mv", "-f", remote+kRemoteUploadExt, remote); err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("Failed to install %s on %s", err, remote, rb.hostname)
	}
	return remote, nil
}

// ensureCerts installs the CA and the server certificate of the host in certsDir.
func (rb remoteBootstrap) ensureCerts(certsDir string) error {
	localDir, err := rb.agentCertsDir(rb.hostname)
	if err != nil {
		return err
	}
	if err := rb.ensureDir(certsDir); err != nil {
		return err
	}
	for _, file := range []string{"ca.pem", "agent-srv.pem", "agent-srv-key.pem"} {
		if err := rb.upload(filepath.Join(localDir, file), path.Join(certsDir, file), kRemoteSecretsPerm); err != nil {
			return err
		}
	}
	return nil
}

// upload copies the local file to the remote path with the given permissions.
func (rb remoteBootstrap) upload(local, remote, perm string) error {
	// the remote file name is the one of the copied file
	stagingDir, err := cos.CreateTempDir(cg.EmptyStr, "cds-upload")
	if err != nil {
		return cerr.AppendError("Failed to create upload staging directory", err)
	}
	defer func() {
		_ = cos.Fs.RemoveAll(stagingDir)
	}()
	staged := filepath.Join(stagingDir, path.Base(remote))
	if err := cenv.CopyFile(local, staged); err != nil {
		return cerr.AppendErrorFmt("Failed to stage %s for upload", err, local)
	}

	if err := rb.h.Copy(staged, path.Dir(remote)); err != nil {
		return cerr.AppendErrorFmt("Failed to copy %s to %s", err, local, rb.hostname)
	}
	if _, err := rb.h.Execute("chmod", perm, remote); err != nil {
		return cerr.AppendErrorFmt("Failed to set permissions of %s on %s", err, remote, rb.hostname)
	}
	return nil
}

// sha256Probe returns the sh script printing the sha256 of file, or nothing
// when the file is missing: the probe does not fail.
func sha256Probe(file string) string {
	return cg.ShellSingleQuote(fmt.Sprintf("sha256sum %s 2>/dev/null || true", cg.ShellSingleQuote(file)))
}

// startRemoteService runs the agent as a systemd user service of the host. The
// units are reconciled with the installed binary and port; the service is
// restarted only when the units changed, to pick up an updated binary without
// needlessly disrupting a running service and its containers.
func startRemoteService(h remoteHost, binary string, port int) error {
	sysd := systemd.New(systemd.WithTarget(h), systemd.WithServicePort(port), systemd.WithServiceBinary(binary))
	if !sysd.In() {
		return cerr.NewError("a systemd user session is required to run the agent, consider `loginctl enable-linger`")
	}
	changed, err := sysd.StartService()
	if err != nil {
		return err
	}
	if changed {
		return sysd.RestartService()
	}
	return nil
}

func sha256File(file string) (string, error) {
	f, err := cos.Fs.Open(file)
	if err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("Failed to open %s", err, file)
	}
	defer func() {
		_ = f.Close()
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("Failed to read %s", err, file)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
