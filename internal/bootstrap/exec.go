package bootstrap

import (
	"fmt"
	"os/exec"
	"strconv"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

// execAgent starts the agent as a detached process listening on port.
// It does not wait for the agent to exit: the agent is a long running process.
func execAgent(binary string, port int) error {
	cmd := exec.Command(binary, "-port", strconv.Itoa(port))
	cmd.SysProcAttr = detachedProcAttr()

	if err := cmd.Start(); err != nil {
		return cerr.AppendErrorFmt("Failed to start %s", err, binary)
	}

	go func() {
		if err := cmd.Wait(); err != nil {
			clog.Error("Agent process finished with error:", err)
		}
	}()

	clog.Debug(fmt.Sprintf("Agent started with pid: %d", cmd.Process.Pid))
	return nil
}
