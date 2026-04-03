package bo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetProjStatus_Empty(t *testing.T) {
	assert.Equal(t, "empty", GetProjStatus([]ContainerInfo{}))
}

func Test_GetProjStatus_Nil(t *testing.T) {
	assert.Equal(t, "empty", GetProjStatus(nil))
}

func Test_GetProjStatus_AllRunning(t *testing.T) {
	containers := []ContainerInfo{
		{Id: "1", Name: "c1", Status: "running"},
		{Id: "2", Name: "c2", Status: "running"},
	}
	assert.Equal(t, "running", GetProjStatus(containers))
}

func Test_GetProjStatus_AllStopped(t *testing.T) {
	containers := []ContainerInfo{
		{Id: "1", Name: "c1", Status: "exited"},
		{Id: "2", Name: "c2", Status: "exited"},
	}
	assert.Equal(t, "stopped", GetProjStatus(containers))
}

func Test_GetProjStatus_Mixed(t *testing.T) {
	containers := []ContainerInfo{
		{Id: "1", Name: "c1", Status: "running"},
		{Id: "2", Name: "c2", Status: "exited"},
	}
	assert.Equal(t, "unknown", GetProjStatus(containers))
}

func Test_GetProjStatus_SingleRunning(t *testing.T) {
	containers := []ContainerInfo{
		{Id: "1", Name: "c1", Status: "running"},
	}
	assert.Equal(t, "running", GetProjStatus(containers))
}

func Test_GetProjStatus_UnknownStatus(t *testing.T) {
	containers := []ContainerInfo{
		{Id: "1", Name: "c1", Status: "something-else"},
	}
	assert.Equal(t, "unknown", GetProjStatus(containers))
}
