package db

import (
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
)

// Scaffold: As said in bo.host_scaffold. bo.Host and bo.HostInfo shoule be removed or keep one at most. To be reviewed
func GetHost(hostName string) (bo.Host, error) {
	var fn visitHost = func(h *host) any {
		return bo.Host{
			Name:     h.Name,
			Username: h.Username,
			KeyPair: bo.KeyPair{
				PathToPub: h.PathToPubKey,
				PathToPrv: h.PathToKey,
			},
		}
	}
	cHost, err := fn.get(hostName)
	if err != nil {
		return bo.Host{}, err
	}

	return cHost.(bo.Host), nil
}

func GetHostInfo(hostName string) bo.HostInfo {
	var fn visitHost = func(h *host) any {
		return bo.HostInfo{
			Name:           h.Name,
			Username:       h.Username,
			PublicKeyPath:  h.PathToPubKey,
			PrivateKeyPath: h.PathToKey,
			Projects:       h.Projects,
			InUse:          h.InUse,
			IsDefault:      h.IsDefault,
			Orchestration: bo.OrchestrationInfo{
				Name:  h.OrchestrationInfo.Name,
				State: h.OrchestrationInfo.State,
				RegistryInfo: bo.RegistryInfo{
					State: h.OrchestrationInfo.RegistryInfo.State,
					Port:  h.OrchestrationInfo.RegistryInfo.Port,
				},
			},
		}
	}
	cHost, err := fn.get(hostName)
	if err != nil {
		return bo.HostInfo{}
	}

	return cHost.(bo.HostInfo)

}

// Same thing bo.host should be removed.
func GetHostFromProjectName(projectName string) bo.Host {
	projectHost := ProjectHostName(projectName)
	return bo.Host{
		Name: projectHost,
		KeyPair: bo.KeyPair{
			PathToPrv: GetHostKey(projectHost),
			PathToPub: GetHostPubKey(projectHost),
		},
		Username: GetHostUsername(projectHost),
	}
}

// Scaffold There should be no dependency between project and hosts. Both RemoveHost(hostname) && SetProjectHost(projectName, "") should be called instead of this function.
func RemoveHostFromConfig(hostName string) error {
	projectList := ProjectNamesFromHost(hostName)
	for _, p := range projectList {
		err := SetProjectHost(p, "")
		if err != nil {
			return cerr.AppendErrorFmt("Failed to remove host %s from db", err, hostName)
		}
	}
	RemoveHostFromHostList(hostName)
	return nil
}
