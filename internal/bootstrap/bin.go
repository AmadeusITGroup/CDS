//nolint:unused
package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

const (
	kAPIAgentBinaryName = "cds-api-agent"
	kAPIAgentPathEnvVar = "CDS_API_AGENT_PATH"
)

func ensureBinary(bin binary) error {
	name := bin.name()
	path, err := exec.LookPath(name)

	if err == nil {
		clog.Debug(fmt.Sprintf("%s binary is available at %s", name, path))
		return nil //
	}

	notFound := errors.Is(err, exec.ErrNotFound)
	if notFound {
		return bin.install()
	}
	return cerr.NewError(fmt.Sprintf("Neither found nor managed to install %s binary", name))
}

type binary interface {
	name() string
	install() error
}

type cfsslbin struct {
	n string
}

func (c cfsslbin) name() string {
	return c.n
}

func (c cfsslbin) install() error {
	clog.Warn("Not implemented yet")
	return nil
}

type cfssljsonbin struct {
	n string
}

func (c cfssljsonbin) name() string {
	return c.n
}

func (c cfssljsonbin) install() error {
	clog.Warn("Not implemented yet")
	return nil
}

type cdsagentbin struct {
	n string
}

func (c cdsagentbin) name() string {
	return c.n
}

func (c cdsagentbin) install() error {
	_, err := resolveAgentBinaryPath()
	return err
}

func resolveAgentBinaryPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv(kAPIAgentPathEnvVar)); override != "" {
		path, err := validateExecutableFile(override)
		if err != nil {
			return "", cerr.AppendErrorFmt("invalid %s value %q", err, kAPIAgentPathEnvVar, override)
		}
		return path, nil
	}

	path, err := exec.LookPath(kAPIAgentBinaryName)
	if err == nil {
		return path, nil
	}
	if !errors.Is(err, exec.ErrNotFound) {
		return "", cerr.AppendErrorFmt("failed to resolve %s from PATH", err, kAPIAgentBinaryName)
	}

	for _, candidate := range localAgentBinaryCandidates() {
		path, err := validateExecutableFile(candidate)
		if err == nil {
			clog.Debug(fmt.Sprintf("%s binary is available at %s", kAPIAgentBinaryName, path))
			return path, nil
		}
	}

	return "", cerr.NewError(fmt.Sprintf(
		"%s binary not found. Run `make build-api-agent-fast`, set %s, or put %s on PATH",
		kAPIAgentBinaryName,
		kAPIAgentPathEnvVar,
		kAPIAgentBinaryName,
	))
}

func localAgentBinaryCandidates() []string {
	candidates := make([]string, 0, 3)
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), kAPIAgentBinaryName))
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return candidates
	}
	candidates = append(candidates, filepath.Join(workingDir, kAPIAgentBinaryName))

	if repositoryRoot := findCDSRepositoryRoot(workingDir); repositoryRoot != "" {
		candidates = append(candidates, filepath.Join(repositoryRoot, kAPIAgentBinaryName))
	}

	return uniquePaths(candidates)
}

func findCDSRepositoryRoot(startDir string) string {
	for dir := startDir; dir != ""; dir = filepath.Dir(dir) {
		if isCDSGoModule(filepath.Join(dir, "go.mod")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return ""
}

func isCDSGoModule(goModPath string) bool {
	data, err := os.ReadFile(goModPath)
	return err == nil && strings.Contains(string(data), "module github.com/amadeusitgroup/cds")
}

func validateExecutableFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", cerr.NewError(fmt.Sprintf("%s is a directory", path))
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return "", cerr.NewError(fmt.Sprintf("%s is not executable", path))
	}
	return path, nil
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}
