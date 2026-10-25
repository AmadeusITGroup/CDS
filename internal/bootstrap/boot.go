package bootstrap

import (
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// kAgentBinaryName is the name of the agent binary, as built by `make build-api-agent`.
const kAgentBinaryName = "cds-api-agent"

// StartAgent makes sure the agent of hostname is running, starting (and for
// remote hosts installing) it when needed. It returns StartOnRunError when the
// agent was already running.
func StartAgent(hostname string) error {
	running, err := isAgentRunning(hostname)
	if err != nil {
		return cerr.AppendErrorFmt("failed to check for agent running on %s", err, hostname)
	}
	if running {
		clog.Debug("Agent is already running")
		return StartOnRunError{}
	}
	if hostname == cg.KLocalhost {
		return fire()
	}
	return fireRemote(hostname)
}

/************************************************************/
/*                                                          */
/*                 boot errors management                   */
/*                                                          */
/************************************************************/

type StartOnRunError struct{}

func (s StartOnRunError) Error() string {
	return "Agent is already running"
}

// func dummyAuthForAr() {
// 	a := authmgr.New(
// 		authmgr.WithLogin("dummy"),
// 		authmgr.WithPrompt(authmgr.DefaultPrompt()),
// 	)
// 	ar.SetAuthenticationHandler(a)
// 	ar.SetTokenHandler(a)

// }
