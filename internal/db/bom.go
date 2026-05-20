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
	Context context `json:"context,omitempty,omitzero"`
	projects
	hosts
	registryInstances
}

func (s *store) unmarshall(r io.Reader) error {
	if err := json.NewDecoder(r).Decode(&s.d); err != nil {
		return cerr.AppendError("Failed deserialize config", err)
	}
	s.d.normalize()
	return nil
}

// //////////////////////////////////////////////////////////////////
//
//	Context Struct
//
// //////////////////////////////////////////////////////////////////

type context struct {
	ProjectContext string `json:"project,omitempty,omitzero"`
}

// //////////////////////////////////////////////////////////////////
//
//	Project Struct
//
// //////////////////////////////////////////////////////////////////

type projects struct {
	Projects []*project `json:"projects,omitempty,omitzero"`
}

type project struct {
	Name               string             `json:"name,omitempty,omitzero"`
	ConfDir            string             `json:"confDir,omitempty,omitzero"`
	Host               string             `json:"host,omitempty,omitzero"`
	Containers         []*containerInfo   `json:"containers,omitempty,omitzero"`
	NasRequested       bool               `json:"nas,omitempty,omitzero"`
	Flavour            flavourInfo        `json:"flavour,omitempty,omitzero"`
	SrcRepo            srcRepoInfo        `json:"srcRepo,omitempty,omitzero"`
	UseSshTunnel       bool               `json:"useSshTunnel,omitempty,omitzero"`
	OverrideImageTag   string             `json:"overrideImageTag,omitempty,omitzero"`
	OrchestrationUsage orchestrationUsage `json:"orchestration,omitempty,omitzero"`
}

// Scaffold: Remove either State or ExpectedState. The status stored in the db can only be the expected one. Makes no sense to try to store 'current' status. The only real way to retrieve it
// is through 'podman ps -a'. Indeed if there is not an use case where State != ExpectedState.
type containerInfo struct {
	Id            string `json:"id,omitempty,omitzero"`
	State         string `json:"status,omitempty,omitzero"`
	ExpectedState string `json:"expectedStatus,omitempty,omitzero"`
	Name          string `json:"name,omitempty,omitzero"`
	PortSSH       int    `json:"portSSH,omitempty,omitzero"`
	RemoteUser    string `json:"remoteUser,omitempty,omitzero"`
}

type flavourInfo struct {
	Name         string `json:"name,omitempty,omitzero"`
	OverrideDir  string `json:"overridefDir,omitempty,omitzero"`
	LocalConfDir string `json:"localConfDir,omitempty,omitzero"`
}
type srcRepoInfo struct {
	LocalConfDir string `json:"localConfDir,omitempty,omitzero"`
	ToClone      bool   `json:"toClone,omitempty,omitzero"`
	URI          string `json:"uri,omitempty,omitzero"`
	Ref          string `json:"reference,omitempty,omitzero"`
}

type orchestrationUsage struct {
	Cluster  clusterUsage  `json:"cluster,omitempty,omitzero"`
	Registry registryUsage `json:"registry,omitempty,omitzero"`
}

// //////////////////////////////////////////////////////////////////
//
//	Host Struct
//
// //////////////////////////////////////////////////////////////////

type hosts struct {
	Hosts []*host `json:"hosts,omitempty,omitzero"`
}

type host struct {
	Name              string            `json:"name,omitempty,omitzero"`
	Projects          []string          `json:"projects,omitempty,omitzero"`
	InUse             bool              `json:"inUse,omitempty,omitzero"`
	IsDefault         bool              `json:"default,omitempty,omitzero"`
	OrchestrationInfo orchestrationInfo `json:"orchestrationInfo,omitempty,omitzero"`
	sshInfo
}

type orchestrationInfo struct {
	Name         string       `json:"name,omitempty,omitzero"`
	RegistryInfo registryInfo `json:"registry,omitempty,omitzero"`
	State        string       `json:"status,omitempty,omitzero"`
}
type clusterUsage struct {
	Use bool `json:"use,omitempty,omitzero"`
}

type registryInfo struct {
	State string `json:"status,omitempty,omitzero"`
	Port  int    `json:"port,omitempty,omitzero"`
}

type registryUsage struct {
	Use bool `json:"use,omitempty,omitzero"`
}

type sshInfo struct {
	Username     string `json:"username,omitempty,omitzero"`
	UseKey       bool   `json:"useKey,omitempty,omitzero"`
	PathToKey    string `json:"key,omitempty,omitzero"`
	PathToPubKey string `json:"pubKey,omitempty,omitzero"`
}

// //////////////////////////////////////////////////////////////////
//
//	Registry istances Struct
//
// //////////////////////////////////////////////////////////////////

type registryInstance struct {
	Name string `json:"name,omitempty,omitzero"`
}
type registryInstances struct {
	Instances []*registryInstance `json:"registries,omitempty,omitzero"`
}
