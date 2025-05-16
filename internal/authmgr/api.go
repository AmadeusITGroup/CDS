package authmgr

import (
	"github.com/amadeusitgroup/cds/internal/clog"
)

type auth struct {
	login  string
	prompt string
}

func New(options ...func(*auth)) *auth {
	a := &auth{}
	for _, option := range options {
		option(a)
	}
	return a
}

func WithLogin(login string) func(*auth) {
	return func(a *auth) {
		a.login = login
	}
}

func WithPrompt(prompt string) func(*auth) {
	return func(a *auth) {
		a.prompt = prompt
	}
}

/************************************************************/
/*                                                          */
/* `ar` and `scm` authentication interfaces implementation  */
/*                                                          */
/************************************************************/

func (a *auth) User(secretKey string) string {
	user := secretLogin(secretKey)
	if len(user) > 0 {
		return user
	}
	return a.login
}

func (a *auth) Password(secretKey string) []byte {

	pwd := secretRaw(secretKey)
	if len(pwd) > 0 {
		return pwd
	}

	var stdin string
	var err error
	if stdin, err = password(a.prompt); err != nil {
		clog.Error("Failed to acquire password")
		return []byte{}
	}
	return []byte(stdin)
}

func (a *auth) Retry(secretKey string) []byte {
	return a.Password(secretKey)
}

func (a *auth) Save(secretKey string, secret []byte) error {
	return setRaw(secretKey, secret)
}

func (a *auth) SaveInfo(secretKey string, info []byte) error {
	return setMetadata(secretKey, info)
}

func (a *auth) Token(secretKey string) []byte {
	return secretRaw(secretKey)
}

func (a *auth) TokenInfo(secretKey string) []byte {
	return secretMetadata(secretKey)
}
