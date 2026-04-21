package command

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bootstrap"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/output"
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

	// TODO: fix::bootstrap, There is a mix of responsibilities between the command and the bootstrap agent regarding host management and registration.
	// Currently the bootstrap package registers the agent in the config. I believe it should be the responsibility of the command to manage the config entries,
	// while the bootstrap package should focus on launching and managing the agent process.
	err = bootstrap.StartAgent(hostName)
	alreadyRunning := false
	if err != nil {
		if !errors.As(err, &bootstrap.StartOnRunError{}) {
			return err
		}
		alreadyRunning = true
	}

	// TODO: Persist or refresh the CLI host entry once bootstrap exposes the resolved endpoint and credentials again.
	// TODO: Honor the full --target-server value during registration instead of only deriving the bootstrap hostname.
	// TODO: Reintroduce local port selection once bootstrap accepts launch options directly.

	message := fmt.Sprintf("Bootstrapped host %q", hostName)
	if alreadyRunning {
		message = fmt.Sprintf("Host %q is already running", hostName)
	}

	o := output.FromContext(cmd.Context())
	return output.Render(o, output.SimpleResult{Message: message})
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
