package command

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/bootstrap"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/containerruntime"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

// detectRuntime is the container-runtime detector used during local host
// registration. It is a package variable so tests can substitute a fake.
var detectRuntime = containerruntime.Detect

type localHostRegistration struct {
	Info              containerruntime.Info
	AlreadyRegistered bool
}

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

	// For a local host, detect the container runtime and register the host as a
	// DevContainer deploy target. Detection failures (no Podman, machine not
	// running) abort registration with an actionable message. The same predicate
	// as bootstrap.StartAgent is used so a given target is treated as local by
	// both the agent launch and the host registration.
	if isLocalTarget(hostName) {
		registration, err := registerLocalHost(hostName)
		if err != nil {
			return err
		}
		ownership, err := bootstrap.StartAgent(hostName)
		if err != nil {
			if !errors.As(err, &bootstrap.StartOnRunError{}) {
				if !registration.AlreadyRegistered && db.HasHost(hostName) {
					db.RemoveHostFromHostList(hostName)
				}
				return err
			}
		}
		if ownership.Manager != cg.EmptyStr {
			if err := db.SetHostAgentOwnership(hostName, ownership); err != nil {
				if stopErr := bootstrap.StopManagedAgent(ownership); stopErr != nil {
					return cerr.AppendError("failed to rollback started local agent after ownership persistence failure", stopErr)
				}
				return err
			}
		}
		message := fmt.Sprintf("Registered local host %q with runtime %s %s",
			hostName, registration.Info.Engine, registration.Info.Version)
		if registration.AlreadyRegistered {
			message = fmt.Sprintf("Local host %q is already registered with runtime %s %s",
				hostName, registration.Info.Engine, registration.Info.Version)
		}
		o := output.FromContext(cmd.Context())
		return output.Render(o, output.SimpleResult{Message: message})
	}

	// TODO: fix::bootstrap, There is a mix of responsibilities between the command and the bootstrap agent regarding host management and registration.
	// Currently the bootstrap package registers the agent in the config. I believe it should be the responsibility of the command to manage the config entries,
	// while the bootstrap package should focus on launching and managing the agent process.
	_, err = bootstrap.StartAgent(hostName)
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

// isLocalTarget reports whether hostName denotes the local machine, matching the
// decision bootstrap.StartAgent makes between fire() and fireRemote().
func isLocalTarget(hostName string) bool {
	return hostName == cg.KLocalhost
}

// registerLocalHost records the local host as a target for DevContainer
// deployment. It is idempotent: an existing host with runtime info is reported
// as already registered instead of re-detecting or duplicating it. The persisted
// state is flushed to disk by the caller (main defers db.Save()).
func registerLocalHost(hostName string) (localHostRegistration, error) {
	if db.HasHost(hostName) {
		runtimeInfo := db.GetHostRuntime(hostName)
		if runtimeInfo.Engine != cg.EmptyStr || runtimeInfo.Version != cg.EmptyStr {
			return localHostRegistration{
				Info: containerruntime.Info{
					Engine:  runtimeInfo.Engine,
					Version: runtimeInfo.Version,
				},
				AlreadyRegistered: true,
			}, nil
		}
	}

	// Return the detection error as-is: it already carries an actionable message,
	// and not wrapping it preserves the typed sentinels (containerruntime.Err*)
	// so callers can branch with errors.Is.
	info, err := detectRuntime()
	if err != nil {
		return localHostRegistration{}, err
	}

	alreadyRegistered := db.HasHost(hostName)
	if !alreadyRegistered {
		db.AddHost(hostName, cenv.GetUsernameFromEnv())
	}

	if err := db.SetHostRuntime(hostName, bo.RuntimeInfo{
		Engine:  info.Engine,
		Version: info.Version,
	}); err != nil {
		return localHostRegistration{}, err
	}

	return localHostRegistration{Info: info, AlreadyRegistered: alreadyRegistered}, nil
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
