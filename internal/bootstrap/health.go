package bootstrap

import (
	"context"
	"fmt"
	"net"
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

const (
	kHealthCheckTimeout = 2 * time.Second
	kAgentStartTimeout  = 30 * time.Second
	kHealthCheckPeriod  = 200 * time.Millisecond
)

// agentVersion returns the version of the agent listening at address, after a
// mutual TLS handshake verifying that its certificate is issued for serverName,
// using the given TLS material.
func agentVersion(address, serverName string, tlsFiles cdstls.TLSConfig) (string, error) {
	tlsFiles.ServerAddress = serverName
	tlsConfig, err := cdstls.SetupTLSConfig(tlsFiles)
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("failed to setup agent TLS config", err)
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("failed to create agent client for %s", err, address)
	}
	defer func() {
		_ = conn.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), kHealthCheckTimeout)
	defer cancel()
	reply, err := cdspb.NewAgentInfoServiceClient(conn).GetVersion(ctx, &cdspb.GetVersionRequest{})
	if err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("agent at %s is not healthy", err, address)
	}
	return reply.GetCurrent(), nil
}

// provisioningTLS is the client's default CA/certificate/key. A freshly
// bootstrapped agent presents a server certificate issued from this CA, so
// readiness after provisioning must use these credentials — not a possibly
// stale registration, which would prevent reaching the agent to update it.
func provisioningTLS() cdstls.TLSConfig {
	return cdstls.TLSConfig{
		CAFile:   cdstls.CAFilePath(),
		CertFile: cdstls.ClientCertFilePath(),
		KeyFile:  cdstls.ClientKeyFilePath(),
	}
}

// registeredAgentTLS returns the CA/certificate/key registered for the agent of
// hostname. It falls back to the client's default provisioning certificates
// when the agent is not registered or its registration omits TLS material.
func registeredAgentTLS(hostname string) cdstls.TLSConfig {
	defaults := provisioningTLS()
	address, err := config.AgentAddress(hostname)
	if err != nil || address == cg.EmptyStr {
		return defaults
	}
	agent, err := config.RegisteredAgent(address)
	if err != nil {
		return defaults
	}
	if agent.Certs.CA == cg.EmptyStr || agent.Certs.Cert == cg.EmptyStr || agent.Certs.Key == cg.EmptyStr {
		return defaults
	}
	return cdstls.TLSConfig{CAFile: agent.Certs.CA, CertFile: agent.Certs.Cert, KeyFile: agent.Certs.Key}
}

// waitForHealthyAgent waits until the agent just provisioned at address answers
// and reports the version of this CLI. It uses the provisioning credentials
// (the identity the fresh agent presents), so a stale registration cannot keep
// readiness from reaching the agent before the registration is updated.
// Requiring the version (not merely a response) ensures bootstrap does not
// report success while an old agent binary is still running.
func waitForHealthyAgent(address, serverName string) error {
	deadline := time.Now().Add(kAgentStartTimeout)
	for {
		version, err := agentVersion(address, serverName, provisioningTLS())
		if err == nil {
			if version == cg.Version {
				return nil
			}
			err = cerr.NewError(fmt.Sprintf("agent runs version %s instead of %s", version, cg.Version))
		}
		if time.Now().After(deadline) {
			return cerr.AppendErrorFmt("agent at %s did not become healthy within %s", err, address, kAgentStartTimeout)
		}
		time.Sleep(kHealthCheckPeriod)
	}
}

// isAgentRunning tells whether the agent registered for hostname is healthy.
// Remote agents must also run the version of the CLI, otherwise they are
// bootstrapped again.
func isAgentRunning(hostname string) (bool, error) {
	address, err := config.AgentAddress(hostname)
	if err != nil {
		if config.IsAgentNotFound(err) {
			return false, nil
		}
		return false, cerr.AppendError("failed to resolve agent address", err)
	}
	if address == cg.EmptyStr {
		return false, nil
	}

	version, err := agentVersion(address, hostname, registeredAgentTLS(hostname))
	if err != nil {
		clog.Debug(fmt.Sprintf("Agent of %s is not running", hostname), err)
		return false, nil
	}
	if version != cg.Version {
		if hostname != cg.KLocalhost {
			clog.Info(fmt.Sprintf("Agent of %s runs version %s instead of %s, it will be updated", hostname, version, cg.Version))
			return false, nil
		}
		clog.Warn(fmt.Sprintf("Local agent runs version %s instead of %s, restart it to update it", version, cg.Version))
	}
	return true, nil
}

// registerAgent registers, or updates, the agent of hostname in the CLI config.
func registerAgent(hostname, address string) error {
	if err := config.UpsertAgentForHostInConfig(hostname, config.NewAgent(
		config.WithTargetAddress(address),
		config.WithAgentTLS(
			config.NewTlssecret(
				config.WithCA(cdstls.CAFilePath()),
				config.WithCert(cdstls.ClientCertFilePath()),
				config.WithKey(cdstls.ClientKeyFilePath())),
		),
	)); err != nil {
		return cerr.AppendErrorFmt("failed to register agent of %s in CLI config", err, hostname)
	}
	return nil
}

func agentAddress(hostname string, port int) string {
	return net.JoinHostPort(hostname, fmt.Sprint(port))
}
