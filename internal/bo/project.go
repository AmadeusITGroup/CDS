package bo

type SrcRepoInfo struct {
	RepoURI string
	RepoRef string
	ToClone bool
}

// ProjectInfo holds the display information for a project used by the `list` command.
type ProjectInfo struct {
	Name               string             `json:"name"`
	Host               string             `json:"host"`
	Status             string             `json:"status"`
	Current            bool               `json:"current"`
	Containers         []ContainerInfo    `json:"containers"`
	OrchestrationUsage OrchestrationUsage `json:"orchestration"`
}

// ContainerInfo is a serialisable view of a container's state, used for output formatting.
type ContainerInfo struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	ExpectedStatus string `json:"expectedStatus"`
}

// GetProjStatus derives a human-readable project status from its containers.
func GetProjStatus(containers []ContainerInfo) string {
	if len(containers) == 0 {
		return "empty"
	}
	if areAllStatus(containers, "running") {
		return "running"
	}
	if areAllStatus(containers, "exited") {
		return "stopped"
	}
	return "unknown"
}

func areAllStatus(cs []ContainerInfo, status string) bool {
	for _, c := range cs {
		if c.Status != status {
			return false
		}
	}
	return true
}
