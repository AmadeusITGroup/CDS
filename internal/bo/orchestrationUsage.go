package bo

type OrchestrationUsage struct {
	Cluster  ClusterUsage  `json:"cluster"`
	Registry RegistryUsage `json:"registry"`
}

type ClusterUsage struct {
	Use bool `json:"use"`
}

type RegistryUsage struct {
	Use     bool `json:"use"`
	Secured bool `json:"secured"`
}
