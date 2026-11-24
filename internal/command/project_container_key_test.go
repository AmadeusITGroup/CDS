package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/shexec"
)

func TestEnsureProjectContainerKeyPairKeepsExistingKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	host := projectAgentHost("p")
	pair := shexec.KeyPairPaths(host)
	if err := os.MkdirAll(filepath.Dir(pair.PathToPrv), 0700); err != nil {
		t.Fatal(err)
	}

	// Only the private key exists: must refuse and leave it untouched.
	if err := os.WriteFile(pair.PathToPrv, []byte("PRIVATE"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensureProjectContainerKeyPair("p"); err == nil {
		t.Fatal("expected an error for an incomplete key pair")
	}
	if b, _ := os.ReadFile(pair.PathToPrv); string(b) != "PRIVATE" {
		t.Fatalf("private key was overwritten: %q", b)
	}

	// Full pair exists: reused unchanged.
	if err := os.WriteFile(pair.PathToPub, []byte("PUBLIC"), 0644); err != nil {
		t.Fatal(err)
	}
	prv, _, err := ensureProjectContainerKeyPair("p")
	if err != nil || prv != pair.PathToPrv {
		t.Fatalf("got %q, %v", prv, err)
	}
	if b, _ := os.ReadFile(pair.PathToPrv); string(b) != "PRIVATE" {
		t.Fatalf("private key was overwritten: %q", b)
	}
}
