package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapRemoteGOArch(t *testing.T) {
	tests := []struct {
		name    string
		machine string
		want    string
		wantErr bool
	}{
		{name: "x86_64", machine: "x86_64", want: "amd64"},
		{name: "amd64", machine: "amd64", want: "amd64"},
		{name: "aarch64", machine: "aarch64", want: "arm64"},
		{name: "arm64", machine: "arm64", want: "arm64"},
		{name: "unsupported", machine: "sparc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapRemoteGOArch(tt.machine)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRequiredRuntimeCertsIncludesClientAndAgentCerts(t *testing.T) {
	t.Setenv("CDS_CONFIG_PATH", "/tmp/testconfig")
	t.Cleanup(cenv.SetConfigDirForClient)
	cenv.SetConfigDirForClient()

	certs := requiredRuntimeCerts()

	assert.Contains(t, certs, filepath.Join("/tmp/testconfig", ".xcds", "certs", "ca.pem"))
	assert.Contains(t, certs, filepath.Join("/tmp/testconfig", ".xcds", "certs", "client.pem"))
	assert.Contains(t, certs, filepath.Join("/tmp/testconfig", ".xcds", "certs", "client-key.pem"))
	assert.Contains(t, certs, filepath.Join("/tmp/testconfig", ".xcds-agent", "certs", "ca.pem"))
	assert.Contains(t, certs, filepath.Join("/tmp/testconfig", ".xcds-agent", "certs", "agent-srv.pem"))
	assert.Contains(t, certs, filepath.Join("/tmp/testconfig", ".xcds-agent", "certs", "agent-srv-key.pem"))
}

func TestAgentAddressParsing(t *testing.T) {
	tests := []struct {
		name       string
		address    string
		wantPort   string
		wantTCP    string
		wantErrTCP bool
	}{
		{name: "port only", address: ":9091", wantPort: "9091", wantTCP: ":9091"},
		{name: "host port", address: "remote.example.com:9092", wantPort: "9092", wantTCP: "remote.example.com:9092"},
		{name: "url", address: "https://remote.example.com:9093", wantPort: "9093", wantTCP: "remote.example.com:9093"},
		{name: "host default port", address: "remote.example.com", wantPort: "8087", wantTCP: "remote.example.com:8087"},
		{name: "empty", address: "", wantPort: "8087", wantErrTCP: true},
		{name: "invalid port", address: "remote.example.com:bad", wantErrTCP: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPort, portErr := agentPort(tt.address)
			if tt.wantPort == "" {
				require.Error(t, portErr)
			} else {
				require.NoError(t, portErr)
				assert.Equal(t, tt.wantPort, gotPort)
			}
			gotTCP, err := agentTCPAddress(tt.address)
			if tt.wantErrTCP {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTCP, gotTCP)
		})
	}
}
