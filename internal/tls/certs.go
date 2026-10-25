package cdstls

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

const (
	kcertsjsonDir   = "certsjson"
	kcertsDir       = "certs"
	kAgentsCertsDir = "agents"

	kCAFile         = "ca.pem"
	kCAKeyFile      = "ca-key.pem"
	kClientCertName = "client"
	kAgentCertName  = "agent-srv"
)

var (
	//go:embed json/ca-config.json
	caConfig []byte
	//go:embed json/ca-csr.json
	caCSR []byte
	//go:embed json/server-csr.json
	serverCSR []byte
	//go:embed json/client-csr.json
	clientCSR []byte
)

// writeCertsJsonFiles writes the embedded cfssl templates into jsonDir.
func writeCertsJsonFiles(jsonDir string) error {
	if err := os.MkdirAll(jsonDir, cg.KPermDir); err != nil {
		return cerr.AppendErrorFmt("Failed to create directory %s", err, jsonDir)
	}

	files := []struct {
		name string
		data []byte
	}{
		{name: "ca-config.json", data: caConfig},
		{name: "ca-csr.json", data: caCSR},
		{name: "server-csr.json", data: serverCSR},
		{name: "client-csr.json", data: clientCSR},
	}

	for _, file := range files {
		if err := os.WriteFile(filepath.Join(jsonDir, file.name), file.data, cg.KPermFile); err != nil {
			return cerr.AppendErrorFmt("Failed to write file %s", err, file.name)
		}
	}
	return nil
}

// EnsureCerts provisions the cds PKI. The CA is only generated once: agents
// already provisioned keep trusting it. Missing client or local agent
// certificates are issued again from the existing CA.
func EnsureCerts() error {
	clientCertsDir := cenv.ClientConfigDir(kcertsDir)
	if !filesExist(clientCertsDir, kCAFile, kCAKeyFile) {
		return BuildCerts()
	}

	issues := []struct {
		dir     string
		profile string
		csr     []byte
		name    string
	}{
		{dir: clientCertsDir, profile: "client", csr: clientCSR, name: kClientCertName},
		{dir: cenv.AgentConfigDir(kcertsDir), profile: "server", csr: serverCSR, name: kAgentCertName},
	}
	for _, issue := range issues {
		if filesExist(issue.dir, kCAFile, issue.name+".pem", issue.name+"-key.pem") {
			continue
		}
		clog.Debug(fmt.Sprintf("Issuing missing %s certificate in %s", issue.name, issue.dir))
		if err := issueCert(issue.dir, issue.profile, issue.csr, issue.name); err != nil {
			return err
		}
	}
	return nil
}

