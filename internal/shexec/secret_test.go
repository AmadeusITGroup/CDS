package shexec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetCredentialCallbacks restores the package-level callbacks after a test
// so cases do not leak registration state into each other.
func resetCredentialCallbacks(t *testing.T) {
	t.Helper()
	prevGetter := getterOnShexecPassword
	prevValidation := validationOnShexecPassword
	t.Cleanup(func() {
		getterOnShexecPassword = prevGetter
		validationOnShexecPassword = prevValidation
	})
}

// TestPromptForSecretWithoutRegistrationReturnsError guards the regression
// where an unregistered password getter caused a nil-pointer panic during SSH
// password authentication (the from-scratch `cds project run` crash). It must
// return a clean error instead of panicking.
func TestPromptForSecretWithoutRegistrationReturnsError(t *testing.T) {
	resetCredentialCallbacks(t)
	getterOnShexecPassword = nil

	assert.NotPanics(t, func() {
		got, err := promptForSecret("password?")
		assert.Error(t, err)
		assert.Empty(t, got)
	})
}

// TestNotifySecretValidationWithoutRegistrationDoesNotPanic guards the second
// nil-deref site (validation callback) on the same path.
func TestNotifySecretValidationWithoutRegistrationDoesNotPanic(t *testing.T) {
	resetCredentialCallbacks(t)
	validationOnShexecPassword = nil

	assert.NotPanics(t, func() {
		assert.True(t, notifySecretValidation(true))
		assert.False(t, notifySecretValidation(false))
	})
}

// TestPromptForSecretUsesRegisteredGetter confirms a registered getter is
// actually invoked and its result propagated.
func TestPromptForSecretUsesRegisteredGetter(t *testing.T) {
	resetCredentialCallbacks(t)

	var gotMessage string
	RegisterShexecCredentialCallbacks(
		func(message string) (string, error) {
			gotMessage = message
			return "s3cr3t", nil
		},
		func(valid bool) bool { return valid },
	)

	got, err := promptForSecret("enter password:")
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", got)
	assert.Equal(t, "enter password:", gotMessage)
	assert.True(t, notifySecretValidation(true))
}
