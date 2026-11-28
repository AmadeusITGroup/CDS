package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingCmd struct{}

// Execute changes the db, then fails, like `project rsh` forgetting an absent
// container before reporting the project has no containers.
func (failingCmd) Execute() error {
	db.AddHost("cds102.example.net", "someone")
	return errors.New("boom")
}

// TestRunSavesDBWhenCommandFails: a failing command exits 1 and its db changes
// are still written (CDS-102: os.Exit(1) skipped the deferred db.Save).
func TestRunSavesDBWhenCommandFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CDS_CONFIG_PATH", dir)

	assert.Equal(t, 1, run(failingCmd{}))

	content, err := os.ReadFile(filepath.Join(dir, ".cds", "db.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "cds102.example.net")
}
