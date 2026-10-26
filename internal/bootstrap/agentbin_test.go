package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// remotePlatform is never the platform the tests run on.
var remotePlatform = platform{os: "linux", arch: "riscv64"}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0700))
}

func TestValidateExecutable(t *testing.T) {
	dir := t.TempDir()

	exe := filepath.Join(dir, "agent")
	require.NoError(t, os.WriteFile(exe, []byte("bin"), 0700))
	assert.NoError(t, validateExecutable(exe), "a regular executable file is valid")

	missing := filepath.Join(dir, "nope")
	assert.Error(t, validateExecutable(missing), "a missing file is invalid")

	assert.Error(t, validateExecutable(dir), "a directory is not a regular file")

	// On Unix a file without the execute bit is rejected; on Windows the bit is
	// not exposed by os.Stat, so a regular file must be accepted regardless.
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.WriteFile(plain, []byte("data"), 0600))
	if runtime.GOOS == cg.WINDOWS {
		assert.NoError(t, validateExecutable(plain))
	} else {
		assert.Error(t, validateExecutable(plain), "a non-executable file is rejected on Unix")
	}
}

func TestResolveAgentBinaryFromSetting(t *testing.T) {
	t.Setenv("CDS_CONFIG_PATH", t.TempDir())
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "cds-api-agent-linux-riscv64"), "agent")
	t.Setenv("CDS_AGENT_BINARY", filepath.Join(dir, "cds-api-agent-{{os}}-{{arch}}"))

	path, err := resolveAgentBinary(remotePlatform)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "cds-api-agent-linux-riscv64"), path)

	require.NoError(t, os.Chmod(path, 0600))
	_, err = resolveAgentBinary(remotePlatform)
	assert.Error(t, err, "a configured binary must be executable")

	_, err = resolveAgentBinary(platform{os: "linux", arch: "s390x"})
	require.Error(t, err, "a missing configured binary falls back to the other sources")
	assert.Contains(t, err.Error(), "no cds-api-agent binary available for linux/s390x")
}

func TestResolveAgentBinaryFromCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)
	cached := filepath.Join(root, ".xcds", "cache", "agents", cg.Version, "linux-riscv64", "cds-api-agent")
	writeExecutable(t, cached, "agent")

	path, err := resolveAgentBinary(remotePlatform)
	require.NoError(t, err)
	assert.Equal(t, cached, path)
}

func TestResolveAgentBinaryDownloadsIntoCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", root)
	var requested string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		_, _ = w.Write([]byte("downloaded agent"))
	}))
	defer server.Close()
	previous := sHTTPClient
	sHTTPClient = server.Client()
	t.Cleanup(func() { sHTTPClient = previous })
	t.Setenv("CDS_AGENT_DOWNLOAD_URL", server.URL+"/{{version}}/cds-api-agent-{{os}}-{{arch}}")

	path, err := resolveAgentBinary(remotePlatform)

	require.NoError(t, err)
	assert.Equal(t, "/"+cg.Version+"/cds-api-agent-linux-riscv64", requested)
	assert.Equal(t, filepath.Join(root, ".xcds", "cache", "agents", cg.Version, "linux-riscv64", "cds-api-agent"), path)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "downloaded agent", string(content))
	assert.NoError(t, validateExecutable(path))
}

func TestResolveAgentBinaryDownloadFailures(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	previous := sHTTPClient
	sHTTPClient = server.Client()
	t.Cleanup(func() { sHTTPClient = previous })

	for name, url := range map[string]string{
		"not found":         server.URL + "/agent",
		"https is required": "http://example.com/agent",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("CDS_CONFIG_PATH", root)
			t.Setenv("CDS_AGENT_DOWNLOAD_URL", url)

			_, err := resolveAgentBinary(remotePlatform)

			require.Error(t, err)
			entries, _ := os.ReadDir(filepath.Join(root, ".xcds", "cache", "agents", cg.Version, "linux-riscv64"))
			assert.Empty(t, entries, "nothing is left in the cache")
		})
	}
}

func TestResolveAgentBinaryNotAvailable(t *testing.T) {
	t.Setenv("CDS_CONFIG_PATH", t.TempDir())

	_, err := resolveAgentBinary(remotePlatform)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "CDS_AGENT_BINARY")
}
