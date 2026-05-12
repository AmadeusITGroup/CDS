package bootstrap

import (
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/host"
	"github.com/amadeusitgroup/cds/internal/systemd"
)

func StartAgent(hostname string) error {
	hostName := strings.TrimSpace(hostname)
	if hostName == cg.EmptyStr {
		hostName = cg.KLocalhost
	}

	// check if agent is already running
	running, address, err := isAgentRunning(hostName)
	if err != nil {
		return cerr.AppendErrorFmt("failed to check for agent running on %s", err, hostName)
	}
	if running {
		if isLocalHost(hostName) {
			if err := registerLocalAgent(address); err != nil {
				return err
			}
		}
		clog.Debug("Agent is already running")
		return StartOnRunError{}
	}
	if isLocalHost(hostName) {
		return fire()
	}
	return fireRemote(hostName)

}

func fireRemote(hostName string) error {
	sysd := systemd.New(systemd.WithTarget(host.New(host.WithName(hostName))))
	if sysd.In() {
		return sysd.StartService()
	}
	return nil
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
