package bo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Contains(t *testing.T) {
	containers := Containers{{Name: "container1"}}
	assert.True(t, containers.Contains("container1"))
	assert.False(t, containers.Contains("container2"))
}

func Test_ContainsId(t *testing.T) {
	containers := Containers{{Name: "container1", Id: "id1"}}
	assert.True(t, containers.ContainsId("id1"))
	assert.False(t, containers.Contains("idX"))
}

func Test_Get(t *testing.T) {
	containers := Containers{{Name: "container1"}}
	assert.Equal(t, Container{Name: "container1"}, containers.Get("container1"))
	assert.Equal(t, Container{}, containers.Get("container2"))
}

func Test_GetById(t *testing.T) {
	containers := Containers{{Name: "container1", Id: "id1"}}
	assert.Equal(t, Container{Name: "container1", Id: "id1"}, containers.GetById("id1"))
	assert.Equal(t, Container{}, containers.GetById("idX"))
}

func Test_AddPort(t *testing.T) {
	container := Container{}
	err := container.AddPort("port1", 8080)
	assert.NoError(t, err)
	assert.Equal(t, PortMapping{"port1": 8080}, container.Pmapping)

	err = container.AddPort("port2", 9090)
	assert.NoError(t, err)
	assert.Equal(t, PortMapping{"port1": 8080, "port2": 9090}, container.Pmapping)

	err = container.AddPort("port1", 7070)
	assert.Error(t, err)
	assert.Equal(t, PortMapping{"port1": 8080, "port2": 9090}, container.Pmapping)
}
