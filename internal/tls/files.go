package cdstls

import (
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cenv"
)

// CAFilePath returns the active CA certificate path for the current config directory.
func CAFilePath() string {
	return certFilePath("ca.pem")
}

// AgentServerCertFilePath returns the active agent server certificate path for the current config directory.
func AgentServerCertFilePath() string {
	return certFilePath("agent-srv.pem")
}

// AgentServerKeyFilePath returns the active agent server private key path for the current config directory.
func AgentServerKeyFilePath() string {
	return certFilePath("agent-srv-key.pem")
}

// ClientCertFilePath returns the active client certificate path for the current config directory.
func ClientCertFilePath() string {
	return certFilePath("client.pem")
}

// ClientKeyFilePath returns the active client private key path for the current config directory.
func ClientKeyFilePath() string {
	return certFilePath("client-key.pem")
}

func certFilePath(filename string) string {
	return filepath.Join(cenv.ConfigDir(kcertsDir), filename)
}
