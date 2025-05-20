package command

import (
	"context"
	"time"

	cdspb "github.com/amadeusitgroup/cds/internal/api/v1"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type stubCallback func(c cdspb.AgentClient, ctx context.Context) error

func (s stubCallback) execute() error {
	addr, err := getAgentServerAddress()
	if err != nil {
		return cerr.AppendError("Failed to get server IP address", err)
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

	c := cdspb.NewAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s(c, ctx)
}

func getAgentServerAddress() (string, error) {
	return "localhost:8087", nil
}
