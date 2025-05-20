package db

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// //////////////////////////////////////////////////////////////////
//
//	Hosts Struct (List of Host in db)
//
// //////////////////////////////////////////////////////////////////

type decorateHostList func(*hosts)
type visitHostList func(hosts) any

func (d *data) getHostList() *hosts {
	return &d.hosts
}

func (v decorateHostList) update() {
	instance().Lock()
	defer instance().Unlock()
	v(instance().d.getHostList())
}

func (v visitHostList) get() any {
	instance().Lock()
	defer instance().Unlock()
	return v(*instance().d.getHostList())
}

func ListHostNames() []string {
	var fn visitHostList = func(hList hosts) any {
		hostNameList := []string{}
		for _, h := range hList.Hosts {
			hostNameList = append(hostNameList, h.Name)
		}
		return hostNameList
	}
	return fn.get().([]string)
}

// Scaffold: AddHost used to take into as parameter isDefault. However here is not the proper place to set that (as when you set one host as default, you must change the value of previous default host)
// From caller's perspective, if you want to set a new host as default, 1st AddHost() then SetHostDefualt()
func AddHost(hostName, username string) {
	var fn decorateHostList = func(hList *hosts) {
		newHost := &host{
			Name:    hostName,
			sshInfo: sshInfo{Username: username},
		}
		hList.Hosts = append(hList.Hosts, newHost)
	}
	fn.update()
}

func RemoveHostFromHostList(hostName string) { //TODO: Rename to 'RemoveHost' once 'RemoveHostFromConfig' has been removed.
	var fn decorateHostList = func(hList *hosts) {
		myList := cg.FilterSlice(hList.Hosts, func(h *host) bool { return h.Name != hostName })
		hList.Hosts = myList
	}
	fn.update()
}

func GetDefaultHostName() string {
	var fn visitHostList = func(hList hosts) any {
		for _, h := range hList.Hosts {
			if h.IsDefault {
				return h.Name
			}
		}
		return ""
	}
	return fn.get().(string)
}

func SetHostToDefault(hostName string) {
	var fn decorateHostList = func(hList *hosts) {
		for _, h := range hList.Hosts {
			h.IsDefault = h.Name == hostName
		}
	}
	fn.update()
}

// //////////////////////////////////////////////////////////////////
//
//	Host Struct
//
// //////////////////////////////////////////////////////////////////

type decorateHost func(*host)
type visitHost func(*host) any

func (v decorateHost) update(hostName string) error {
	instance().Lock()
	defer instance().Unlock()
	host, err := instance().d.getHost(hostName)
	if err != nil {
		return err
	}
	v(host)
	return nil
}

func (v visitHost) get(hostName string) (any, error) {
	instance().Lock()
	defer instance().Unlock()
	host, err := instance().d.getHost(hostName)
	if err != nil {
		return nil, err
	}
	return v(host), nil
}

func (d *data) getHost(hostName string) (*host, error) {
	h, found := cg.FindElemFromSlice(d.Hosts, func(h *host) bool { return h.Name == hostName })
	if !found {
		return nil, cerr.NewError((fmt.Sprintf("Failed to get host %s ", hostName)))
	}
	return h, nil
}

func HasHost(hostName string) bool {
	var fn visitHost = func(h *host) any {
		return h
	}
	_, err := fn.get(hostName)
	return err == nil
}

func GetHostKey(hostName string) string {
	var fn visitHost = func(h *host) any {
		return h.PathToKey
	}
	value, err := fn.get(hostName)
	if err != nil {
		return ""
	}
	return value.(string)
}

func GetHostPubKey(hostName string) string {
	var fn visitHost = func(h *host) any {
		return h.PathToPubKey
	}
	value, err := fn.get(hostName)
	if err != nil {
		return ""
	}
	return value.(string)
}

func UpdateHostKey(bh bo.Host) error {
	var fn decorateHost = func(h *host) {
		h.UseKey = true
		h.PathToKey = bh.PathToPrv
		h.PathToPubKey = bh.PathToPub
	}
	if err := fn.update(bh.Name); err != nil {
		return cerr.AppendErrorFmt("Failed to update host key for host: %s", err, bh.Name)
	}
	return nil
}

func ProjectNamesFromHost(hostName string) []string {
	var fn visitHost = func(h *host) any {
		return h.Projects
	}
	value, err := fn.get(hostName)
	if err != nil {
		return []string{}
	}
	return value.([]string)
}

func RemoveProjectFromHost(hostName string, projectName string) error {
	var fn decorateHost = func(h *host) {
		h.Projects = cg.RemoveElemFromSlice(h.Projects, projectName)
	}
	if err := fn.update(hostName); err != nil {
		return cerr.AppendErrorFmt("Failed to update host %s", err, projectName)
	}

	return nil
}

func RegisterProjectInHost(hostName, projectName string) error {
	var fn decorateHost = func(h *host) {
		h.InUse = true
		h.Projects = cg.AddElementToSliceIfNotExists(h.Projects, projectName)
	}
	if err := fn.update(hostName); err != nil {
		return cerr.AppendErrorFmt("Failed to add register project in host: %s", err, hostName)
	}
	return nil
}

func GetHostUsername(hostName string) string {
	var fn visitHost = func(h *host) any {
		return h.Username
	}
	value, err := fn.get(hostName)
	if err != nil {
		return ""
	}
	return value.(string)
}

func SetOrcInfoName(hostName, name string) error {
	var fn decorateHost = func(h *host) {
		h.OrchestrationInfo.Name = name
	}
	if err := fn.update(hostName); err != nil {
		return cerr.AppendErrorFmt("Failed to update orchestration-info name for host %s", err, hostName)
	}
	return nil
}

func SetOrcInfoStatus(hostName string, status bo.ContainerStatus) error {
	var fn decorateHost = func(h *host) {
		h.OrchestrationInfo.State = status.ToString()
	}
	if err := fn.update(hostName); err != nil {
		return cerr.AppendErrorFmt("Failed to update orchestration-info status for host %s", err, hostName)
	}
	return nil
}

func SetOrcInfoRegistryStatus(hostName string, status bo.ContainerStatus) error {
	var fn decorateHost = func(h *host) {
		h.OrchestrationInfo.RegistryInfo.State = status.ToString()
	}
	if err := fn.update(hostName); err != nil {
		return cerr.AppendErrorFmt("Failed to update orchestration-info registry-status for host %s", err, hostName)
	}
	return nil
}

func SetOrcInfoRegPort(hostName string, port int) error {
	var fn decorateHost = func(h *host) {
		h.OrchestrationInfo.RegistryInfo.Port = port
	}
	if err := fn.update(hostName); err != nil {
		return cerr.AppendErrorFmt("Failed to update orchestration-info registry-port for host %s", err, hostName)
	}
	return nil
}

func GetRegistryInfoFromHost(hostName string) bo.RegistryInfo {
	var fn visitHost = func(h *host) any {
		return bo.RegistryInfo{
			State: h.OrchestrationInfo.RegistryInfo.State,
			Port:  h.OrchestrationInfo.RegistryInfo.Port,
		}
	}
	value, err := fn.get(hostName)
	if err != nil {
		return bo.RegistryInfo{}
	}
	return value.(bo.RegistryInfo)
}
