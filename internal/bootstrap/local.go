package bootstrap

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/config"
	cg "github.com/amadeusitgroup/cds/internal/global"
	cdstls "github.com/amadeusitgroup/cds/internal/tls"
)

var (
	// binaries required to provision the cds PKI
	binaries = []binary{cfsslbin{n: "cfssl"}, cfssljsonbin{n: "cfssljson"}}
)

// agentStarter starts the agent binary listening on port.
type agentStarter func(binary string, port int) error

// fireLocal starts the local agent with start, waits until it is healthy and
// registers it in the CLI config.
func fireLocal(start agentStarter) error {
	for _, bin := range binaries {
		if err := ensureBinary(bin); err != nil {
			return cerr.AppendErrorFmt("Failed to install binary %s", err, bin.name())
		}
	}

	if err := cdstls.EnsureCerts(); err != nil {
		return cerr.AppendError("Failed to provision certificates", err)
	}

	binary, err := resolveAgentBinary(localPlatform())
	if err != nil {
		return err
	}

	port := config.AgentPort()
	clog.Debug(fmt.Sprintf("Starting local agent %s on port %d", binary, port))
	if err := start(binary, port); err != nil {
		return cerr.AppendError("Failed to start agent", err)
	}

	address := agentAddress(cg.KLocalhost, port)
	if err := waitForHealthyAgent(address, cg.KLocalhost); err != nil {
		return err
	}
	return registerAgent(cg.KLocalhost, address)
}
