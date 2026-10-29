package cdstls

import (
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cenv"
)

// Certificate paths are resolved on each call so that they follow the active
// configuration directory (CDS_CONFIG_PATH, client vs agent mode) instead of
// being frozen at package initialization.

// CAFilePath returns the CA certificate path of the active configuration directory.
func CAFilePath() string {
	return certFilePath("ca.pem")
}

// AgentServerCertFilePath returns the agent server certificate path of the active configuration directory.
func AgentServerCertFilePath() string {
	return certFilePath("agent-srv.pem")
}

// AgentServerKeyFilePath returns the agent server private key path of the active configuration directory.
func AgentServerKeyFilePath() string {
	return certFilePath("agent-srv-key.pem")
}

// ClientCertFilePath returns the client certificate path of the active configuration directory.
func ClientCertFilePath() string {
	return certFilePath("client.pem")
}

// ClientKeyFilePath returns the client private key path of the active configuration directory.
func ClientKeyFilePath() string {
	return certFilePath("client-key.pem")
}

func certFilePath(filename string) string {
	return filepath.Join(cenv.ConfigDir(kcertsDir), filename)
}
