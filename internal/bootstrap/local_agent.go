package bootstrap

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
)

const (
	localAgentAddress       = ":8087"
	agentBinaryName         = "cds-api-agent"
	agentBinaryEnv          = "CDS_API_AGENT_BIN"
	localAgentStartDeadline = 30 * time.Second
)

var (
	localAgentRequiredBinaries = []binary{cfsslbin{n: "cfssl"}, cfssljsonbin{n: "cfssljson"}}
	agentLookPath              = exec.LookPath
	agentExecutable            = os.Executable
	agentWorkingDir            = os.Getwd
	agentStat                  = os.Stat
)

type agentCommand struct {
	name string
	args []string
	dir  string
}

func fireLocalAgent(osName string) error {
	clog.Debug(fmt.Sprintf("Starting agent on %s", osName))
	defer clog.Debug(fmt.Sprintf("Agent started on %s", osName))

	for _, bin := range localAgentRequiredBinaries {
		if err := ensureBinary(bin); err != nil {
			return cerr.AppendErrorFmt("Failed to install binary %s", err, bin.name())
		}
	}

	if err := cdstls.BuildCerts(); err != nil {
		return cerr.AppendError("Failed to build certs", err)
	}

	server, err := startAgent()
	if err != nil {
		return cerr.AppendError("Failed to start agent", err)
	}

	return registerLocalAgent(server)
}

func startAgent() (string, error) {
	agentCmd, err := resolveAgentCommand()
	if err != nil {
		return cg.EmptyStr, err
	}

	cmd := exec.Command(agentCmd.name, agentCmd.args...)
	cmd.Dir = agentCmd.dir
	prepareAgentCommand(cmd)

	clog.Debug("Start agent...")
	if err := cmd.Start(); err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to start agent", err)
	}

	go func() {
		clog.Debug("Waiting agent to start...")
		if err := cmd.Wait(); err != nil {
			clog.Error("Command finished with error:", err)
		}
	}()

	clog.Debug(fmt.Sprintf("Agent started with pid: %d", cmd.Process.Pid))
	if err := waitForAgent(localAgentAddress); err != nil {
		return cg.EmptyStr, err
	}
	return localAgentAddress, nil
}

func resolveAgentCommand() (agentCommand, error) {
	if configured := strings.TrimSpace(os.Getenv(agentBinaryEnv)); configured != cg.EmptyStr {
		return agentCommand{name: configured}, nil
	}

	if path, err := agentLookPath(agentBinaryName); err == nil {
		return agentCommand{name: path}, nil
	}

	if exe, err := agentExecutable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), agentBinaryName)
		if isExecutableFile(candidate) {
			return agentCommand{name: candidate}, nil
		}
		if runtime.GOOS == "windows" {
			windowsCandidate := candidate + ".exe"
			if isExecutableFile(windowsCandidate) {
				return agentCommand{name: windowsCandidate}, nil
			}
		}
	}

	if cwd, err := agentWorkingDir(); err == nil {
		candidate := filepath.Join(cwd, agentBinaryName)
		if isExecutableFile(candidate) {
			return agentCommand{name: candidate}, nil
		}
		if runtime.GOOS == "windows" {
			windowsCandidate := candidate + ".exe"
			if isExecutableFile(windowsCandidate) {
				return agentCommand{name: windowsCandidate}, nil
			}
		}
		if repoDir, ok := findCDSRepo(cwd); ok {
			if goBinary, err := agentLookPath("go"); err == nil {
				return agentCommand{
					name: goBinary,
					args: []string{"run", "./cmd/api-agent/cds-api-agent.go"},
					dir:  repoDir,
				}, nil
			}
		}
	}

	return agentCommand{}, cerr.NewError(fmt.Sprintf("%s not found; build it with `make build-api-agent` or set %s", agentBinaryName, agentBinaryEnv))
}

func isAgentRunning(hostName string) (bool, string, error) {
	address, found, err := config.AgentAddressIfRegistered(hostName)
	if err != nil {
		return false, cg.EmptyStr, cerr.AppendError("failed to resolve agent address", err)
	}
	if !found {
		if !isLocalHost(hostName) {
			return false, cg.EmptyStr, nil
		}
		address = localAgentAddress
	}

	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		clog.Debug(fmt.Sprintf("Agent is not listening at %s", address))
		return false, address, nil
	}
	defer func() {
		_ = conn.Close()
	}()
	return true, address, nil
}

func waitForAgent(address string) error {
	deadline := time.Now().Add(localAgentStartDeadline)
	for {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return cerr.AppendErrorFmt("agent did not start listening on %s", err, address)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func registerLocalAgent(address string) error {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return cerr.AppendError("Failed to parse agent address", err)
	}

	if err := config.UpsertAgentForHostInConfig(cg.KLocalhost, config.NewAgent(
		config.WithTargetAddress(addr.String()),
		config.WithAgentTLS(
			config.NewTlssecret(
				config.WithCA(cdstls.CAFilePath),
				config.WithCert(cdstls.ClientCertFilePath),
				config.WithKey(cdstls.ClientKeyFilePath),
			),
		),
	)); err != nil {
		return cerr.AppendError("failed to register local agent in CLI config", err)
	}
	return nil
}

func isLocalHost(hostName string) bool {
	normalized := strings.TrimSpace(hostName)
	if normalized == cg.EmptyStr || normalized == cg.KLocalhost || strings.HasPrefix(normalized, ":") {
		return true
	}
	if strings.Contains(normalized, "://") {
		parsedURL, err := url.Parse(normalized)
		return err == nil && parsedURL.Hostname() == cg.KLocalhost
	}
	host, _, err := net.SplitHostPort(normalized)
	return err == nil && (host == cg.EmptyStr || host == cg.KLocalhost)
}

func isExecutableFile(path string) bool {
	info, err := agentStat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func findCDSRepo(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		if isFile(filepath.Join(dir, "go.mod")) && isFile(filepath.Join(dir, "cmd", "api-agent", "cds-api-agent.go")) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cg.EmptyStr, false
		}
		dir = parent
	}
}

func isFile(path string) bool {
	info, err := agentStat(path)
	return err == nil && !info.IsDir()
}
