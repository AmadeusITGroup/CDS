package command

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/containerruntime"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDetectRuntime replaces the package-level detector for the test's duration.
func stubDetectRuntime(t *testing.T, info containerruntime.Info, err error) {
	t.Helper()
	orig := detectRuntime
	detectRuntime = func() (containerruntime.Info, error) { return info, err }
	t.Cleanup(func() { detectRuntime = orig })
}

// loadEmptyDB initialises an isolated in-memory db backed by the mocked
// filesystem. ResetForTest clears any store left by a prior test so Load starts
// from a clean state.
func loadEmptyDB(t *testing.T) {
	t.Helper()
	db.ResetForTest()
	t.Cleanup(db.ResetForTest)
	src, err := config.DBSource()
	require.NoError(t, err)
	require.NoError(t, db.Load(src))
}

func TestRegisterLocalHost_Success(t *testing.T) {
	setupCommandConfigTestFS(t)
	loadEmptyDB(t)
	stubDetectRuntime(t, containerruntime.Info{Engine: "podman", Version: "5.2.2"}, nil)

	info, err := registerLocalHost(cg.KLocalhost)

	require.NoError(t, err)
	assert.Equal(t, "podman", info.Engine)
	assert.Equal(t, "5.2.2", info.Version)
	assert.True(t, db.HasHost(cg.KLocalhost))
	assert.Equal(t, bo.RuntimeInfo{Engine: "podman", Version: "5.2.2"}, db.GetHostRuntime(cg.KLocalhost))
}

func TestRegisterLocalHost_DetectionFailureRegistersNothing(t *testing.T) {
	setupCommandConfigTestFS(t)
	loadEmptyDB(t)
	stubDetectRuntime(t, containerruntime.Info{}, containerruntime.ErrMachineNotRunning)

	_, err := registerLocalHost(cg.KLocalhost)

	require.Error(t, err)
	assert.ErrorIs(t, err, containerruntime.ErrMachineNotRunning)
	assert.False(t, db.HasHost(cg.KLocalhost), "no host should be registered when detection fails")
}

func TestRegisterLocalHost_Idempotent(t *testing.T) {
	setupCommandConfigTestFS(t)
	loadEmptyDB(t)
	stubDetectRuntime(t, containerruntime.Info{Engine: "podman", Version: "5.2.2"}, nil)

	_, err := registerLocalHost(cg.KLocalhost)
	require.NoError(t, err)
	_, err = registerLocalHost(cg.KLocalhost)
	require.NoError(t, err)

	hosts := db.ListHostNames()
	count := 0
	for _, h := range hosts {
		if h == cg.KLocalhost {
			count++
		}
	}
	assert.Equal(t, 1, count, "re-registering must not duplicate the host")
}

func TestIsLocalTarget(t *testing.T) {
	assert.True(t, isLocalTarget(cg.KLocalhost))
	assert.False(t, isLocalTarget("agent.example"))
	assert.False(t, isLocalTarget(""))
}
