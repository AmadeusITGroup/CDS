package bootstrap

import (
	"net"

	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/host"
	"github.com/amadeusitgroup/cds/internal/systemd"
)

func StartAgent(hostname string) error {
	// check if agent is already running
	if isAgentRunning(hostname) {
		clog.Debug("Agent is already running")
		return StartOnRunError{}
	}
	if hostname == cg.KLocalhost {
		return fire()
	}
	return fireRemote(hostname)

}

func isAgentRunning(hostName string) bool {
	server := config.AgentAddress(hostName)
	conn, err := net.Dial("tcp", server)
	if err != nil {
		clog.Debug("Failed to connect to agent", err)
		return false
	}
	defer func() {
		_ = conn.Close()
	}()
	return true
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
