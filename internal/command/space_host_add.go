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
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/containerruntime"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/output"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
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

	// For a local target, detect and record the container runtime (Podman) so
	// the host is registered as a DevContainer deploy target. Detection failures
	// (no Podman, machine not running) abort with an actionable message.
	runtimeSuffix := ""
	if isLocalTarget(hostName) {
		registration, regErr := registerLocalHost(hostName)
		if regErr != nil {
			return regErr
		}
		if registration.AlreadyRegistered {
			runtimeSuffix = fmt.Sprintf(" (runtime %s %s, already registered)", registration.Info.Engine, registration.Info.Version)
		} else {
			runtimeSuffix = fmt.Sprintf(" (runtime %s %s)", registration.Info.Engine, registration.Info.Version)
		}
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

	message := fmt.Sprintf("Bootstrapped host %q%s", hostName, runtimeSuffix)
	if alreadyRunning {
		message = fmt.Sprintf("Host %q is already running%s", hostName, runtimeSuffix)
	}

	o := output.FromContext(cmd.Context())
	return output.Render(o, output.SimpleResult{Message: message})
}

// isLocalTarget reports whether hostName denotes the local machine, matching the
// decision bootstrap.StartAgent makes between a local and remote agent.
func isLocalTarget(hostName string) bool {
	return hostName == cg.KLocalhost
}

// registerLocalHost records the local host as a target for DevContainer
// deployment. It is idempotent: an existing host with runtime info is reported
// as already registered instead of re-detecting or duplicating it.
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

	// Reaching here means no runtime was registered for this host yet (the
	// early return above handles the already-registered case). The host row may
	// still exist because normalization re-creates it from referencing projects,
	// so add it only when truly absent, but always report a fresh registration.
	if !db.HasHost(hostName) {
		db.AddHost(hostName, cenv.GetUsernameFromEnv())
	}

	if err := db.SetHostRuntime(hostName, bo.RuntimeInfo{
		Engine:  info.Engine,
		Version: info.Version,
	}); err != nil {
		return localHostRegistration{}, err
	}

	return localHostRegistration{Info: info, AlreadyRegistered: false}, nil
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
