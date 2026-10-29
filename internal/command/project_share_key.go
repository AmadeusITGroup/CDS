package command

import (
	"fmt"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

// A project is shared by installing in its container the public key of a key
// pair dedicated to the project, whose private key is handed over.

func projectSharedKeySuffix(projectName string) string {
	return "cds_share_" + strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, projectName)
}

func projectSharedKeyPair(projectName string) shexec.KeyPair {
	return shexec.KeyPairPaths(projectSharedKeySuffix(projectName))
}

// ensureProjectSharedKeyPair returns the shared key pair of the project,
// generated when missing.
func ensureProjectSharedKeyPair(projectName string) (shexec.KeyPair, error) {
	keyPair := projectSharedKeyPair(projectName)
	if cos.Exists(keyPair.PathToPrv) && cos.Exists(keyPair.PathToPub) {
		return keyPair, nil
	}
	keyPair, err := shexec.GenerateKeyPair(projectSharedKeySuffix(projectName))
	if err != nil {
		return shexec.KeyPair{}, cerr.AppendErrorFmt("Failed to generate the shared key pair of project %s", err, projectName)
	}
	return keyPair, nil
}

func readPublicKey(keyPair shexec.KeyPair) (string, error) {
	publicKey, err := cos.ReadFile(keyPair.PathToPub)
	if err != nil {
		return "", cerr.AppendErrorFmt("Failed to read shared public key %s", err, keyPair.PathToPub)
	}
	return strings.TrimSpace(string(publicKey)), nil
}

func removeProjectSharedKeyPair(keyPair shexec.KeyPair) {
	for _, path := range []string{keyPair.PathToPrv, keyPair.PathToPub} {
		if err := cos.Fs.Remove(path); err != nil && cos.Exists(path) {
			clog.Warn(fmt.Sprintf("Failed to remove shared key file %s", path), err)
		}
	}
}
