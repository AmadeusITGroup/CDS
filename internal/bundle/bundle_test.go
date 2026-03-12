package bundle

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoundTrip(t *testing.T) {
	entries := []struct {
		entry entry
		data  string
	}{
		{NewEntryForRessource("feature", "feature", "ssh-daemon"), "feature-tarball-bytes"},
		{NewEntryForRessource("file", "registry_auth", "auth.json"), `{"auths":{}}`},
		{NewEntryForRessource("file", "user_config", "my-bashrc"), "export PS1=..."},
	}

	var buf bytes.Buffer
	bw, err := NewWriter(WithWriter(&buf))
	assert.Nil(t, err)
	for _, e := range entries {
		if err := bw.Add(e.entry, bytes.NewReader([]byte(e.data))); err != nil {
			t.Fatalf("Add(%s) failed: %v", e.entry, err)
		}
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	br, err := NewReader(WithReader(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	for _, e := range entries {
		data, err := br.Read(e.entry)
		if err != nil {
			t.Errorf("Read(%s) failed: %v", e.entry, err)
			continue
		}
		if string(data) != e.data {
			t.Errorf("Open(%s) = %q, want %q", e.entry, data, e.data)
		}
	}
}

func TestCollisionDetection(t *testing.T) {
	var buf bytes.Buffer
	bw, err := NewWriter(WithWriter(&buf))
	assert.Nil(t, err)
	entry := NewEntryForRessource("file", "user_config", "my-bashrc")
	if err := bw.Add(entry, bytes.NewReader([]byte("first"))); err != nil {
		t.Fatalf("first Add failed: %v", err)
	}
	if err := bw.Add(entry, bytes.NewReader([]byte("second"))); err == nil {
		t.Fatal("expected error on duplicate entry, got nil")
	}
}

func TestEmptyBundle(t *testing.T) {
	var buf bytes.Buffer
	bw, err := NewWriter(WithWriter(&buf))
	assert.Nil(t, err)
	if err := bw.Close(); err != nil {
		t.Fatalf("Close on empty writer failed: %v", err)
	}

	br, err := NewReader(WithReader(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatalf("NewReader on empty bundle failed: %v", err)
	}
	if len(br.datamap) != 0 {
		t.Errorf("expected 0 entries, got %d", len(br.datamap))
	}
}

func TestLookup(t *testing.T) {
	var buf bytes.Buffer
	bw, err := NewWriter(WithWriter(&buf))
	assert.Nil(t, err)
	bashrcEntry := NewEntryForRessource("file", "user_config", "my-bashrc")
	bw.Add(bashrcEntry, bytes.NewReader([]byte("bash")))
	sshFeatureEntry := NewEntryForRessource("feature", "feature", "ssh-daemon")
	bw.Add(sshFeatureEntry, bytes.NewReader([]byte("ssh")))
	bw.Close()

	br, _ := NewReader(WithReader(bytes.NewReader(buf.Bytes())))

	// Lookup by kind + name.
	ok := br.Exists(bashrcEntry)
	if !ok {
		t.Fatal("Lookup(user_config, my-bashrc) not found")
	}

	ok = br.Exists(NewEntryForRessource("it", "doesn't", "exist"))
	if ok {
		t.Error("Lookup for nonexistent should return false")
	}
}

func TestWriterClosedError(t *testing.T) {
	var buf bytes.Buffer
	bw, err := NewWriter(WithWriter(&buf))
	assert.Nil(t, err)
	bw.Close()

	err = bw.Add(NewEntryForRessource("file", "file", "late"), bytes.NewReader([]byte("data")))
	if err == nil {
		t.Error("expected error when adding to closed writer, got nil")
	}
}
