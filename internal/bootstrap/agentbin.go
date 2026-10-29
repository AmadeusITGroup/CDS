package bootstrap

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	kAgentsCacheDir = "cache/agents"
	kPermExecutable = os.FileMode(0700)
)

var sHTTPClient = &http.Client{Timeout: 10 * time.Minute}

// platform identifies the OS and architecture an agent binary is built for, as GOOS/GOARCH.
type platform struct {
	os   string
	arch string
}

func (p platform) String() string {
	return p.os + "/" + p.arch
}

func localPlatform() platform {
	return platform{os: runtime.GOOS, arch: runtime.GOARCH}
}

func agentBinaryName(p platform) string {
	if p.os == cg.WINDOWS {
		return kAgentBinaryName + ".exe"
	}
	return kAgentBinaryName
}

// resolveAgentBinary returns the local path of the agent binary matching the
// version of cds for platform p. In order:
//   - the agent_binary setting (CDS_AGENT_BINARY) when it exists for p, mainly for development;
//   - for the local platform, the binary on PATH or next to the cds executable;
//   - the local cache of downloaded binaries;
//   - a download from the agent_download_url setting (CDS_AGENT_DOWNLOAD_URL) into the cache.
func resolveAgentBinary(p platform) (string, error) {
	if configured := config.AgentBinary(p.os, p.arch, cg.Version); configured != cg.EmptyStr {
		// the template may not be provided for every platform
		if _, err := os.Stat(configured); err == nil {
			if err := validateExecutable(configured); err != nil {
				return cg.EmptyStr, cerr.AppendErrorFmt("invalid configured agent binary for %s", err, p)
			}
			return configured, nil
		}
		clog.Debug(fmt.Sprintf("Configured agent binary %s does not exist for %s", configured, p))
	}

	if p == localPlatform() {
		if path, err := exec.LookPath(agentBinaryName(p)); err == nil {
			return path, nil
		}
		if executable, err := os.Executable(); err == nil {
			sibling := filepath.Join(filepath.Dir(executable), agentBinaryName(p))
			if validateExecutable(sibling) == nil {
				return sibling, nil
			}
		}
	}

	cached := cenv.ClientConfigDir(filepath.Join(kAgentsCacheDir, cg.Version, p.os+"-"+p.arch, agentBinaryName(p)))
	if validateExecutable(cached) == nil {
		return cached, nil
	}

	if downloadURL := config.AgentDownloadURL(p.os, p.arch, cg.Version); downloadURL != cg.EmptyStr {
		if err := downloadAgentBinary(downloadURL, cached); err != nil {
			return cg.EmptyStr, err
		}
		return cached, nil
	}

	return cg.EmptyStr, cerr.NewError(fmt.Sprintf(
		"no %s binary available for %s (version %s): set CDS_AGENT_BINARY or CDS_AGENT_DOWNLOAD_URL",
		kAgentBinaryName, p, cg.Version))
}

func validateExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return cerr.AppendErrorFmt("cannot access %s", err, path)
	}
	if !info.Mode().IsRegular() {
		return cerr.NewError(fmt.Sprintf("%s is not a regular file", path))
	}
	// The Unix owner-execute bit is only meaningful on platforms that expose it
	// through os.Stat. On Windows, os.FileInfo does not carry Unix permission
	// bits, so a valid executable (including a Linux agent binary a Windows
	// client resolves to upload for remote bootstrap) would be wrongly rejected.
	if runtime.GOOS != cg.WINDOWS && info.Mode().Perm()&0100 == 0 {
		return cerr.NewError(fmt.Sprintf("%s is not an executable file", path))
	}
	return nil
}

// downloadAgentBinary downloads the agent binary into dst, atomically.
func downloadAgentBinary(rawURL, dst string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return cerr.NewError(fmt.Sprintf("agent download URL %q must be an https URL", rawURL))
	}

	clog.Info(fmt.Sprintf("Downloading %s from %s", kAgentBinaryName, rawURL))
	resp, err := sHTTPClient.Get(rawURL)
	if err != nil {
		return cerr.AppendErrorFmt("failed to download %s", err, rawURL)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return cerr.NewError(fmt.Sprintf("failed to download %s: %s", rawURL, resp.Status))
	}

	if err := os.MkdirAll(filepath.Dir(dst), cg.KPermDir); err != nil {
		return cerr.AppendErrorFmt("failed to create %s", err, filepath.Dir(dst))
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".download-*")
	if err != nil {
		return cerr.AppendError("failed to create download file", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return cerr.AppendErrorFmt("failed to download %s", err, rawURL)
	}
	if err := tmp.Close(); err != nil {
		return cerr.AppendError("failed to write download file", err)
	}
	if err := os.Chmod(tmp.Name(), kPermExecutable); err != nil {
		return cerr.AppendError("failed to make the agent binary executable", err)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return cerr.AppendErrorFmt("failed to install %s", err, dst)
	}
	return nil
}
