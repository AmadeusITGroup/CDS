package cdstls

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

const (
	kcertsjsonDir = "certsjson"
	kcertsDir     = "certs"
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

// BuildCerts generates the CA, agent server and client certificates with cfssl
// and moves them into the certs directory of the active configuration directory.
// cfssl templates only live in a temporary working directory.
func BuildCerts() error {
	var (
		workDir string
		err     error
	)

	if workDir, err = os.MkdirTemp(cg.EmptyStr, "cds"); err != nil {
		return cerr.AppendError("Failed to create temp dir", err)
	}
	defer func() {
		_ = os.RemoveAll(workDir)
	}()

	clog.Debug(fmt.Sprintf("Working directory: %s", workDir))

	jsonDir := filepath.Join(workDir, kcertsjsonDir)
	if err := writeCertsJsonFiles(jsonDir); err != nil {
		return cerr.AppendError("Failed to prepare certificate templates", err)
	}
	certsjson := func(filename string) string {
		return filepath.Join(jsonDir, filename)
	}

	pipes := []shexec.Pipe{
		{
			Left:  shexec.Execcmd{Name: "cfssl", Args: []string{"gencert", "-initca", certsjson("ca-csr.json")}},
			Right: shexec.Execcmd{Name: "cfssljson", Args: []string{"-bare", "ca"}},
		},
		{
			Left:  shexec.Execcmd{Name: "cfssl", Args: []string{"gencert", "-ca=ca.pem", "-ca-key=ca-key.pem", fmt.Sprintf("-config=%s", certsjson("ca-config.json")), "-profile=server", certsjson("server-csr.json")}},
			Right: shexec.Execcmd{Name: "cfssljson", Args: []string{"-bare", "agent-srv"}},
		},
		{
			Left:  shexec.Execcmd{Name: "cfssl", Args: []string{"gencert", "-ca=ca.pem", "-ca-key=ca-key.pem", fmt.Sprintf("-config=%s", certsjson("ca-config.json")), "-profile=client", certsjson("client-csr.json")}},
			Right: shexec.Execcmd{Name: "cfssljson", Args: []string{"-bare", "client"}},
		},
	}

	for idx, pipe := range pipes {
		if _, err := shexec.ExecutePipe(pipe, workDir); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to execute pipe[%d](%s...)", idx, pipe.Left.Name), err)
		}
	}

	if err := installCerts(workDir); err != nil {
		return cerr.AppendError("Failed to install certificates", err)
	}
	return nil
}

// installCerts copies the generated material from workDir into the client and
// agent certs directories, each one receiving only what it needs.
// The CA key stays on the client side, which is the trust root issuing certificates.
func installCerts(workDir string) error {
	clientCertsDir := cenv.ClientConfigDir(kcertsDir)
	agentCertsDir := cenv.AgentConfigDir(kcertsDir)

	installs := []struct {
		dir   string
		files []string
	}{
		{dir: clientCertsDir, files: []string{"ca.pem", "ca-key.pem", "client.pem", "client-key.pem"}},
		{dir: agentCertsDir, files: []string{"ca.pem", "agent-srv.pem", "agent-srv-key.pem"}},
	}

	for _, install := range installs {
		if err := os.MkdirAll(install.dir, cg.KPermDir); err != nil {
			return cerr.AppendErrorFmt("Failed to create certs directory %s", err, install.dir)
		}
		for _, file := range install.files {
			data, err := os.ReadFile(filepath.Join(workDir, file))
			if err != nil {
				return cerr.AppendErrorFmt("Failed to read generated certificate %s", err, file)
			}
			dst := filepath.Join(install.dir, file)
			if err := os.WriteFile(dst, data, cg.KPermFile); err != nil {
				return cerr.AppendErrorFmt("Failed to write certificate %s", err, dst)
			}
		}
	}
	return nil
}
