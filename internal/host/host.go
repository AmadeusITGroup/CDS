package host

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/shexec"
)

type host struct {
	name     string
	username string
	password string
	keyPair
	port      int
	isDefault bool
}

func New(ops ...func(*host)) *host {
	h := &host{}
	for _, op := range ops {
		op(h)
	}
	return h
}

func WithName(name string) func(*host) {
	return func(h *host) {
		h.name = name
	}
}

func WithUsername(username string) func(*host) {
	return func(h *host) {
		h.username = username
	}
}

func WithPassword(password string) func(*host) {
	return func(h *host) {
		h.password = password
	}
}

func WithKeyPair(keyPair keyPair) func(*host) {
	return func(h *host) {
		h.keyPair = keyPair
	}
}

func WithPort(port int) func(*host) {
	return func(h *host) {
		h.port = port
	}
}

func WithSetAsDefault(isDefault bool) func(*host) {
	return func(h *host) {
		h.isDefault = isDefault
	}
}

func (h *host) IsValid() bool {
	return len(h.name) > 0
}

type keyPair struct {
	pathToPub string
	pathToPrv string
}

func NewKeyPair(ops ...func(*keyPair)) keyPair {
	k := keyPair{}
	for _, op := range ops {
		op(&k)
	}
	return k
}

func WithPathToPub(pathToPub string) func(*keyPair) {
	return func(k *keyPair) {
		k.pathToPub = pathToPub
	}
}

func WithPathToPrv(pathToPrv string) func(*keyPair) {
	return func(k *keyPair) {
		k.pathToPrv = pathToPrv
	}
}

func encode(h host) bo.Host {
	return bo.Host{
		Name:      h.name,
		Username:  h.username,
		Password:  h.password,
		KeyPair:   bo.KeyPair{PathToPub: h.pathToPub, PathToPrv: h.pathToPrv},
		Port:      h.port,
		IsDefault: h.isDefault,
	}
}

/***********************************************************/
/*                                                         */
/*		Implement:                                         */
/*			`systemd.host` interface                       */
/*			`shexec.host`  interface                       */
/*                                                         */
/***********************************************************/

func (h *host) Defined() bool {
	return db.HasHost(h.name)
}

func (h *host) Build() (returnErr error) {
	if len(h.username) == 0 {
		h.username = cenv.GetUsernameFromEnv()
	}

	db.AddHost(h.name, h.username)
	if h.isDefault {
		db.SetHostToDefault(h.name)
	}

	// cleanup any partial configuration if buildHost failed to avoid falling in a locked config state
	defer func() {
		if returnErr != nil {
			db.RemoveHostFromHostList(h.name)
		}
	}()

	// EnsureDir would be enough but we might as well ensure that config exists
	if err := cenv.EnsureSSHClientConfig(nil); err != nil {
		return cerr.AppendError("Failed to ensure SSH directory existence", err)
	}

	keys, errGen := shexec.GenerateKeyPair(h.name)
	if errGen != nil {
		return cerr.AppendError(fmt.Sprintf("Unable to generate key pair for %v", h.name), errGen)
	}

	h.keyPair = keyPair{pathToPub: keys.PathToPub, pathToPrv: keys.PathToPrv}

	if err := db.UpdateHostKey(encode(*h)); err != nil {
		return cerr.AppendError(fmt.Sprintf("Unable to update host %v with key path", h.name), err)
	}

	if h.name != cg.KLocalhost {
		if err := shexec.CopyKey(shexec.UsingPassword(h), keys.PathToPub, "~/."); err != nil {
			return cerr.AppendError(fmt.Sprintf("Unable to copy public key to %v", h.name), err)
		}
	}

	return nil
}

func (h *host) FQDN() string {
	return h.name
}

func (h *host) HasPassword() bool {
	return len(h.password) > 0
}

func (h *host) Password() string {
	return h.password
}

func (h *host) PathToPrv() string {
	return h.pathToPrv
}

func (h *host) PathToPub() string {
	return h.pathToPub
}

func (h *host) Port() int {
	return h.port
}

func (h *host) Username() string {
	return h.username
}
