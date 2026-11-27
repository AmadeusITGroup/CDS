package command

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/spf13/cobra"
)

type version struct {
	defaultCmd
}

/************************************************************/
/*                                                          */
/*                      Business logic                      */
/*                                                          */
/************************************************************/

func (v *version) initFlags() {
}

func (v *version) initSubCommands() {
	v.subCmds = []baseCmd{}
}

func (v *version) runE(cmd *cobra.Command, args []string) error {
	// biz logic on client side
	clog.Info(fmt.Sprintf("cds CLI version: %s", cg.Version))

	// biz logic on server side
	agentVersion, err := agentVersionOn(cg.KLocalhost)
	if err != nil {
		return cerr.AppendError("Failed to execute version command", err)
	}
	clog.Info(fmt.Sprintf("Agent server version: %s", agentVersion))
	return nil
}

/***********************************************************/
/*                                                         */
/*              Implement `baseCmd` interface              */
/*                                                         */
/***********************************************************/

var _ baseCmd = (*version)(nil)

func (v *version) command() *cobra.Command {
	if v.cmd == nil {
		v.cmd = &cobra.Command{
			Use:           "version",
			Short:         "Print the version number of cds",
			Long:          `All software has versions. This is cds's`,
			Args:          cobra.NoArgs,
			RunE:          v.runE,
			SilenceErrors: true,
			SilenceUsage:  true,
		}
		v.initSubCommands()
		v.initFlags()
	}
	return v.cmd
}

func (v *version) subCommands() []baseCmd {
	return v.subCmds
}

/***********************************************************/
/*                                                         */
/*                    Callback for gRPC                    */
/*                                                         */
/***********************************************************/

// agentVersionOn returns the version of the agent of hostname.
func agentVersionOn(hostname string, options ...func(*callOptions)) (string, error) {
	var version string
	err := stubCallback(func(c agentServices, ctx context.Context) error {
		reply, err := c.info.GetVersion(ctx, &cdspb.GetVersionRequest{})
		if err != nil {
			return cerr.AppendError("Failed to get agent version", err)
		}
		version = reply.GetCurrent()
		return nil
	}).executeOn(hostname, options...)
	return version, err
}

// warnOnVersionMismatch logs a warning when the connected agent runs another
// version than this client. It is read-only and never fails the caller.
func warnOnVersionMismatch(ctx context.Context, c agentServices, hostname string) {
	reply, err := c.info.GetVersion(ctx, &cdspb.GetVersionRequest{})
	if err != nil {
		clog.Debug("Failed to get agent version", err)
		return
	}
	if reply.GetCurrent() != cg.Version {
		clog.Warn(versionMismatchHint(hostname, reply.GetCurrent()))
	}
}

// versionMismatchHint tells how to update an agent of hostname that runs
// version: `orc run` re-bootstraps a remote agent, but only warns for a local
// one, which must be restarted (BD-09, CDS-066).
func versionMismatchHint(hostname, version string) string {
	if hostname == cg.KLocalhost {
		return fmt.Sprintf("Local agent runs version %s instead of %s, restart it to update it", version, cg.Version)
	}
	return fmt.Sprintf("Agent runs version %s instead of %s, run 'cds space orc run' to update it", version, cg.Version)
}
