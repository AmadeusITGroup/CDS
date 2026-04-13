package command

import (
	"context"
	"time"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type agentServices struct {
	info      cdspb.AgentInfoServiceClient
	container cdspb.ContainerServiceClient
}

type stubCallback func(c agentServices, ctx context.Context) error

func (s stubCallback) execute() error {
	addr, err := getAgentServerAddress()
	if err != nil {
		return cerr.AppendError("Failed to get agent server address", err)
	}

	clientTLSConfig, errTLS := cdstls.SetupTLSConfig(cdstls.TLSConfig{CAFile: cdstls.CAFilePath,
		CertFile: cdstls.ClientCertFilePath,
		KeyFile:  cdstls.ClientKeyFilePath,
	})
	if errTLS != nil {
		return cerr.AppendError("Failed to setup TLS config", errTLS)
	}

	clientCreds := credentials.NewTLS(clientTLSConfig)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(clientCreds))
	if err != nil {
		clog.Error("Failed to connect", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	c := agentServices{
		info:      cdspb.NewAgentInfoServiceClient(conn),
		container: cdspb.NewContainerServiceClient(conn),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s(c, ctx)
}

func getAgentServerAddress() (string, error) {
	addr, err := config.AgentAddress(cg.KLocalhost)
	if err != nil {
		return cg.EmptyStr, err
	}
	if addr == cg.EmptyStr {
		return cg.EmptyStr, cerr.NewError("localhost agent target server is not configured")
	}
	return addr, nil
}
