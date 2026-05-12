package command

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type agentServices struct {
	info      cdspb.AgentInfoServiceClient
	container cdspb.ContainerServiceClient
	artifact  cdspb.ArtifactServiceClient
}

const agentRequestTimeout = 30 * time.Minute

type stubCallback func(c agentServices, ctx context.Context) error

func (s stubCallback) execute() error {
	return s.executeForHost(cg.KLocalhost)
}

func (s stubCallback) executeForHost(hostName string) error {
	conn, ctx, cleanup, err := connectAgentServices(hostName)
	if err != nil {
		return err
	}
	defer cleanup()
	return s(conn, ctx)
}

func connectAgentServices(hostName string) (agentServices, context.Context, func(), error) {
	target, err := getAgentTarget(hostName)
	if err != nil {
		return agentServices{}, nil, nil, cerr.AppendError("Failed to get agent target", err)
	}

	clientTLSConfig, errTLS := cdstls.SetupTLSConfig(cdstls.TLSConfig{
		CAFile:        target.caFile,
		CertFile:      target.certFile,
		KeyFile:       target.keyFile,
		ServerAddress: target.serverName,
	})
	if errTLS != nil {
		return agentServices{}, nil, nil, cerr.AppendError("Failed to setup TLS config", errTLS)
	}

	clientCreds := credentials.NewTLS(clientTLSConfig)
	conn, err := grpc.NewClient(target.address, grpc.WithTransportCredentials(clientCreds))
	if err != nil {
		return agentServices{}, nil, nil, cerr.AppendError("Failed to create agent gRPC client", err)
	}

	cleanup := func() {
		_ = conn.Close()
	}

	c := agentServices{
		info:      cdspb.NewAgentInfoServiceClient(conn),
		container: cdspb.NewContainerServiceClient(conn),
		artifact:  cdspb.NewArtifactServiceClient(conn),
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentRequestTimeout)
	return c, ctx, func() {
		cancel()
		cleanup()
	}, nil
}

func getAgentServerAddress() (string, error) {
	return getAgentServerAddressForHost(cg.KLocalhost)
}

func getAgentServerAddressForHost(hostName string) (string, error) {
	normalizedHost := normalizeAgentHostName(hostName)
	addr, err := config.AgentAddress(normalizedHost)
	if err != nil {
		return cg.EmptyStr, err
	}
	if addr == cg.EmptyStr {
		return cg.EmptyStr, cerr.NewError("agent target server is not configured")
	}
	return addr, nil
}

type agentTarget struct {
	hostName   string
	address    string
	caFile     string
	certFile   string
	keyFile    string
	serverName string
}

func getAgentTarget(hostName string) (agentTarget, error) {
	normalizedHost := normalizeAgentHostName(hostName)
	addr, err := getAgentServerAddressForHost(normalizedHost)
	if err != nil {
		return agentTarget{}, err
	}

	registeredAgent, err := config.RegisteredAgent(addr)
	if err != nil {
		return agentTarget{}, err
	}

	target := agentTarget{
		hostName:   normalizedHost,
		address:    addr,
		caFile:     registeredAgent.Certs.CA,
		certFile:   registeredAgent.Certs.Cert,
		keyFile:    registeredAgent.Certs.Key,
		serverName: agentServerName(addr),
	}
	if target.caFile == cg.EmptyStr {
		target.caFile = cdstls.CAFilePath
	}
	if target.certFile == cg.EmptyStr {
		target.certFile = cdstls.ClientCertFilePath
	}
	if target.keyFile == cg.EmptyStr {
		target.keyFile = cdstls.ClientKeyFilePath
	}
	return target, nil
}

func normalizeAgentHostName(hostName string) string {
	normalized := strings.TrimSpace(hostName)
	if normalized == cg.EmptyStr {
		return cg.KLocalhost
	}
	return normalized
}

func agentServerName(_ string) string {
	// The generated agent server certificate is issued for localhost. Remote
	// agents are contacted by address, but still present that certificate.
	return cg.KLocalhost
}

func addressHost(address string) string {
	normalized := strings.TrimSpace(address)
	if normalized == cg.EmptyStr || strings.HasPrefix(normalized, ":") {
		return cg.KLocalhost
	}
	if strings.Contains(normalized, "://") {
		parsedURL, err := url.Parse(normalized)
		if err == nil {
			return parsedURL.Hostname()
		}
	}
	if host, _, err := net.SplitHostPort(normalized); err == nil {
		if host == cg.EmptyStr {
			return cg.KLocalhost
		}
		return host
	}
	return normalized
}
