package db

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/amadeusitgroup/cds/internal/cerr"
)

type bom interface {
	unmarshall(io.Reader) error
}

// store
type store struct {
	sync.Mutex
	d data
}

type data struct {
	Context context `json:"context"`
	projects
	hosts
	registryInstances
}

func (s *store) unmarshall(r io.Reader) error {
	if err := json.NewDecoder(r).Decode(&s.d); err != nil {
		return cerr.AppendError("Failed deserialize config", err)
	}
	return nil
}

// //////////////////////////////////////////////////////////////////
//
//	Context Struct
//
// //////////////////////////////////////////////////////////////////

type context struct {
	ProjectContext string `json:"project"`
}



// //////////////////////////////////////////////////////////////////
//
//	Project Struct
//
// //////////////////////////////////////////////////////////////////

type projects struct {
	Projects []*project `json:"projects"`
}

type project struct {
	Name               string             `json:"name"`
	ConfDir            string             `json:"confDir"`
	Host               string             `json:"host"`
	Containers         []*containerInfo   `json:"containers"`
	NasRequested       bool               `json:"nas"`
	Flavour            flavourInfo        `json:"flavour"`
	SrcRepo            srcRepoInfo        `json:"srcRepo"`
	UseSshTunnel       bool               `json:"useSshTunnel"`
	OverrideImageTag   string             `json:"overrideImageTag"`
	OrchestrationUsage orchestrationUsage `json:"orchestration"`
}

// Scaffold: Remove either State or ExpectedState. The status stored in the db can only be the expected one. Makes no sense to try to store 'current' status. The only real way to retrieve it
// is through 'podman ps -a'. Indeed if there is not an use case where State != ExpectedState.
type containerInfo struct {
	Id            string `json:"id"`
	State         string `json:"status"`
	ExpectedState string `json:"expectedStatus"`
	Name          string `json:"name"`
	PortSSH       int    `json:"portSSH"`
	RemoteUser    string `json:"remoteUser"`
}

type flavourInfo struct {
	Name         string `json:"name"`
	OverrideDir  string `json:"overridefDir"`
	LocalConfDir string `json:"localConfDir"`
}
type srcRepoInfo struct {
	LocalConfDir string `json:"localConfDir"`
	ToClone      bool   `json:"toClone"`
	URI          string `json:"uri"`
	Ref          string `json:"reference"`
}

type orchestrationUsage struct {
	Cluster  clusterUsage  `json:"cluster"`
	Registry registryUsage `json:"registry"`
}

// //////////////////////////////////////////////////////////////////
//
//	Host Struct
//
// //////////////////////////////////////////////////////////////////

type hosts struct {
	Hosts []*host `json:"hosts"`
}

type host struct {
	Name              string            `json:"name"`
	Projects          []string          `json:"projects"`
	InUse             bool              `json:"inUse"`
	IsDefault         bool              `json:"default"`
	OrchestrationInfo orchestrationInfo `json:"orchestrationInfo"`
	sshInfo
}

type orchestrationInfo struct {
	Name         string       `json:"name"`
	RegistryInfo registryInfo `json:"registry"`
	State        string       `json:"status"`
}
type clusterUsage struct {
	Use bool `json:"use"`
}

type registryInfo struct {
	State string `json:"status"`
	Port  int    `json:"port"`
}

type registryUsage struct {
	Use bool `json:"use"`
}

type sshInfo struct {
	Username     string `json:"username"`
	UseKey       bool   `json:"useKey"`
	PathToKey    string `json:"key"`
	PathToPubKey string `json:"pubKey"`
}

// //////////////////////////////////////////////////////////////////
//
//	Registry istances Struct
//
// //////////////////////////////////////////////////////////////////

type registryInstance struct {
	Name string
}
type registryInstances struct {
	Instances []*registryInstance `json:"registries"`
}
