package command

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cg "github.com/amadeusitgroup/cds/internal/global"
)

// TestVersionMismatchHint: a local agent is told to restart (orc run does not
// update it, BD-09); a remote agent keeps the orc run hint (CDS-066).
func TestVersionMismatchHint(t *testing.T) {
	local := versionMismatchHint(cg.KLocalhost, "v0")
	assert.Contains(t, local, "restart it to update it")
	assert.NotContains(t, local, "orc run")
	assert.Contains(t, local, "v0")

	remote := versionMismatchHint("host.example.com", "v0")
	assert.Contains(t, remote, "run 'cds space orc run' to update it")
	assert.Contains(t, remote, "v0")
}
