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

func init() {
	if err := cenv.EnsureDir(cenv.ConfigDir(kcertsjsonDir), cg.KPermDir); err != nil {
		clog.Error("Failed to create certs tmp directory", err)
	}
	if err := ensureCertsJsonFiles(); err != nil {
		clog.Error("Failed to ensure certs json files", err)
	}
	if err := cenv.EnsureDir(cenv.ConfigDir(kcertsDir), cg.KPermDir); err != nil {
		clog.Error("Failed to create certs directory", err)
	}
}

func certsjson(filename string) string {
	return filepath.Join(cenv.ConfigDir(kcertsjsonDir), filename)
}

func ensureCertsJsonFiles() error {
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
		if err := os.WriteFile(certsjson(file.name), file.data, cg.KPermFile); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to write file %s", file.name), err)
		}
	}
	return nil
}

func BuildCerts() error {
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

	for idx, pipe := range pipes {
		if _, err := shexec.ExecutePipe(pipe, workDir); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to execute pipe[%d](%s...)", idx, pipe.Left.Name), err)
		}
	}

	// wildcard (*), in mv command, needs a shell to be interpreted correctly
	move := shexec.Execcmd{Name: "sh", Args: []string{"-c", fmt.Sprintf("mv *.pem *.csr %s", cenv.ConfigDir(kcertsDir))}}
	if out, err := shexec.ExecuteCmd(move, workDir); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to execute command(%s...): %s", move.Name, out), err)
	}
	return nil
}
