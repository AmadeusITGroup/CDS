package command

import (
	"context"
	"errors"
	"time"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bootstrap"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	// kAgentRequestTimeout bounds regular agent calls.
	kAgentRequestTimeout = 30 * time.Second
	// kAgentLongRequestTimeout bounds calls building containers (image pulls, builds...).
	kAgentLongRequestTimeout = 30 * time.Minute
)

type agentServices struct {
	info      cdspb.AgentInfoServiceClient
	container cdspb.ContainerServiceClient
	artifact  cdspb.ArtifactServiceClient
}

type stubCallback func(c agentServices, ctx context.Context) error

type callOptions struct {
	timeout   time.Duration
	autostart bool
}

// withLongTimeout is for calls building containers.
func withLongTimeout() func(*callOptions) {
	return func(o *callOptions) {
		o.timeout = kAgentLongRequestTimeout
	}
}

// withoutAutostart only reaches an agent already running.
func withoutAutostart() func(*callOptions) {
	return func(o *callOptions) {
		o.autostart = false
	}
}

// executeOn calls the agent of hostname, starting it first when needed.
func (s stubCallback) executeOn(hostname string, options ...func(*callOptions)) error {
	opts := callOptions{timeout: kAgentRequestTimeout, autostart: true}
	for _, option := range options {
		option(&opts)
	}

	if opts.autostart {
		if err := ensureAgent(hostname); err != nil {
			return err
		}
	}

	conn, err := dialAgent(hostname)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	c := agentServices{
		info:      cdspb.NewAgentInfoServiceClient(conn),
		container: cdspb.NewContainerServiceClient(conn),
		artifact:  cdspb.NewArtifactServiceClient(conn),
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	return s(c, ctx)
}

// sStartedAgents tracks the agents ensured by the current command.
var sStartedAgents = map[string]bool{}

// ensureAgent starts the agent of hostname unless it is already running. It
// is checked once per command.
func ensureAgent(hostname string) error {
	if sStartedAgents[hostname] {
		return nil
	}
	if err := bootstrap.StartAgent(hostname); err != nil && !errors.As(err, &bootstrap.StartOnRunError{}) {
		return cerr.AppendErrorFmt("Failed to start agent of %s", err, hostname)
	}
	sStartedAgents[hostname] = true
	return nil
}

// dialAgent creates a client of the agent registered for hostname, verifying
// that its certificate is issued for hostname.
func dialAgent(hostname string) (*grpc.ClientConn, error) {
	addr, err := config.AgentAddress(hostname)
	if err != nil {
		return nil, cerr.AppendError("Failed to get agent server address", err)
	}
	agent, err := config.RegisteredAgent(addr)
	if err != nil {
		return nil, cerr.AppendError("Failed to get agent configuration", err)
	}

	tlsFiles := cdstls.TLSConfig{
		CAFile:        agent.Certs.CA,
		CertFile:      agent.Certs.Cert,
		KeyFile:       agent.Certs.Key,
		ServerAddress: hostname,
	}
	if tlsFiles.CAFile == cg.EmptyStr || tlsFiles.CertFile == cg.EmptyStr || tlsFiles.KeyFile == cg.EmptyStr {
		tlsFiles.CAFile, tlsFiles.CertFile, tlsFiles.KeyFile = cdstls.CAFilePath(), cdstls.ClientCertFilePath(), cdstls.ClientKeyFilePath()
	}
	clientTLSConfig, err := cdstls.SetupTLSConfig(tlsFiles)
	if err != nil {
		return nil, cerr.AppendError("Failed to setup TLS config", err)
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(clientTLSConfig)))
	if err != nil {
		return nil, cerr.AppendErrorFmt("Failed to create agent client for %s", err, addr)
	}
	return conn, nil
}
