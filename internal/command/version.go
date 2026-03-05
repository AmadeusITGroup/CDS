package command

import (
	"context"
	"fmt"

	cdspb "github.com/amadeusitgroup/cds/internal/api/v1"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	// override version during build with ldflags, eg:
	// go build -ldflags="-X github.com/amadeusitgroup/cds/commands.appVersion=0.0.1"
	appVersion string = "develop"
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
	clog.Info(fmt.Sprintf("cds CLI version: %s", appVersion))

	// biz logic on server side
	if err := agentVersion.execute(); err != nil {
		return cerr.AppendError("Failed to execute version command", err)
	}
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

var agentVersion stubCallback = func(c cdspb.AgentClient, ctx context.Context) error {
	reply, err := c.GetVersion(ctx, &emptypb.Empty{})

	if err != nil {
		return cerr.NewError("Failed to get agent version")
	}

	clog.Info(fmt.Sprintf("Agent server version: %s", reply.GetCurrent()))
	return nil
}