// AgentCertsDir returns the directory holding the CA and the agent server
// certificate (agent-srv.pem, agent-srv-key.pem) of a remote agent running on
// hostname, issuing the certificate from the cds CA when needed.
func AgentCertsDir(hostname string) (string, error) {
	if hostname == cg.EmptyStr || strings.ContainsAny(hostname, `/\`) || strings.HasPrefix(hostname, ".") {
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("invalid agent hostname %q", hostname))
	}
	if err := EnsureCerts(); err != nil {
		return cg.EmptyStr, err
	}

	dir := cenv.ClientConfigDir(filepath.Join(kcertsDir, kAgentsCertsDir, hostname))
	if filesExist(dir, kCAFile, kAgentCertName+".pem", kAgentCertName+"-key.pem") {
		return dir, nil
	}

	csr, err := serverCSRFor(hostname)
	if err != nil {
		return cg.EmptyStr, err
	}
	if err := issueCert(dir, "server", csr, kAgentCertName); err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("Failed to issue agent certificate for %s", err, hostname)
	}
	return dir, nil
}

// serverCSRFor returns the agent server CSR template bound to hostname.
func serverCSRFor(hostname string) ([]byte, error) {
	var csr map[string]any
	if err := json.Unmarshal(serverCSR, &csr); err != nil {
		return nil, cerr.AppendError("Failed to parse server CSR template", err)
	}
	csr["CN"] = hostname
	csr["hosts"] = []string{hostname}
	data, err := json.Marshal(csr)
	if err != nil {
		return nil, cerr.AppendError("Failed to serialize server CSR", err)
	}
	return data, nil
}

func filesExist(dir string, names ...string) bool {
	for _, name := range names {
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// withCfssl runs fn in a temporary working directory holding the cfssl templates.
func withCfssl(fn func(workDir string, certsjson func(string) string) error) error {
	workDir, err := os.MkdirTemp(cg.EmptyStr, "cds")
	if err != nil {
		return cerr.AppendError("Failed to create temp dir", err)
	}
	defer func() {
		_ = os.RemoveAll(workDir)
	}()

	jsonDir := filepath.Join(workDir, kcertsjsonDir)
	if err := writeCertsJsonFiles(jsonDir); err != nil {
		return cerr.AppendError("Failed to prepare certificate templates", err)
	}
	return fn(workDir, func(filename string) string {
		return filepath.Join(jsonDir, filename)
	})
}

func executePipes(workDir string, pipes []shexec.Pipe) error {
	for idx, pipe := range pipes {
		if _, err := shexec.ExecutePipe(pipe, workDir); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to execute pipe[%d](%s...)", idx, pipe.Left.Name), err)
		}
	}
	return nil
}

func signPipe(caFile, caKeyFile, configFile, profile, csrFile, name string) shexec.Pipe {
	return shexec.Pipe{
		Left: shexec.Execcmd{Name: "cfssl", Args: []string{"gencert",
			fmt.Sprintf("-ca=%s", caFile), fmt.Sprintf("-ca-key=%s", caKeyFile),
			fmt.Sprintf("-config=%s", configFile), fmt.Sprintf("-profile=%s", profile), csrFile}},
		Right: shexec.Execcmd{Name: "cfssljson", Args: []string{"-bare", name}},
	}
}

// issueCert signs a certificate for csr with the cds CA and installs it, along
// with the CA certificate, into dir as <name>.pem and <name>-key.pem.
func issueCert(dir, profile string, csr []byte, name string) error {
	clientCertsDir := cenv.ClientConfigDir(kcertsDir)
	return withCfssl(func(workDir string, certsjson func(string) string) error {
		csrFile := filepath.Join(workDir, name+"-csr.json")
		if err := os.WriteFile(csrFile, csr, cg.KPermFile); err != nil {
			return cerr.AppendError("Failed to write CSR", err)
		}
		pipe := signPipe(filepath.Join(clientCertsDir, kCAFile), filepath.Join(clientCertsDir, kCAKeyFile), certsjson("ca-config.json"), profile, csrFile, name)
		if err := executePipes(workDir, []shexec.Pipe{pipe}); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(clientCertsDir, kCAFile), filepath.Join(workDir, kCAFile)); err != nil {
			return err
		}
		return installFiles(workDir, dir, kCAFile, name+".pem", name+"-key.pem")
	})
}

// BuildCerts generates a new CA, the local agent server and the client
// certificates with cfssl, and installs them into the client and agent certs
// directories. Agents provisioned with a previous CA will no longer be trusted:
// use EnsureCerts unless a new PKI is wanted.
func BuildCerts() error {
	return withCfssl(func(workDir string, certsjson func(string) string) error {
		pipes := []shexec.Pipe{
			{
				Left:  shexec.Execcmd{Name: "cfssl", Args: []string{"gencert", "-initca", certsjson("ca-csr.json")}},
				Right: shexec.Execcmd{Name: "cfssljson", Args: []string{"-bare", "ca"}},
			},
			signPipe(kCAFile, kCAKeyFile, certsjson("ca-config.json"), "server", certsjson("server-csr.json"), kAgentCertName),
			signPipe(kCAFile, kCAKeyFile, certsjson("ca-config.json"), "client", certsjson("client-csr.json"), kClientCertName),
		}
		if err := executePipes(workDir, pipes); err != nil {
			return err
		}
		if err := installCerts(workDir); err != nil {
			return cerr.AppendError("Failed to install certificates", err)
		}
		return nil
	})
}

// installCerts copies the generated material from workDir into the client and
// agent certs directories, each one receiving only what it needs.
// The CA key stays on the client side, which is the trust root issuing certificates.
func installCerts(workDir string) error {
	if err := installFiles(workDir, cenv.ClientConfigDir(kcertsDir), kCAFile, kCAKeyFile, kClientCertName+".pem", kClientCertName+"-key.pem"); err != nil {
		return err
	}
	return installFiles(workDir, cenv.AgentConfigDir(kcertsDir), kCAFile, kAgentCertName+".pem", kAgentCertName+"-key.pem")
}

func installFiles(srcDir, dstDir string, files ...string) error {
	if err := os.MkdirAll(dstDir, cg.KPermDir); err != nil {
		return cerr.AppendErrorFmt("Failed to create certs directory %s", err, dstDir)
	}
	for _, file := range files {
		if err := copyFile(filepath.Join(srcDir, file), filepath.Join(dstDir, file)); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to read certificate %s", err, src)
	}
	if err := os.WriteFile(dst, data, cg.KPermFile); err != nil {
		return cerr.AppendErrorFmt("Failed to write certificate %s", err, dst)
	}
	return nil
}
