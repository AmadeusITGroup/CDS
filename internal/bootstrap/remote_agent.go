package bootstrap

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
)

const (
	remoteAgentPort       = "8087"
	remoteAgentBinaryPath = "~/.local/bin/cds-api-agent"
	remoteAgentCertDir    = "~/.xcds-agent/certs"
	remoteAgentLogPath    = "~/.xcds-agent/agent.log"
)

func startRemoteAgent(hostName string) error {
	address, err := ensureRemoteAgentRegistered(hostName)
	if err != nil {
		return err
	}
	if err := ensureRuntimeCerts(); err != nil {
		return err
	}

	agentBinaryPath, cleanup, err := prepareRemoteAgentBinary(hostName)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := copyRemoteAgentAssets(hostName, agentBinaryPath); err != nil {
		return err
	}
	port, err := agentPort(address)
	if err != nil {
		return err
	}
	if err := stopRemoteAgentProcess(hostName, port); err != nil {
		return err
	}
	if err := startRemoteAgentProcess(hostName, port); err != nil {
		return err
	}

	tcpAddress, err := agentTCPAddress(address)
	if err != nil {
		return err
	}
	if err := waitForHealthyAgent(address); err != nil {
		return cerr.AppendErrorFmt("remote agent did not start on %s; check %s on the remote host", err, tcpAddress, remoteAgentLogPath)
	}
	return nil
}

func ensureRemoteAgentRegistered(hostName string) (string, error) {
	if address, found, err := config.AgentAddressIfRegistered(hostName); err != nil {
		return cg.EmptyStr, err
	} else if found {
		return address, nil
	}

	address := net.JoinHostPort(hostName, remoteAgentPort)
	return address, config.UpsertAgentForHostInConfig(hostName, config.NewAgent(
		config.WithTargetAddress(address),
		config.WithAgentTLS(config.NewTlssecret(
			config.WithCA(cdstls.CAFilePath()),
			config.WithCert(cdstls.ClientCertFilePath()),
			config.WithKey(cdstls.ClientKeyFilePath()),
		)),
	))
}

func ensureRuntimeCerts() error {
	for _, certPath := range requiredRuntimeCerts() {
		if _, err := os.Stat(certPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return cerr.AppendError(fmt.Sprintf("Failed to inspect certificate %s", certPath), err)
		}

		for _, bin := range localAgentRequiredBinaries {
			if err := ensureBinary(bin); err != nil {
				return cerr.AppendErrorFmt("Failed to install binary %s", err, bin.name())
			}
		}
		if err := cdstls.BuildCerts(); err != nil {
			return cerr.AppendError("Failed to build certs", err)
		}
		return nil
	}
	return nil
}

func requiredRuntimeCerts() []string {
	clientCertsDir := cenv.ClientConfigDir("certs")
	agentCertsDir := cenv.AgentConfigDir("certs")
	return []string{
		filepath.Join(clientCertsDir, "ca.pem"),
		filepath.Join(clientCertsDir, "client.pem"),
		filepath.Join(clientCertsDir, "client-key.pem"),
		filepath.Join(agentCertsDir, "ca.pem"),
		filepath.Join(agentCertsDir, "agent-srv.pem"),
		filepath.Join(agentCertsDir, "agent-srv-key.pem"),
	}
}

func prepareRemoteAgentBinary(hostName string) (string, func(), error) {
	if configured := strings.TrimSpace(os.Getenv(kAPIAgentPathEnvVar)); configured != cg.EmptyStr {
		path, err := validateExecutableFile(configured)
		if err != nil {
			return cg.EmptyStr, func() {}, cerr.AppendErrorFmt("invalid %s value %q", err, kAPIAgentPathEnvVar, configured)
		}
		return path, func() {}, nil
	}

	repositoryRoot, err := repositoryRootForRemoteBuild()
	if err != nil {
		return cg.EmptyStr, func() {}, err
	}
	goarch, err := remoteGOArch(hostName)
	if err != nil {
		return cg.EmptyStr, func() {}, err
	}

	workDir, err := os.MkdirTemp(cg.EmptyStr, "cds-agent-")
	if err != nil {
		return cg.EmptyStr, func() {}, cerr.AppendError("Failed to create temp dir", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(workDir)
	}

	binaryPath := filepath.Join(workDir, "cds-api-agent")
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/api-agent/cds-api-agent.go")
	cmd.Dir = repositoryRoot
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+goarch)
	if output, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return cg.EmptyStr, func() {}, commandError("failed to build Linux agent binary", output, err)
	}
	return binaryPath, cleanup, nil
}

func repositoryRootForRemoteBuild() (string, error) {
	cwd, err := os.Getwd()
	if err == nil {
		if repoDir, ok := findCDSRepo(cwd); ok {
			return repoDir, nil
		}
	}

	executable, err := os.Executable()
	if err == nil {
		if repoDir, ok := findCDSRepo(filepath.Dir(executable)); ok {
			return repoDir, nil
		}
	}

	return cg.EmptyStr, cerr.NewError(fmt.Sprintf("CDS repository root not found; set %s to a Linux %s binary", kAPIAgentPathEnvVar, kAPIAgentBinaryName))
}

func remoteGOArch(hostName string) (string, error) {
	output, err := remoteOutput(hostName, "uname -m")
	if err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("Failed to detect CPU architecture on remote host %s", err, hostName)
	}
	return mapRemoteGOArch(strings.TrimSpace(output))
}

func mapRemoteGOArch(machine string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(machine)) {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("unsupported remote CPU architecture %q", machine))
	}
}

