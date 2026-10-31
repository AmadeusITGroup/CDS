package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRemoteHost answers commands from outputs (keyed by command line) and
// records executed commands and copied files.
type fakeRemoteHost struct {
	outputs  map[string]string
	failures map[string]bool
	executed []string
	copied   map[string]string // remote path -> content
}

func newFakeRemoteHost() *fakeRemoteHost {
	return &fakeRemoteHost{
		outputs: map[string]string{
			"uname -sm":         "Linux x86_64\n",
			"systemd-path user": "/srv/home/alice\n",
		},
		failures: map[string]bool{},
		copied:   map[string]string{},
	}
}

func (f *fakeRemoteHost) Execute(name string, args ...string) (string, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	f.executed = append(f.executed, cmd)
	if f.failures[cmd] {
		return "", errors.New("exit status 1")
	}
	return f.outputs[cmd], nil
}

func (f *fakeRemoteHost) Copy(localPath, remoteDir string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	f.copied[remoteDir+"/"+filepath.Base(localPath)] = string(data)
	return nil
}

const (
	kTestAgentDir = "/srv/home/alice/.cds-agent"
	kTestBinary   = kTestAgentDir + "/bin/" + "%s" + "/cds-api-agent"
)

type bootstrapRecorder struct {
	started            []string
	healthy            []string
	registered         []string
	credentialsChanged []bool
}

func newTestRemoteBootstrap(t *testing.T, h *fakeRemoteHost) (remoteBootstrap, *bootstrapRecorder) {
	t.Helper()
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "cds-api-agent-linux-amd64")
	writeExecutable(t, binary, "agent binary")
	certsDir := t.TempDir()
	for _, file := range []string{"ca.pem", "agent-srv.pem", "agent-srv-key.pem"} {
		require.NoError(t, os.WriteFile(filepath.Join(certsDir, file), []byte(file), 0600))
	}

	rec := &bootstrapRecorder{}
	return remoteBootstrap{
		hostname: "remote.example.com",
		h:        h,
		port:     8087,
		resolveBinary: func(p platform) (string, error) {
			if p != (platform{os: "linux", arch: "amd64"}) {
				return "", errors.New("unexpected platform " + p.String())
			}
			return binary, nil
		},
		agentCertsDir: func(hostname string) (string, error) {
			assert.Equal(t, "remote.example.com", hostname)
			return certsDir, nil
		},
		startService: func(_ remoteHost, binary string, port int, credentialsChanged bool) error {
			rec.started = append(rec.started, binary)
			rec.credentialsChanged = append(rec.credentialsChanged, credentialsChanged)
			assert.Equal(t, 8087, port)
			return nil
		},
		waitHealthy: func(address, serverName string) error {
			rec.healthy = append(rec.healthy, address+" "+serverName)
			return nil
		},
		register: func(hostname, address string) error {
			rec.registered = append(rec.registered, hostname+" "+address)
			return nil
		},
	}, rec
}

func remoteBinaryPath() string {
	return strings.Replace(kTestBinary, "%s", cg.Version, 1)
}

func TestRemoteBootstrapInstallsAndStartsTheAgent(t *testing.T) {
	h := newFakeRemoteHost()
	rb, rec := newTestRemoteBootstrap(t, h)

	require.NoError(t, rb.run())

	binary := remoteBinaryPath()
	assert.Equal(t, "agent binary", h.copied[binary+".upload"], "the binary is uploaded aside")
	assert.Contains(t, h.executed, "chmod 700 "+binary+".upload")
	assert.Contains(t, h.executed, "mv -f "+binary+".upload "+binary)
	assert.Contains(t, h.executed, "chmod 700 "+kTestAgentDir)
	for _, file := range []string{"ca.pem", "agent-srv.pem", "agent-srv-key.pem"} {
		remote := kTestAgentDir + "/certs/" + file
		assert.Equal(t, file, h.copied[remote])
		assert.Contains(t, h.executed, "chmod 600 "+remote)
	}
	assert.Equal(t, []string{binary}, rec.started)
	assert.Equal(t, []string{"remote.example.com:8087 remote.example.com"}, rec.healthy)
	assert.Equal(t, []string{"remote.example.com remote.example.com:8087"}, rec.registered)
}

