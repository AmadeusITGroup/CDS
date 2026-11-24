package command

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/amadeusitgroup/cds/internal/cerr"
)

// realSSHAuthError dials an in-process SSH server that rejects every key, so
// the test matches the error text golang.org/x/crypto/ssh really returns.
func realSSHAuthError(t *testing.T) error {
	t.Helper()
	newSigner := func() ssh.Signer {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := ssh.NewSignerFromKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return signer
	}
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, errors.New("key not trusted")
		},
	}
	serverConfig.AddHostKey(newSigner())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _, _, _ = ssh.NewServerConn(conn, serverConfig)
	}()

	_, err = ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User:            "dev",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(newSigner())},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err == nil {
		t.Fatal("expected the SSH server to reject the key")
	}
	// wrapped as connectSSHKey does
	return cerr.AppendError("Failed to open SSH connection", err)
}

func TestWithRedeployTipOnSSHAuthFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	err := withRedeployTip("p", realSSHAuthError(t))
	if err == nil || !strings.Contains(err.Error(), "cds project rebuild") {
		t.Fatalf("expected a redeploy tip, got %v", err)
	}

	for _, detail := range []cerr.ErrorDetail{cerr.MessageOnly, cerr.TopLevelOnly} {
		saved := cerr.DefaultErrorDetail
		cerr.DefaultErrorDetail = detail
		found := isSSHAuthFailure(realSSHAuthError(t))
		cerr.DefaultErrorDetail = saved
		if !found {
			t.Fatalf("auth failure not detected with error detail %v", detail)
		}
	}
}

func TestWithRedeployTipLeavesOtherErrors(t *testing.T) {
	if err := withRedeployTip("p", nil); err != nil {
		t.Fatalf("nil error became %v", err)
	}
	other := cerr.AppendError("Failed to open SSH connection", errors.New("dial tcp: connection refused"))
	if err := withRedeployTip("p", other); err != other {
		t.Fatalf("non-auth error was changed: %v", err)
	}
}
