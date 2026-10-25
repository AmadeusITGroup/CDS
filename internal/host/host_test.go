package host

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// the db store is only used in memory, nothing is persisted
	db.AddHost("known.example.com", "alice")
	t.Cleanup(func() { db.RemoveHostFromHostList("known.example.com") })
	require.NoError(t, db.UpdateHostKey(bo.Host{
		Name:    "known.example.com",
		KeyPair: bo.KeyPair{PathToPub: "/keys/id.pub", PathToPrv: "/keys/id"},
	}))

	h, found := Load("known.example.com")
	require.True(t, found)
	assert.Equal(t, "known.example.com", h.FQDN())
	assert.Equal(t, "alice", h.Username())
	assert.Equal(t, "/keys/id", h.PathToPrv())
	assert.Equal(t, "/keys/id.pub", h.PathToPub())

	h, found = Load("unknown.example.com")
	assert.False(t, found)
	assert.Equal(t, "unknown.example.com", h.FQDN())
	assert.Empty(t, h.Username())
}
