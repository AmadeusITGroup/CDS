package cdstls

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	clientCAPath := filepath.Join(clientCertsDir, kCAFile)
	for _, issue := range issues {
		if leafReusable(issue.dir, issue.name, clientCAPath) {
			continue
		}
		clog.Debug(fmt.Sprintf("Issuing missing or invalid %s certificate in %s", issue.name, issue.dir))
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
	clientCAPath := filepath.Join(cenv.ClientConfigDir(kcertsDir), kCAFile)
	if leafReusable(dir, kAgentCertName, clientCAPath) {
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

// leafReusable reports whether the <name> leaf certificate in dir can still be
// used with the active client CA at caPath: all files exist, the leaf parses,
// is within its validity window, chains to the active CA, and its private key
// matches it. caPath is the current client CA (not the possibly-stale copy in
// dir), so a leaf signed by a replaced CA — or paired with a replaced key — is
// reissued instead of being reused by mere existence and date validity.
func leafReusable(dir, name, caPath string) bool {
	if !filesExist(dir, kCAFile, name+".pem", name+"-key.pem") {
		return false
	}
	certPath := filepath.Join(dir, name+".pem")
	keyPath := filepath.Join(dir, name+"-key.pem")
	if err := certValidNow(certPath); err != nil {
		clog.Debug(fmt.Sprintf("Reissuing %s: %v", certPath, err))
		return false
	}
	if err := certMatchesCAAndKey(certPath, keyPath, caPath); err != nil {
		clog.Debug(fmt.Sprintf("Reissuing %s: %v", certPath, err))
		return false
	}
	return true
}

// certMatchesCAAndKey verifies the leaf certificate chains to the CA and that
// the key pair is consistent, returning a describing error otherwise.
func certMatchesCAAndKey(certPath, keyPath, caPath string) error {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to read certificate %s", err, certPath)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to read key %s", err, keyPath)
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to read CA %s", err, caPath)
	}

	// key must match the certificate (tls.X509KeyPair fails on a mismatch)
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return cerr.AppendError("certificate and key do not match", err)
	}

	// certificate must chain to the CA currently in use
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return cerr.NewError(fmt.Sprintf("no CA certificate in %s", caPath))
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return cerr.NewError(fmt.Sprintf("no PEM certificate in %s", certPath))
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to parse certificate %s", err, certPath)
	}
	// verify the signature chain only; key usage is enforced at handshake time
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); err != nil {
		return cerr.AppendError("certificate does not chain to the active CA", err)
	}
	return nil
}

// certValidNow returns nil when the PEM certificate file holds a certificate
// valid at the current time, or an error describing why it is not.
func certValidNow(certPath string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to read certificate %s", err, certPath)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return cerr.NewError(fmt.Sprintf("no PEM certificate in %s", certPath))
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to parse certificate %s", err, certPath)
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return cerr.NewError(fmt.Sprintf("certificate %s is not valid at %s (valid %s to %s)", certPath, now.Format(time.RFC3339), cert.NotBefore.Format(time.RFC3339), cert.NotAfter.Format(time.RFC3339)))
	}
	return nil
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
