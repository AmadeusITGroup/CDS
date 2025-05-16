package authmgr

import (
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/cos"

	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	kPermFile = fs.FileMode(0600)
)

var (
	sRepo        *repository
	repoFilePath = cenv.ConfigFile("secret.json")
)

func resetContent() {
	sRepo = nil
}

func repoContent() *repository {
	if sRepo == nil {
		sRepo = &repository{}
		if err := parseFile(repoFilePath, sRepo); err != nil {
			clog.Error("Failed to read secret file", err)
		}
		// we can have a null value in secret.json, which results in a nil value for sSecret, which is unusable
		if sRepo.Secrets == nil {
			sRepo.Secrets = make(map[string]Secret)
		}
	}

	return sRepo
}

type bom interface {
	unmarshall(string) error
}

func parseFile(path string, b bom) error {
	if cos.NotExist(path) {
		return cerr.NewError(fmt.Sprintf("Failed to parse file (%s), specified path doesn't exist", path))
	}
	if err := b.unmarshall(path); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to deserialize file (%v)", path), err)
	}
	return nil
}

func write() error {
	bytes, jsonErr := json.MarshalIndent(repoContent(), "", "  ")
	if jsonErr != nil {
		return cerr.AppendError("Failed serialize configuration", jsonErr)
	}
	if ioErr := cos.WriteFile(repoFilePath, bytes, kPermFile); ioErr != nil {
		return cerr.AppendError("Failed write configuration", ioErr)
	}
	return nil
}

type repository struct {
	Secrets map[string]Secret `json:"secrets"`
}

type Secret struct {
	Login    string `json:"login"`
	Metadata []byte `json:"metadata"`
	Raw      []byte `json:"raw"`
}

func (s *repository) unmarshall(path string) error {
	if err := cg.UnmarshalJSON(path, s); err != nil {
		return cerr.AppendError("Failed deserialize content", err)
	}
	return nil
}

func secretDetails(key string) Secret {
	secret, ok := repoContent().Secrets[key]
	if ok {
		return secret
	}
	return Secret{}
}

func secretLogin(key string) string {
	return secretDetails(key).Login
}

func secretMetadata(key string) []byte {
	return secretDetails(key).Metadata
}

func secretRaw(key string) []byte {
	return secretDetails(key).Raw
}

type visitSecret func(*Secret)

func (v visitSecret) set(key string) error {
	s, ok := repoContent().Secrets[key]
	if !ok {
		s = Secret{}
	}
	v(&s)
	repoContent().Secrets[key] = s

	if err := write(); err != nil {
		return cerr.AppendError("Failed to save secret", err)
	}
	return nil
}

func setLogin(key string, l string) error {
	var fn visitSecret = func(s *Secret) {
		s.Login = l
	}
	return fn.set(key)
}

func setMetadata(key string, m []byte) error {
	var fn visitSecret = func(s *Secret) {
		s.Metadata = m
	}
	return fn.set(key)
}

func setRaw(key string, r []byte) error {
	var fn visitSecret = func(s *Secret) {
		s.Raw = r
	}
	return fn.set(key)
}
