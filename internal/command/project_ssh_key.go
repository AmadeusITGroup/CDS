package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/db"
)

var defaultSSHKeyNames = []string{"id_rsa", "id_ed25519"}

func projectSSHKeyPair(projectName string) (string, string, error) {
	hostName := projectAgentHost(projectName)
	if privateKeyPath, publicKeyPath := db.GetHostKey(hostName), db.GetHostPubKey(hostName); privateKeyPath != "" && publicKeyPath != "" {
		return privateKeyPath, publicKeyPath, nil
	}

	sshDir := filepath.Join(cenv.GetUserHomeDir(), ".ssh")
	for _, keyName := range defaultSSHKeyNames {
		privateKeyPath := filepath.Join(sshDir, keyName)
		publicKeyPath := privateKeyPath + ".pub"
		if regularFileExists(privateKeyPath) && regularFileExists(publicKeyPath) {
			return privateKeyPath, publicKeyPath, nil
		}
	}
	return "", "", fmt.Errorf("no default SSH key pair found for project %q", projectName)
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
