package command

import (
	"path/filepath"
	"testing"
)

func TestCheckConfDirExists(t *testing.T) {
	dir := t.TempDir()
	if err := checkConfDirExists(dir); err != nil {
		t.Fatalf("existing dir: unexpected error: %v", err)
	}
	if err := checkConfDirExists(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing dir: expected error")
	}
}
