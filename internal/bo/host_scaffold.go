package bo

//Scaffold: In my opinion we should only keep one between HostInfo/Host. Ideally we would not have any of those, in case we need it to avoid recurrence (to be checked) let's keep at one at most.

// TODO:BK: HostInfo has to be deprecated in favor of appropriate functions and methods in package host
type HostInfo struct {
	Name           string            `json:"name"`
	Username       string            `json:"username"`
	PublicKeyPath  string            `json:"public_key_path"`
	PrivateKeyPath string            `json:"private_key_path"`
	Projects       []string          `json:"projects"`
	InUse          bool              `json:"in_use"`
	IsDefault      bool              `json:"default"`
	Orchestration  OrchestrationInfo `json:"orchestration"`
}

type OrchestrationInfo struct {
	Name         string       `json:"name"`
	RegistryInfo RegistryInfo `json:"registry"`
	State        string       `json:"status"`
}

type RegistryInfo struct {
	State string `json:"status"`
	Port  int    `json:"port"`
}
