package bootstrap

import (
	"net"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/host"
	"github.com/amadeusitgroup/cds/internal/systemd"
)

func StartAgent(hostname string) error {
	// check if agent is already running
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

func isAgentRunning(hostName string) (bool, error) {
	server, err := config.AgentAddress(hostName)
	if err != nil {
		return false, cerr.AppendError("failed to resolve agent address", err)
	}
	if server == cg.EmptyStr {
		return false, nil
	}
	conn, err := net.Dial("tcp", server)
	if err != nil {
		clog.Debug("Failed to connect to agent", err)
		return false, nil
	}
	defer func() {
		_ = conn.Close()
	}()
	return true, nil
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