func copyRemoteAgentAssets(hostName, agentBinaryPath string) error {
	if err := remoteRun(hostName, fmt.Sprintf("mkdir -p ~/.local/bin %s", remoteAgentCertDir)); err != nil {
		return err
	}
	if err := scpToRemote(hostName, agentBinaryPath, remoteAgentBinaryPath); err != nil {
		return cerr.AppendError("Failed to copy agent binary to remote host", err)
	}

	certs := agentRuntimeCertFiles()
	args := make([]string, 0, len(certs)+1)
	args = append(args, certs...)
	args = append(args, remotePath(hostName, remoteAgentCertDir+"/"))
	if err := runSCP(args...); err != nil {
		return cerr.AppendError("Failed to copy agent certificates to remote host", err)
	}

	return remoteRun(hostName, fmt.Sprintf("chmod 755 %s && chmod 700 ~/.xcds-agent %s && chmod 600 %s/*", remoteAgentBinaryPath, remoteAgentCertDir, remoteAgentCertDir))
}

func agentRuntimeCertFiles() []string {
	agentCertsDir := cenv.AgentConfigDir("certs")
	return []string{
		filepath.Join(agentCertsDir, "ca.pem"),
		filepath.Join(agentCertsDir, "agent-srv.pem"),
		filepath.Join(agentCertsDir, "agent-srv-key.pem"),
	}
}

func startRemoteAgentProcess(hostName, port string) error {
	command := fmt.Sprintf("nohup %s -port %s > %s 2>&1 < /dev/null &", remoteAgentBinaryPath, port, remoteAgentLogPath)
	return remoteRun(hostName, command)
}

func stopRemoteAgentProcess(hostName, port string) error {
	command := fmt.Sprintf(`port=%s
pid=""
if command -v lsof >/dev/null 2>&1; then
  pid="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | head -n 1)"
fi
if [ -z "$pid" ] && command -v ss >/dev/null 2>&1; then
  pid="$(ss -ltnp "sport = :$port" 2>/dev/null | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' | head -n 1)"
fi
if [ -n "$pid" ]; then
  kill "$pid"
  for i in 1 2 3 4 5 6 7 8 9 10; do
    kill -0 "$pid" 2>/dev/null || exit 0
    sleep 0.1
  done
  exit 1
fi`, port)
	return remoteRun(hostName, command)
}

func agentPort(address string) (string, error) {
	normalized := strings.TrimSpace(address)
	if normalized == cg.EmptyStr || strings.HasPrefix(normalized, ":") {
		if strings.HasPrefix(normalized, ":") && len(normalized) > 1 {
			return validateAgentPort(strings.TrimPrefix(normalized, ":"))
		}
		return remoteAgentPort, nil
	}
	if strings.Contains(normalized, "://") {
		parsedURL, err := url.Parse(normalized)
		if err == nil && parsedURL.Port() != cg.EmptyStr {
			return validateAgentPort(parsedURL.Port())
		}
		return remoteAgentPort, nil
	}
	if _, port, err := net.SplitHostPort(normalized); err == nil && port != cg.EmptyStr {
		return validateAgentPort(port)
	}
	return remoteAgentPort, nil
}

func validateAgentPort(port string) (string, error) {
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("invalid agent port %q", port))
	}
	return port, nil
}

func agentTCPAddress(address string) (string, error) {
	normalized := strings.TrimSpace(address)
	if normalized == cg.EmptyStr {
		return cg.EmptyStr, cerr.NewError("agent address is required")
	}
	if strings.HasPrefix(normalized, ":") {
		return normalized, nil
	}
	if strings.Contains(normalized, "://") {
		parsedURL, err := url.Parse(normalized)
		if err != nil {
			return cg.EmptyStr, cerr.AppendError("failed to parse agent address", err)
		}
		host := parsedURL.Hostname()
		if host == cg.EmptyStr {
			return cg.EmptyStr, cerr.NewError("agent address host is required")
		}
		port, err := agentPort(normalized)
		if err != nil {
			return cg.EmptyStr, err
		}
		return net.JoinHostPort(host, port), nil
	}
	if host, port, err := net.SplitHostPort(normalized); err == nil {
		if _, err := validateAgentPort(port); err != nil {
			return cg.EmptyStr, err
		}
		if host == cg.EmptyStr {
			return ":" + port, nil
		}
		return net.JoinHostPort(host, port), nil
	}
	return net.JoinHostPort(normalized, remoteAgentPort), nil
}

func remoteOutput(hostName, command string) (string, error) {
	args := append(defaultSSHArgs(hostName), command)
	cmd := exec.Command("ssh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return cg.EmptyStr, commandError("failed to run remote command", output, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func remoteRun(hostName, command string) error {
	args := append(defaultSSHArgs(hostName), command)
	cmd := exec.Command("ssh", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return commandError("failed to run remote command", output, err)
	}
	return nil
}

func defaultSSHArgs(hostName string) []string {
	return []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		hostName,
	}
}

func scpToRemote(hostName, source, destination string) error {
	return runSCP(source, remotePath(hostName, destination))
}

func runSCP(args ...string) error {
	baseArgs := []string{"-q", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10"}
	cmd := exec.Command("scp", append(baseArgs, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return commandError("failed to copy file to remote host", output, err)
	}
	return nil
}

func remotePath(hostName, path string) string {
	return hostName + ":" + path
}

func commandError(message string, output []byte, err error) error {
	trimmed := strings.TrimSpace(string(output))
	if trimmed != cg.EmptyStr {
		message = fmt.Sprintf("%s: %s", message, trimmed)
	}
	return cerr.AppendError(message, err)
}
