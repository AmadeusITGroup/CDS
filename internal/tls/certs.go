package cdstls

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
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

func certsjson(jsonDir, filename string) string {
	return filepath.Join(jsonDir, filename)
}

func ensureCertsJsonFiles(jsonDir string) error {
	if err := os.MkdirAll(jsonDir, cg.KPermDir); err != nil {
		return cerr.AppendError("Failed to create certs JSON directory", err)
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
		if err := os.WriteFile(certsjson(jsonDir, file.name), file.data, cg.KPermFile); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to write file %s", file.name), err)
		}
	}
	return nil
}

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

	jsonDir := filepath.Join(workDir, kcertsjsonDir)
	if err := ensureCertsJsonFiles(jsonDir); err != nil {
		return cerr.AppendError("Failed to prepare certificate templates", err)
	}

	pipes := []shexec.Pipe{
		{
			Left:  shexec.Execcmd{Name: "cfssl", Args: []string{"gencert", "-initca", certsjson(jsonDir, "ca-csr.json")}},
			Right: shexec.Execcmd{Name: "cfssljson", Args: []string{"-bare", "ca"}},
		},
		{
			Left:  shexec.Execcmd{Name: "cfssl", Args: []string{"gencert", "-ca=ca.pem", "-ca-key=ca-key.pem", fmt.Sprintf("-config=%s", certsjson(jsonDir, "ca-config.json")), "-profile=server", certsjson(jsonDir, "server-csr.json")}},
			Right: shexec.Execcmd{Name: "cfssljson", Args: []string{"-bare", "agent-srv"}},
		},
		{
			Left:  shexec.Execcmd{Name: "cfssl", Args: []string{"gencert", "-ca=ca.pem", "-ca-key=ca-key.pem", fmt.Sprintf("-config=%s", certsjson(jsonDir, "ca-config.json")), "-profile=client", certsjson(jsonDir, "client-csr.json")}},
			Right: shexec.Execcmd{Name: "cfssljson", Args: []string{"-bare", "client"}},
		},
	}

	for idx, pipe := range pipes {
		if _, err := shexec.ExecutePipe(pipe, workDir); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to execute pipe[%d](%s...)", idx, pipe.Left.Name), err)
		}
	}

	if err := installRuntimeCerts(workDir); err != nil {
		return cerr.AppendError("Failed to install runtime certificates", err)
	}
	return nil
}

func installRuntimeCerts(workDir string) error {
	clientCertsDir := cenv.ClientConfigDir(kcertsDir)
	agentCertsDir := cenv.AgentConfigDir(kcertsDir)

	for _, dir := range []string{clientCertsDir, agentCertsDir} {
		if err := cenv.EnsureDir(dir, cg.KPermDir); err != nil {
			return err
		}
		if err := cleanManagedCertDir(dir); err != nil {
			return err
		}
	}

	certs := []struct {
		src string
		dst string
	}{
		{src: "ca.pem", dst: filepath.Join(clientCertsDir, "ca.pem")},
		{src: "client.pem", dst: filepath.Join(clientCertsDir, "client.pem")},
		{src: "client-key.pem", dst: filepath.Join(clientCertsDir, "client-key.pem")},
		{src: "ca.pem", dst: filepath.Join(agentCertsDir, "ca.pem")},
		{src: "agent-srv.pem", dst: filepath.Join(agentCertsDir, "agent-srv.pem")},
		{src: "agent-srv-key.pem", dst: filepath.Join(agentCertsDir, "agent-srv-key.pem")},
	}

	for _, cert := range certs {
		data, err := os.ReadFile(filepath.Join(workDir, cert.src))
		if err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to read generated certificate %s", cert.src), err)
		}
		if err := os.WriteFile(cert.dst, data, cg.KPermFile); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to write certificate %s", cert.dst), err)
		}
	}

	if err := removeObsoleteCertsJSONDirs(); err != nil {
		return cerr.AppendError("Failed to remove obsolete certificate template directories", err)
	}

	return nil
}

func cleanManagedCertDir(dir string) error {
	for _, pattern := range []string{"*.pem", "*.csr"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to list managed certificate files in %s", dir), err)
		}
		for _, match := range matches {
			if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
				return cerr.AppendError(fmt.Sprintf("Failed to remove managed certificate file %s", match), err)
			}
		}
	}
	return nil
}

func removeObsoleteCertsJSONDirs() error {
	for _, dir := range []string{cenv.ClientConfigDir(kcertsjsonDir), cenv.AgentConfigDir(kcertsjsonDir)} {
		if err := os.RemoveAll(dir); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to remove directory %s", dir), err)
		}
	}
	return nil
}
