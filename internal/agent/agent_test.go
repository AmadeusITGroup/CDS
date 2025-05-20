package agent

import (
	"context"
	"net"
	"testing"

	cdspb "github.com/amadeusitgroup/cds/internal/api/v1"
	"github.com/amadeusitgroup/cds/internal/core"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/emptypb"
)

func newMock() commandManager {
	// add initialization here
	return &mock{}
}

type mock struct {
}

func (m mock) Version() string {
	return "99.99.99"
}

func (m mock) Space() map[string]core.Cmd {
	// TODO Implement
	return map[string]core.Cmd{}
}

func (m mock) Project() map[string]core.Cmd {
	// TODO Implement
	return map[string]core.Cmd{}
}

func TestAgent(t *testing.T) {
	for usecase, fn := range map[string]func(t *testing.T, client cdspb.AgentClient, config *bom){
		"get server version succeeds": testServerVersion,
	} {
		t.Run(usecase, func(t *testing.T) {
			client, config, teardown := setupTest(t, nil)
			defer teardown()
			fn(t, client, config)
		})
	}
}

func setupTest(t *testing.T, fn func(*bom)) (client cdspb.AgentClient, cfg *bom, teardown func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)

	// Configure the client’s TLS credentials to use our self-signed CA as the client’s Root CA (the CA it will use to verify the server).
	// Set the client to use those credentials for its connection.
	clientTLSConfig, err := cdstls.SetupTLSConfig(cdstls.TLSConfig{CAFile: cdstls.CAFilePath,
		// Settings the following two attributes is needed for mutual TLS authentication (server authenticates the client,
		// on top of the default authentication where the client authenticates the server).
		CertFile: cdstls.ClientCertFilePath,
		KeyFile:  cdstls.ClientKeyFilePath,
	})
	assert.NoError(t, err)

	clientCreds := credentials.NewTLS(clientTLSConfig)
	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(clientCreds /*insecure.NewCredentials()*/),
	)
	assert.NoError(t, err)

	// Configure the Agent TLS and enable it to handle TLS connections.
	agentTLSConfig, err := cdstls.SetupTLSConfig(cdstls.TLSConfig{
		CertFile:      cdstls.AgentServerCertFilePath,
		KeyFile:       cdstls.AgentServerKeyFilePath,
		CAFile:        cdstls.CAFilePath,
		ServerAddress: lis.Addr().String(),
		Server:        true, // Setting Server attribute to true enable authentication of clients at server side. Mutual TLS authentication use case
	})
	agentCreds := credentials.NewTLS(agentTLSConfig)
	require.NoError(t, err)

	cfg = &bom{manager: newMock()}
	if fn != nil {
		fn(cfg)
	}
	server, err := NewAgent(cfg, grpc.Creds(agentCreds))
	assert.NoError(t, err)

	// Serve blocks, needs to be run in its own goroutine
	go func() {
		err := server.Serve(lis)
		require.NoError(t, err)
	}()

	client = cdspb.NewAgentClient(conn)

	return client, cfg, func() {
		server.Stop()
		_ = conn.Close()
		_ = lis.Close()
	}
}

func testServerVersion(t *testing.T, client cdspb.AgentClient, config *bom) {
	ctx := context.Background()
	reply, err := client.GetVersion(ctx, &emptypb.Empty{})
	assert.NoError(t, err)
	want := "9.9.9"
	assert.Equal(t, want, reply.GetCurrent(), "they should be equal")

}
