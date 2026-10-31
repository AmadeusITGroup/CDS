package systemd

import (
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

// localHost is a HostOps that runs on the agent host itself: commands execute
// locally and copies are plain file writes. The agent runs on the host it
// manages, so supervising local units (kind node, registry) goes through this
// rather than SSH.
type localHost struct{}

// LocalHost returns a HostOps targeting the agent's own host.
func LocalHost() HostOps { return localHost{} }

func (localHost) Execute(name string, args ...string) (string, error) {
	return shexec.ExecuteCmd(shexec.Execcmd{Name: name, Args: args}, "")
}

// Copy writes localPath into the remotePath directory on the local host,
// mirroring the remote Copy contract (destination is a directory, the file
// keeps its base name).
func (localHost) Copy(localPath, remotePath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return cerr.AppendError("failed to read local file for copy", err)
	}
	if err := os.MkdirAll(remotePath, 0755); err != nil {
		return cerr.AppendErrorFmt("failed to create target directory %s", err, remotePath)
	}
	dst := filepath.Join(remotePath, filepath.Base(localPath))
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return cerr.AppendErrorFmt("failed to write %s", err, dst)
	}
	return nil
}
