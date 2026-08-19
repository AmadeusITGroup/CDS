package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

	if err := ensureRuntimeCerts(); err != nil {
		return err
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
	address, err := nextLocalAgentAddress()
	if err != nil {
		return cg.EmptyStr, err
	}
	port, err := agentPort(address)
	if err != nil {
		return cg.EmptyStr, err
	}
	agentCmd = agentCmd.withPort(port)

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
	if err := waitForHealthyAgent(address); err != nil {
		return cg.EmptyStr, err
	}
	recordManagedAgentOwnership(address, agentCmd.name, cmd.Process.Pid)
	return address, nil
}

// recordManagedAgentOwnership persists, best-effort, the identity of the local
// agent process CDS started so it can later be stopped via StopManagedAgent.
// Failures (e.g. the host is not yet registered) are non-fatal.
func recordManagedAgentOwnership(address, binary string, pid int) {
	if err := db.SetHostAgentOwnership(cg.KLocalhost, bo.AgentOwnership{
		PID:     pid,
		Address: address,
		Binary:  binary,
		Manager: "process",
	}); err != nil {
		clog.Debug(fmt.Sprintf("Could not record local agent ownership: %v", err))
	}
}

func (c agentCommand) withPort(port string) agentCommand {
	args := make([]string, 0, len(c.args)+2)
	args = append(args, c.args...)
	args = append(args, "-port", port)
	c.args = args
	return c
}

func nextLocalAgentAddress() (string, error) {
	listener, err := net.Listen("tcp", localAgentAddress)
	if err == nil {
		_ = listener.Close()
		return localAgentAddress, nil
	}

	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("Failed to allocate local agent port", err)
	}
	defer func() {
		_ = listener.Close()
	}()
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("unexpected local listener address %s", listener.Addr()))
	}
	return fmt.Sprintf(":%d", tcpAddr.Port), nil
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

	tcpAddress, err := agentTCPAddress(address)
	if err != nil {
		return false, address, err
	}

	conn, err := net.DialTimeout("tcp", tcpAddress, time.Second)
	if err != nil {
		clog.Debug(fmt.Sprintf("Agent is not listening at %s", tcpAddress))
		return false, address, nil
	}
	defer func() {
		_ = conn.Close()
	}()
	if err := verifyAgentHealth(address); err != nil {
		clog.Warn(fmt.Sprintf("Agent is listening at %s but failed TLS verification; it will be repaired.", tcpAddress), err)
		return false, address, nil
	}
	return true, address, nil
}

func waitForHealthyAgent(address string) error {
	deadline := time.Now().Add(localAgentStartDeadline)
	var lastErr error
	for {
		if err := verifyAgentHealth(address); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return cerr.AppendErrorFmt("agent did not pass TLS verification on %s", lastErr, address)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func verifyAgentHealth(address string) error {
	tcpAddress, err := agentTCPAddress(address)
	if err != nil {
		return err
	}

	target := agentTLSMaterialForAddress(address)
	clientTLSConfig, err := cdstls.SetupTLSConfig(cdstls.TLSConfig{
		CAFile:        target.caFile,
		CertFile:      target.certFile,
		KeyFile:       target.keyFile,
		ServerAddress: cg.KLocalhost,
	})
	if err != nil {
		return cerr.AppendError("Failed to setup agent TLS verification", err)
	}

	conn, err := grpc.NewClient(tcpAddress, grpc.WithTransportCredentials(credentials.NewTLS(clientTLSConfig)))
	if err != nil {
		return cerr.AppendError("Failed to create agent health client", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = cdspb.NewAgentInfoServiceClient(conn).GetVersion(ctx, &cdspb.GetVersionRequest{})
	return err
}

type agentTLSFiles struct {
	caFile   string
	certFile string
	keyFile  string
}

func agentTLSMaterialForAddress(address string) agentTLSFiles {
	material := agentTLSFiles{
		caFile:   cdstls.CAFilePath(),
		certFile: cdstls.ClientCertFilePath(),
		keyFile:  cdstls.ClientKeyFilePath(),
	}
	registeredAgent, err := config.RegisteredAgent(address)
	if err != nil {
		return material
	}
	if registeredAgent.Certs.CA != cg.EmptyStr {
		material.caFile = registeredAgent.Certs.CA
	}
	if registeredAgent.Certs.Cert != cg.EmptyStr {
		material.certFile = registeredAgent.Certs.Cert
	}
	if registeredAgent.Certs.Key != cg.EmptyStr {
		material.keyFile = registeredAgent.Certs.Key
	}
	return material
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
				config.WithCA(cdstls.CAFilePath()),
				config.WithCert(cdstls.ClientCertFilePath()),
				config.WithKey(cdstls.ClientKeyFilePath()),
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
