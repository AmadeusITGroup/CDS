package authmgr

import (
	"github.com/amadeusitgroup/cds/internal/shexec"
)

// WireShexecPasswordPrompt connects shexec's interactive credential callbacks
// to authmgr's password prompt. It must be called once during CLI startup so
// that SSH password authentication (e.g. the first `cds project run` against a
// host reachable only by LDAP password) can prompt the user interactively
// instead of dereferencing unregistered callbacks.
//
// The password getter prompts the user (interactively on a TTY, or reads a
// line from stdin otherwise). The validation callback is a no-op that echoes
// the reported state back, which lets shexec continue its retry loop.
func WireShexecPasswordPrompt() {
	shexec.RegisterShexecCredentialCallbacks(
		func(message string) (string, error) {
			return password(message)
		},
		func(valid bool) bool {
			return valid
		},
	)
}
