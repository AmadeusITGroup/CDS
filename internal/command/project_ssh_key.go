package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

var defaultSSHKeyNames = []string{"id_rsa", "id_ed25519"}

// projectSSHKeyPair returns the private/public key paths used to connect to a
// project's container. It ensures a key pair exists, generating a CDS-managed
// one when the user has none, so `project ssh`/`rsh` always have a usable key.
func projectSSHKeyPair(projectName string) (string, string, error) {
	return ensureProjectSSHKeyPair(projectName)
}

// ensureProjectSSHKeyPair resolves the SSH key pair for a project's host,
// generating and persisting a CDS-managed pair when none is available. The same
// pair is installed into the container at deploy time and used to connect
// afterwards, so both sides always agree on a single key.
func ensureProjectSSHKeyPair(projectName string) (string, string, error) {
	hostName := projectAgentHost(projectName)

	// 1. A key pair already registered for this host.
	if privateKeyPath, publicKeyPath := db.GetHostKey(hostName), db.GetHostPubKey(hostName); privateKeyPath != "" && publicKeyPath != "" &&
		regularFileExists(privateKeyPath) && regularFileExists(publicKeyPath) {
		return privateKeyPath, publicKeyPath, nil
	}

	// 2. A default user key pair under ~/.ssh, if present. Persist it so the
	//    deploy and connect paths agree on the same pair for this host.
	sshDir := filepath.Join(cenv.GetUserHomeDir(), ".ssh")
	for _, keyName := range defaultSSHKeyNames {
		privateKeyPath := filepath.Join(sshDir, keyName)
		publicKeyPath := privateKeyPath + ".pub"
		if regularFileExists(privateKeyPath) && regularFileExists(publicKeyPath) {
			if err := persistHostKeyPair(hostName, privateKeyPath, publicKeyPath); err != nil {
				return "", "", err
			}
			return privateKeyPath, publicKeyPath, nil
		}
	}

	// 3. A CDS-managed key pair, generated on first use for this host.
	keyPair, err := ensureManagedHostKeyPair(hostName)
	if err != nil {
		return "", "", err
	}
	if err := persistHostKeyPair(hostName, keyPair.PathToPrv, keyPair.PathToPub); err != nil {
		return "", "", err
	}
	return keyPair.PathToPrv, keyPair.PathToPub, nil
}

// ensureManagedHostKeyPair returns the CDS-managed key pair for a host, creating
// it on first use. Mirrors the shared-key generate-if-missing flow.
func ensureManagedHostKeyPair(hostName string) (shexec.KeyPair, error) {
	keyPair := managedHostKeyPairPaths(hostName)
	if regularFileExists(keyPair.PathToPrv) {
		publicKeyPath, err := shexec.GeneratePublicKey(keyPair.PathToPrv)
		if err != nil {
			return shexec.KeyPair{}, err
		}
		keyPair.PathToPub = publicKeyPath
		return keyPair, nil
	}
	generated, err := shexec.GenerateKeyPair(managedHostKeySuffix(hostName))
	if err != nil {
		return shexec.KeyPair{}, cerr.AppendError("Failed to generate SSH key pair", err)
	}
	return generated, nil
}

func managedHostKeyPairPaths(hostName string) shexec.KeyPair {
	suffix := managedHostKeySuffix(hostName)
	sshDir := filepath.Join(cenv.GetUserHomeDir(), ".ssh")
	return shexec.KeyPair{
		PathToPrv: filepath.Join(sshDir, fmt.Sprintf("id_rsa_%s", suffix)),
		PathToPub: filepath.Join(sshDir, fmt.Sprintf("id_rsa_%s.pub", suffix)),
	}
}

func managedHostKeySuffix(hostName string) string {
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, hostName)
	if strings.Trim(sanitized, "_-") == "" {
		return "cds_host"
	}
	return "cds_" + sanitized
}

func persistHostKeyPair(hostName, privateKeyPath, publicKeyPath string) error {
	if !db.HasHost(hostName) {
		db.AddHost(hostName, cenv.GetUsernameFromEnv())
	}
	return db.UpdateHostKey(bo.Host{
		Name:    hostName,
		KeyPair: bo.KeyPair{PathToPrv: privateKeyPath, PathToPub: publicKeyPath},
	})
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
