package registry

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"golang.org/x/crypto/bcrypt"
)

const (
	// credentialBytes is the number of random bytes behind the generated
	// registry username and password (hex-encoded, so the strings are twice as
	// long).
	credentialBytes = 24
	// certValidity is how long the self-signed registry certificate is valid.
	certValidity = 10 * 365 * 24 * time.Hour
)

// material holds the on-host paths and the generated credentials for the
// registry. Paths are directories mounted into the registry container.
type material struct {
	// root is the registry material directory on the host, e.g.
	// ~/.cds-agent/registry.
	root string
	// username/password are the generated basic-auth credentials.
	username string
	password string
}

func (m material) certsDir() string { return filepath.Join(m.root, "certs") }
func (m material) authDir() string  { return filepath.Join(m.root, "auth") }
func (m material) dataDir() string  { return filepath.Join(m.root, "data") }
func (m material) certFile() string { return filepath.Join(m.certsDir(), "domain.crt") }
func (m material) keyFile() string  { return filepath.Join(m.certsDir(), "domain.key") }
func (m material) htpasswd() string { return filepath.Join(m.authDir(), "htpasswd") }
func (m material) credsFile() string { return filepath.Join(m.authDir(), "creds") }

// ensure generates the registry material under root if it is not already
// present: a self-signed TLS certificate/key for Hostname and an htpasswd file
// with freshly generated random credentials. It is idempotent on the
// certificate (regenerating credentials would lock out already-trusting
// clients, so when the material exists it is left as is and the existing
// credentials are reused).
func ensureMaterial(root string) (material, error) {
	m := material{root: root}
	for _, dir := range []string{m.certsDir(), m.authDir(), m.dataDir()} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return material{}, cerr.AppendErrorFmt("failed to create registry dir %s", err, dir)
		}
	}

	// reuse existing material so repeated builds do not rotate credentials and
	// certificates out from under clients that already trust them.
	if fileExists(m.certFile()) && fileExists(m.keyFile()) && fileExists(m.htpasswd()) && fileExists(m.credsFile()) {
		user, pass, err := readPersistedCredentials(m.credsFile())
		if err != nil {
			return material{}, err
		}
		m.username, m.password = user, pass
		return m, nil
	}

	if err := m.generateCertificate(); err != nil {
		return material{}, err
	}
	if err := m.generateCredentials(); err != nil {
		return material{}, err
	}
	return m, nil
}

// generateCertificate writes a self-signed ECDSA certificate and key for
// Hostname into the certs directory.
func (m *material) generateCertificate() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return cerr.AppendError("failed to generate registry key", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return cerr.AppendError("failed to generate certificate serial", err)
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: Hostname},
		DNSNames:              []string{Hostname},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(certValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return cerr.AppendError("failed to create registry certificate", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(m.certFile(), certPEM, 0644); err != nil {
		return cerr.AppendError("failed to write registry certificate", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return cerr.AppendError("failed to marshal registry key", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(m.keyFile(), keyPEM, 0600); err != nil {
		return cerr.AppendError("failed to write registry key", err)
	}
	return nil
}

// generateCredentials creates random credentials and writes a bcrypt htpasswd
// line the registry server accepts.
func (m *material) generateCredentials() error {
	user, err := randomToken()
	if err != nil {
		return err
	}
	pass, err := randomToken()
	if err != nil {
		return err
	}
	// registry usernames cannot contain ':'; the hex token is safe.
	user = "cds" + user[:12]
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return cerr.AppendError("failed to hash registry password", err)
	}
	line := fmt.Sprintf("%s:%s\n", user, string(hash))
	if err := os.WriteFile(m.htpasswd(), []byte(line), 0600); err != nil {
		return cerr.AppendError("failed to write registry htpasswd", err)
	}
	// persist the plaintext credentials (agent-side, 0600) so they can be
	// reused across builds and injected into the user's devcontainer for
	// `docker login`; the bcrypt hash in htpasswd is not reversible.
	if err := os.WriteFile(m.credsFile(), []byte(user+"\n"+pass+"\n"), 0600); err != nil {
		return cerr.AppendError("failed to persist registry credentials", err)
	}
	m.username, m.password = user, pass
	return nil
}

// readPersistedCredentials reads the plaintext user/password persisted at
// generation time.
func readPersistedCredentials(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cg.EmptyStr, cg.EmptyStr, cerr.AppendError("failed to read registry credentials", err)
	}
	lines := strings.SplitN(strings.TrimRight(string(data), "\n"), "\n", 2)
	if len(lines) != 2 {
		return cg.EmptyStr, cg.EmptyStr, cerr.NewError("registry credentials file is malformed")
	}
	return lines[0], lines[1], nil
}

func randomToken() (string, error) {
	b := make([]byte, credentialBytes)
	if _, err := rand.Read(b); err != nil {
		return cg.EmptyStr, cerr.AppendError("failed to read random bytes", err)
	}
	return fmt.Sprintf("%x", b), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
