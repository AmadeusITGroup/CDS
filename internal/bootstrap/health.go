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

// agentVersion returns the version of the agent listening at address, after
// a mutual TLS handshake verifying that its certificate is issued for serverName.
func agentVersion(address, serverName string) (string, error) {
	tlsConfig, err := cdstls.SetupTLSConfig(cdstls.TLSConfig{
		CAFile:        cdstls.CAFilePath(),
		CertFile:      cdstls.ClientCertFilePath(),
		KeyFile:       cdstls.ClientKeyFilePath(),
		ServerAddress: serverName,
	})
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

// waitForHealthyAgent waits until the agent at address answers, see agentVersion.
func waitForHealthyAgent(address, serverName string) error {
	deadline := time.Now().Add(kAgentStartTimeout)
	for {
		_, err := agentVersion(address, serverName)
		if err == nil {
			return nil
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

	version, err := agentVersion(address, hostname)
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
