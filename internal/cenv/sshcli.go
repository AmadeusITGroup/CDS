package cenv

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// wrapper over EnsureFile, if given an empty list, it'll default to $HOME/.ssh/config
func EnsureSSHClientConfig(paths []string) error {
	if paths == nil {
		paths = append(paths, sshDefaultConfigPath(), sshDefaultKnownHostPath())
	}

	for _, path := range paths {
		if err := EnsureFile(path, cg.KPermFile); err != nil {
			return cerr.AppendError(fmt.Sprintf("Failed to ensure presence of ssh config at '%s'", path), err)
		}
	}

	return nil
}

func GetUserHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		clog.Error("Failed to determine user home directory !", err)
	}
	return homeDir
}

func sshDefaultConfigPath() string {
	return filepath.Join(GetUserHomeDir(), ".ssh", "config")
}

func sshDefaultKnownHostPath() string {
	return filepath.Join(GetUserHomeDir(), ".ssh", "known_hosts")
}