func TestRemoteBootstrapSkipsUpToDateBinary(t *testing.T) {
	h := newFakeRemoteHost()
	rb, rec := newTestRemoteBootstrap(t, h)
	local, err := rb.resolveBinary(platform{os: "linux", arch: "amd64"})
	require.NoError(t, err)
	sum, err := sha256File(local)
	require.NoError(t, err)
	h.outputs["sh -c "+sha256Probe(remoteBinaryPath())] = sum + "  " + remoteBinaryPath() + "\n"

	require.NoError(t, rb.run())

	assert.NotContains(t, h.copied, remoteBinaryPath()+".upload")
	assert.Equal(t, []string{remoteBinaryPath()}, rec.started)
}

func TestRemoteBootstrapReplacesDifferentBinary(t *testing.T) {
	h := newFakeRemoteHost()
	h.outputs["sh -c "+sha256Probe(remoteBinaryPath())] = "0000  " + remoteBinaryPath() + "\n"
	rb, _ := newTestRemoteBootstrap(t, h)

	require.NoError(t, rb.run())

	assert.Contains(t, h.copied, remoteBinaryPath()+".upload")
}

func TestRemoteBootstrapPlatformProbe(t *testing.T) {
	for name, tc := range map[string]struct {
		uname   string
		want    platform
		wantErr bool
	}{
		"linux amd64":       {uname: "Linux x86_64\n", want: platform{os: "linux", arch: "amd64"}},
		"linux arm64":       {uname: "Linux aarch64\n", want: platform{os: "linux", arch: "arm64"}},
		"unsupported arch":  {uname: "Linux s390x\n", wantErr: true},
		"unsupported os":    {uname: "Darwin arm64\n", wantErr: true},
		"unexpected output": {uname: "\n", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFakeRemoteHost()
			h.outputs["uname -sm"] = tc.uname
			rb, _ := newTestRemoteBootstrap(t, h)

			p, err := rb.probePlatform()
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, p)
		})
	}
}

func TestRemoteBootstrapRestartsWhenCredentialsChange(t *testing.T) {
	// on first install the host has no certs, so the probe reports no match and
	// the upload is a change: the service must be (re)started to load them (P2)
	h := newFakeRemoteHost()
	rb, rec := newTestRemoteBootstrap(t, h)

	require.NoError(t, rb.run())
	require.Len(t, rec.credentialsChanged, 1)
	assert.True(t, rec.credentialsChanged[0], "freshly uploaded certificates force a restart")
}

func TestRemoteBootstrapStopsOnFailure(t *testing.T) {
	t.Run("unhealthy agent is not registered", func(t *testing.T) {
		rb, rec := newTestRemoteBootstrap(t, newFakeRemoteHost())
		rb.waitHealthy = func(string, string) error { return errors.New("unhealthy") }

		assert.Error(t, rb.run())
		assert.Empty(t, rec.registered)
	})

	t.Run("service start failure", func(t *testing.T) {
		rb, rec := newTestRemoteBootstrap(t, newFakeRemoteHost())
		rb.startService = func(remoteHost, string, int, bool) error { return errors.New("no systemd") }

		assert.Error(t, rb.run())
		assert.Empty(t, rec.registered)
	})

	t.Run("relative home directory", func(t *testing.T) {
		h := newFakeRemoteHost()
		h.outputs["systemd-path user"] = "home\n"
		rb, rec := newTestRemoteBootstrap(t, h)

		assert.Error(t, rb.run())
		assert.Empty(t, rec.started)
	})
}
