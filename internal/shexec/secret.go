package shexec

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

var (
	getterOnShexecPassword     func(string) (string, error)
	validationOnShexecPassword func(bool) bool
)

func RegisterShexecCredentialCallbacks(
	passwordCallback func(string) (string, error),
	validationCallback func(bool) bool,
) {
	getterOnShexecPassword = passwordCallback
	validationOnShexecPassword = validationCallback
}

func retryableGetSecret(hostname string) func() (string, error) {
	authMethod := func() (string, error) {
		if passwordTries == maxPasswordTries {
			return "", cerr.NewError("Maximum amount of authentication tries exceeded for SSH connection")
		}

		if passwordTries > 0 && !secretValidated {
			clog.Error(fmt.Sprintf("Failed to connect with ssh to %v:22, , try re-entering your password (%v tries left)", hostname, maxPasswordTries-passwordTries))
			canContinue := validationOnShexecPassword(false)
			if !canContinue {
				return "", cerr.NewError("Password authentication cannot proceed further")
			}
		}

		var err error = nil
		if len(secret) == 0 || !secretValidated {
			secret, err = getterOnShexecPassword("Enter your office LDAP user password (Hint: windows account password):")

			if err != nil {
				err = cerr.AppendError("Unable to read password", err)
			}

			passwordTries++
		}

		return secret, err
	}

	return authMethod
}
