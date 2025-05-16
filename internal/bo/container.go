package bo

import (
	"fmt"
	"slices"

	"github.com/amadeusitgroup/cds/internal/cerr"
)

type PortMapping map[string]int // key: container's port , val: host's port
type ContainerID string
type ContainerName string
type ContainerStatus int
type ContainerUser string
type ContainerRemoteUser string

const (
	KContainerStatusRunning ContainerStatus = iota
	KContainerStatusExited
	KContainerStatusDeleted
	KContainerStatusUnknown
	KContainerStatusArchived
)

const (
	KSSHPortMapping = "22/tcp"
)

var (
	containerStatuses map[ContainerStatus]string = map[ContainerStatus]string{
		KContainerStatusRunning:  "running",
		KContainerStatusExited:   "exited",
		KContainerStatusDeleted:  "deleted",
		KContainerStatusUnknown:  "unknown",
		KContainerStatusArchived: "archived",
	}
)

type Container struct {
	Id             ContainerID
	Name           ContainerName
	Pmapping       PortMapping
	Status         ContainerStatus
	ExpectedStatus ContainerStatus
	RemoteUser     ContainerRemoteUser
	User           ContainerUser
}

func (status ContainerStatus) ToString() string {
	return FContainerStatus(status)
}

func FContainerStatus(cs ContainerStatus) string {
	fstatus, ok := containerStatuses[cs]
	if !ok {
		return containerStatuses[KContainerStatusUnknown]
	}
	return fstatus
}

func SContainerStatus(containerStratusStr string) ContainerStatus {
	for status, fStatus := range containerStatuses {
		if containerStratusStr == fStatus {
			return status
		}
	}
	return KContainerStatusUnknown
}

func (c *Container) AddPort(key string, val int) error {
	if c.Pmapping == nil {
		c.Pmapping = make(PortMapping)
	}
	if _, ok := c.Pmapping[key]; ok {
		return cerr.NewError(fmt.Sprintf(`attempt to update existing key (%v) 's value from (%v) to (%v)`,
			key,
			c.Pmapping[key],
			val),
		)
	}
	c.Pmapping[key] = val
	return nil
}

func (c *Container) PortMapping() PortMapping {
	return c.Pmapping
}

type Containers []Container

func (cs *Containers) Contains(name ContainerName) bool {
	return slices.ContainsFunc((*cs), func(c Container) bool {
		return c.Name == name
	})
}

// search for a container by container ID
func (cs *Containers) ContainsId(id ContainerID) bool {
	return slices.ContainsFunc((*cs), func(c Container) bool {
		return c.Id == id
	})

}

func (cs *Containers) Get(name ContainerName) Container {
	for _, container := range *cs {
		if container.Name == name {
			return container
		}
	}
	return Container{}
}

func (cs *Containers) GetById(id ContainerID) Container {
	for _, container := range *cs {
		if container.Id == id {
			return container
		}
	}
	return Container{}
}
