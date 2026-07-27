package command

import (
	"errors"
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

	registration, err := registerLocalHost(cg.KLocalhost)

	require.NoError(t, err)
	assert.False(t, registration.AlreadyRegistered)
	assert.Equal(t, "podman", registration.Info.Engine)
	assert.Equal(t, "5.2.2", registration.Info.Version)
	assert.True(t, db.HasHost(cg.KLocalhost))
	assert.Equal(t, bo.RuntimeInfo{Engine: "podman", Version: "5.2.2"}, db.GetHostRuntime(cg.KLocalhost))
}

func TestRegisterLocalHost_AlreadyRegisteredSkipsDetection(t *testing.T) {
	setupCommandConfigTestFS(t)
	loadEmptyDB(t)
	db.AddHost(cg.KLocalhost, "tester")
	require.NoError(t, db.SetHostRuntime(cg.KLocalhost, bo.RuntimeInfo{Engine: "podman", Version: "5.2.2"}))
	stubDetectRuntime(t, containerruntime.Info{}, errors.New("detector should not run"))

	registration, err := registerLocalHost(cg.KLocalhost)

	require.NoError(t, err)
	assert.True(t, registration.AlreadyRegistered)
	assert.Equal(t, containerruntime.Info{Engine: "podman", Version: "5.2.2"}, registration.Info)
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
	registration, err := registerLocalHost(cg.KLocalhost)
	require.NoError(t, err)
	assert.True(t, registration.AlreadyRegistered)
	assert.Equal(t, containerruntime.Info{Engine: "podman", Version: "5.2.2"}, registration.Info)

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

// TestUnregisterLocalHostRemovesDBEntry covers the db-cleanup half of the
// unregister flow: deleting a local host drops its db.json entry (runtimeInfo),
// not just the cliconfig agent. Mirrors registration so the two stores stay in
// sync. The cliconfig half is covered by config.TestLocalhostRegistrationFlow.
func TestUnregisterLocalHostRemovesDBEntry(t *testing.T) {
	setupCommandConfigTestFS(t)
	loadEmptyDB(t)
	stubDetectRuntime(t, containerruntime.Info{Engine: "podman", Version: "5.2.2"}, nil)

	_, err := registerLocalHost(cg.KLocalhost)
	require.NoError(t, err)
	require.True(t, db.HasHost(cg.KLocalhost))

	// The command gates db removal on isLocalTarget + HasHost, then calls
	// RemoveHostFromHostList. Exercise that same cleanup here.
	if isLocalTarget(cg.KLocalhost) && db.HasHost(cg.KLocalhost) {
		db.RemoveHostFromHostList(cg.KLocalhost)
	}

	assert.False(t, db.HasHost(cg.KLocalhost), "local host db entry must be removed on unregister")
}
