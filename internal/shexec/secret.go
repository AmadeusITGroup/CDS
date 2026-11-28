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

// promptForSecret invokes the registered password getter. If no getter was
// registered it returns an error instead of panicking, so a mis-wired build
// fails cleanly rather than with a nil-pointer dereference.
func promptForSecret(message string) (string, error) {
	if getterOnShexecPassword == nil {
		return "", cerr.NewError("no password prompt is registered; cannot read SSH credentials interactively")
	}
	return getterOnShexecPassword(message)
}

// notifySecretValidation reports password-validation state through the
// registered validation callback, tolerating an unregistered callback.
func notifySecretValidation(valid bool) bool {
	if validationOnShexecPassword == nil {
		return valid
	}
	return validationOnShexecPassword(valid)
}

func retryableGetSecret(hostname string) func() (string, error) {
	authMethod := func() (string, error) {
		if passwordTries == maxPasswordTries {
			return "", cerr.NewError("Maximum amount of authentication tries exceeded for SSH connection")
		}

		if passwordTries > 0 && !secretValidated {
			clog.Error(fmt.Sprintf("Failed to connect with ssh to %v:22, , try re-entering your password (%v tries left)", hostname, maxPasswordTries-passwordTries))
			canContinue := notifySecretValidation(false)
			if !canContinue {
				return "", cerr.NewError("Password authentication cannot proceed further")
			}
		}

		var err error = nil
		if len(secret) == 0 || !secretValidated {
			secret, err = promptForSecret("Enter your office LDAP user password (Hint: windows account password):")

			if err != nil {
				err = cerr.AppendError("Unable to read password", err)
			}

			passwordTries++
		}

		return secret, err
	}

	return authMethod
}
