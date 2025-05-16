package ar

import (
	"encoding/json"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

var (
	sAuthH  authHandle
	sTokenH tokenHandle
)

func SetAuthenticationHandler(a authHandle) {
	sAuthH = a
}

func SetTokenHandler(t tokenHandle) {
	sTokenH = t
}

type authHandle interface {
	User(secretKey string) string
	Password(secretKey string) []byte
	Retry(secretKey string) []byte
	Save(secretKey string, secret []byte) error
	SaveInfo(secretKey string, info []byte) error
}
type tokenHandle interface {
	Token(secretKey string) []byte
	TokenInfo(secretKey string) []byte
}

func authenticationHandler() authHandle {
	if sAuthH == nil {
		clog.Error("Authentication handler used without proper initialization")
		clog.Debug("Authentication handler used without proper initialization - using a dummy implementation")
		sAuthH = authDefaultImp{}
	}
	return sAuthH
}

func tokenHandler() tokenHandle {

	if sTokenH == nil {
		clog.Error("Token handler used without proper initialization")
		clog.Debug("Token handler used without proper initialization - using a dummy implementation")
		sTokenH = authDefaultImp{}
	}
	return sTokenH
}

type authDefaultImp struct{}

func (adi authDefaultImp) User(secretKey string) string {
	return cg.EmptyStr
}

func (adi authDefaultImp) Password(secretKey string) []byte {
	return []byte{}
}

func (adi authDefaultImp) Retry(secretKey string) []byte {
	return []byte{}
}

func (adi authDefaultImp) Save(secretKey string, secret []byte) error {
	return nil
}

func (adi authDefaultImp) SaveInfo(secretKey string, secret []byte) error {
	return nil
}

func (adi authDefaultImp) Token(secretKey string) []byte {
	return []byte{}
}

func (adi authDefaultImp) TokenInfo(secretKey string) []byte {
	return []byte{}
}

func tokenKey(instance string) string {
	return fmt.Sprintf("ar-tkn-%s", instance)
}

func passwordKey(instance string) string {
	return fmt.Sprintf("ar-pwd-%s", instance)
}

func userKey(instance string) string {
	return fmt.Sprintf("ar-usr-%s", instance)
}

func getArtifactoryUser(instance string) (string, error) {
	return authenticationHandler().User(userKey(instance)), nil
}

func getArtifactoryToken(instance string) (artifactoryToken, error) {
	key := tokenKey(instance)
	tokenInfo := tokenHandler().TokenInfo(key)
	info := artifactoryToken{}
	if err := json.Unmarshal(tokenInfo, &info); err != nil {
		return artifactoryToken{}, cerr.AppendError(fmt.Sprintf("Failed read and deserialize token %s info", key), err)
	}
	raw := tokenHandler().Token(key)
	info.token = string(raw)
	return info, nil
}

func setArtifactoryToken(instance string, at artifactoryToken) error {
	key := tokenKey(instance)
	info, jErr := json.Marshal(at)
	if jErr != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to serialize token %s info", key), jErr)
	}

	if err := authenticationHandler().Save(key, []byte(at.token)); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to save token %s", key), err)
	}

	if err := authenticationHandler().SaveInfo(key, info); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to save token %s info", key), err)
	}
	return nil
}

func getArtifactoryPassword(instance string) (string, error) {
	k := passwordKey(instance)
	s := authenticationHandler().Password(k)
	return string(s), nil
}
