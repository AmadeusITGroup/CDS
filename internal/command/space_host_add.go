package command

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bootstrap"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/output"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
	"github.com/spf13/cobra"
)

type spcHostAdd struct {
	defaultCmd
}

var _ baseCmd = (*spcHostAdd)(nil)

func (s *spcHostAdd) initFlags() {
}

func (s *spcHostAdd) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "add <target-server>",
			Short:         "Bootstrap an agent host",
			Args:          cobra.ExactArgs(1),
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
		s.initFlags()
	}
	return s.cmd
}

func (s *spcHostAdd) subCommands() []baseCmd {
	return s.subCmds
}

func (s *spcHostAdd) runE(cmd *cobra.Command, args []string) error {
	hostName, err := bootstrapHostName(args[0])
	if err != nil {
		return err
	}

	if err := ensureAgentRegistered(args[0], hostName); err != nil {
		return err
	}

	err = bootstrap.StartAgent(hostName)
	alreadyRunning := false
	if err != nil {
		if !errors.As(err, &bootstrap.StartOnRunError{}) {
			return err
		}
		alreadyRunning = true
	}

	message := fmt.Sprintf("Bootstrapped host %q", hostName)
	if alreadyRunning {
		message = fmt.Sprintf("Host %q is already running", hostName)
	}

	o := output.FromContext(cmd.Context())
	return output.Render(o, output.SimpleResult{Message: message})
}

func ensureAgentRegistered(targetServer, hostName string) error {
	if _, err := config.AgentAddress(hostName); err == nil {
		return nil
	}

	return config.CreateAgentInConfig(config.NewAgent(
		config.WithTargetAddress(defaultAgentTargetAddress(targetServer, hostName)),
		config.WithAgentTLS(config.NewTlssecret(
			config.WithCA(cdstls.CAFilePath()),
			config.WithCert(cdstls.ClientCertFilePath()),
			config.WithKey(cdstls.ClientKeyFilePath()),
		)),
	))
}

func defaultAgentTargetAddress(targetServer, hostName string) string {
	normalizedTarget := strings.TrimSpace(targetServer)
	if normalizedTarget == cg.EmptyStr || strings.HasPrefix(normalizedTarget, ":") {
		return ":8087"
	}
	if strings.Contains(normalizedTarget, "://") {
		return normalizedTarget
	}
	if _, _, err := net.SplitHostPort(normalizedTarget); err == nil {
		return normalizedTarget
	}
	if hostName == cg.KLocalhost {
		return ":8087"
	}
	return net.JoinHostPort(hostName, "8087")
}

func bootstrapHostName(targetServer string) (string, error) {
	normalizedTarget := strings.TrimSpace(targetServer)
	if normalizedTarget == cg.EmptyStr || strings.HasPrefix(normalizedTarget, ":") {
		return cg.KLocalhost, nil
	}

	if strings.Contains(normalizedTarget, "://") {
		parsedURL, err := url.Parse(normalizedTarget)
		if err != nil {
			return cg.EmptyStr, cerr.NewError("failed to parse target server for bootstrap")
		}
		if parsedURL.Hostname() != cg.EmptyStr {
			return parsedURL.Hostname(), nil
		}
	}

	if host, _, err := net.SplitHostPort(normalizedTarget); err == nil {
		if host == cg.EmptyStr {
			return cg.KLocalhost, nil
		}
		return host, nil
	}

	return normalizedTarget, nil
}
