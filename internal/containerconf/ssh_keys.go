package containerconf

import (
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// kDefaultSSHKeyNames are the user key pairs, in order of preference, whose
// public key is installed in containers to access them through SSH.
var kDefaultSSHKeyNames = []string{"id_rsa", "id_ed25519"}

// DefaultSSHKeyPair returns the private and public key paths of the user key
// pair used to access containers, if any.
func DefaultSSHKeyPair() (privateKey, publicKey string, found bool) {
	for _, name := range kDefaultSSHKeyNames {
		privateKey = filepath.Join(cenv.GetUserHomeDir(), ".ssh", name)
		publicKey = privateKey + ".pub"
		if isRegularFile(privateKey) && isRegularFile(publicKey) {
			return privateKey, publicKey, true
		}
	}
	return cg.EmptyStr, cg.EmptyStr, false
}

func isRegularFile(path string) bool {
	info, err := cos.Fs.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
