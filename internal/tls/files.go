package cdstls

import (
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cenv"
)

var (
	CAFilePath              = filepath.Join(cenv.ConfigDir(kcertsDir), "ca.pem")
	AgentServerCertFilePath = filepath.Join(cenv.ConfigDir(kcertsDir), "agent-srv.pem")
	AgentServerKeyFilePath  = filepath.Join(cenv.ConfigDir(kcertsDir), "agent-srv-key.pem")
	ClientCertFilePath      = filepath.Join(cenv.ConfigDir(kcertsDir), "client.pem")
	ClientKeyFilePath       = filepath.Join(cenv.ConfigDir(kcertsDir), "client-key.pem")
)
