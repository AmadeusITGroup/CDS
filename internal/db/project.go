package db

import (
	"fmt"
	"slices"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

const (
	nilPort = -1
)

type decorateProject func(*project)
type visitProject func(project) any

func (v decorateProject) update(projectName string) error {
	instance().Lock()
	defer instance().Unlock()
	project, err := instance().d.getProject(projectName)
	if err != nil {
		return err
	}
	v(project)
	return nil
}

func (v visitProject) get(projectName string) (any, error) {
	instance().Lock()
	defer instance().Unlock()
	project, err := instance().d.getProject(projectName)
	if err != nil {
		return nil, err
	}
	return v(*project), nil
}

func (d *data) getProject(projectName string) (*project, error) {
	for _, p := range d.Projects {
		if p.Name == projectName {
			return p, nil
		}
	}
	return nil, cerr.NewError((fmt.Sprintf("Failed to get project %s ", projectName)))
}

func (d *data) removeProjectFromList(projectName string) {
	d.Projects = slices.DeleteFunc(d.Projects, func(p *project) bool {
		return p.Name == projectName
	})
}

func (project *project) getProjectContainer(containerName string) (*containerInfo, error) {
	for _, container := range project.Containers {
		if container.Name == containerName {
			return container, nil
		}
	}
	return nil, fmt.Errorf("container %s not found", containerName)
}

func RemoveHostAndContainersFromProject(projectName string) error {
	var fn decorateProject = func(p *project) {
		p.Containers = nil
		p.Host = ""
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}
	return nil
}

// Remove Project is going to replace the Delete Project located in the scaffold file
func RemoveProject(projectName string) {
	instance().Lock()
	defer instance().Unlock()
	instance().d.removeProjectFromList(projectName)
}

func AddContainerInfo(projectName string, info bo.Container) error {
	var fn decorateProject = func(p *project) {
		if len(p.Containers) == 0 {
			p.Containers = []*containerInfo{}
		}

		if slices.ContainsFunc(p.Containers, func(c *containerInfo) bool { return c.Name == string(info.Name) }) {
			clog.Warn(fmt.Sprintf("Container name %s already exists, skipping", info.Name))
			return
		}

		portMappings := info.PortMapping()
		var port int
		if val, ok := portMappings[bo.KSSHPortMapping]; ok {
			port = val
		}
		cInfo := &containerInfo{
			Id:            string(info.Id),
			State:         bo.FContainerStatus(info.Status),
			ExpectedState: bo.FContainerStatus(info.ExpectedStatus),
			Name:          string(info.Name),
			PortSSH:       port,
			RemoteUser:    string(info.RemoteUser),
		}
		p.Containers = append(p.Containers, cInfo)
	}

	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}

	return nil
}

func SetProjectHost(projectName, hostName string) error {
	var fn decorateProject = func(p *project) {
		p.Host = strings.ToLower(hostName)
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}
	return nil
}

func SetOrchestrationRequested(projectName string) error {
	var fn decorateProject = func(p *project) {
		p.OrchestrationUsage.Cluster.Use = true
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}
	return nil
}

func SetProjectRegistryUsage(projectName string) error {
	var fn decorateProject = func(p *project) {
		p.OrchestrationUsage.Registry.Use = true
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}
	return nil
}

func SetProjectSshTunnelNeeded(projectName string) error {
	var fn decorateProject = func(p *project) {
		p.UseSshTunnel = true
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}
	return nil
}

func SetNasRequested(projectName string) error {
	var fn decorateProject = func(p *project) {
		p.NasRequested = true
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}
	return nil
}

func SetOverrideImageTag(projectName string, overrideTag string) error {
	var fn decorateProject = func(p *project) {
		p.OverrideImageTag = overrideTag
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}
	return nil
}

func SetProjectSrcRepoInfo(projectName, repoURI, repoRef string, toClone bool) error {
	var fn decorateProject = func(p *project) {
		p.SrcRepo.URI = repoURI
		p.SrcRepo.Ref = repoRef
		p.SrcRepo.ToClone = toClone
	}
	if err := fn.update(projectName); err != nil {
		return cerr.AppendErrorFmt("Failed to update project %s", err, projectName)
	}
	return nil
}

func IsSshTunnelNeeded(projectName string) bool {
	var fn visitProject = func(p project) any {
		return p.UseSshTunnel
	}
	val, err := fn.get(projectName)
	if err != nil {
		return false
	}
	return val.(bool)
}

func ContainerSSHPort(projectName string, containerName string) int {
	var fn visitProject = func(p project) any {
		cInfo, err := p.getProjectContainer(containerName)
		if err != nil {
			clog.Warn("Failed to get container ssh port, returning nil", err)
			return nilPort
		}
		return cInfo.PortSSH
	}
	val, err := fn.get(projectName)
	if err != nil {
		return nilPort
	}
	return val.(int)
}

func ProjectConfig(projectName string) string {
	var fn visitProject = func(p project) any {
		switch {
		case len(p.ConfDir) != 0:
			return p.ConfDir
		case len(p.Flavour.LocalConfDir) != 0:
			return p.Flavour.LocalConfDir
		case len(p.SrcRepo.LocalConfDir) != 0:
			return p.SrcRepo.LocalConfDir
		}
		return ""
	}
	val, err := fn.get(projectName)
	if err != nil {
		return ""
	}
	return val.(string)
}

func ProjectSrcRepoInfo(projectName string) bo.SrcRepoInfo {
	var fn visitProject = func(p project) any {
		return bo.SrcRepoInfo{
			RepoURI: p.SrcRepo.URI,
			RepoRef: p.SrcRepo.Ref,
			ToClone: p.SrcRepo.ToClone,
		}
	}
	val, err := fn.get(projectName)
	if err != nil {
		return bo.SrcRepoInfo{}
	}
	return val.(bo.SrcRepoInfo)
}

func IsProjectConfiguredWithFlavour(projectName string) bool {
	var fn visitProject = func(p project) any {
		return p.Flavour != flavourInfo{}
	}
	val, err := fn.get(projectName)
	if err != nil {
		return false
	}
	return val.(bool)
}

func ProjectFlavourName(projectName string) string {
	var fn visitProject = func(p project) any {
		return p.Flavour.Name
	}
	val, err := fn.get(projectName)
	if err != nil {
		return ""
	}
	return val.(string)
}

func ProjectContainersName(projectName string) []string {
	var fn visitProject = func(p project) any {
		containers := []string{}
		for _, container := range p.Containers {
			containers = append(containers, container.Name)
		}
		return containers
	}
	val, err := fn.get(projectName)
	if err != nil {
		clog.Warn("Failed to get project containers, returning empty", err)
		return []string{}
	}
	return val.([]string)
}

func ProjectContainerRemoteUser(projectName string, containerName string) string {
	var fn visitProject = func(p project) any {
		containerInfo, err := p.getProjectContainer(containerName)
		if err != nil {
			clog.Warn("Failed to get container info, returning empty string", err)
			return ""
		}
		return containerInfo.RemoteUser
	}
	val, err := fn.get(projectName)
	if err != nil {
		return ""
	}
	return val.(string)

}

func ProjectHostName(projectName string) string {
	var fn visitProject = func(p project) any {
		return p.Host
	}
	val, err := fn.get(projectName)
	if err != nil {
		return ""
	}
	return val.(string)
}

func HasProjectSrcRepoToBeCloned(projectName string) bool {
	var fn visitProject = func(p project) any {
		return p.SrcRepo.ToClone
	}
	val, err := fn.get(projectName)
	if err != nil {
		return false
	}
	return val.(bool)
}

func IsNasRequested(projectName string) bool {
	var fn visitProject = func(p project) any {
		return p.NasRequested
	}
	val, err := fn.get(projectName)
	if err != nil {
		return false
	}
	return val.(bool)
}

func OverrideImageTag(projectName string) string {
	var fn visitProject = func(p project) any {
		return p.OverrideImageTag
	}
	val, err := fn.get(projectName)
	if err != nil {
		return ""
	}
	return val.(string)
}

func IsOrchestrationUsed(projectName string) bool {
	var fn visitProject = func(p project) any {
		return p.OrchestrationUsage.Cluster.Use
	}
	val, err := fn.get(projectName)
	if err != nil {
		return false
	}
	return val.(bool)
}

func IsRegistryUsed(projectName string) bool {
	var fn visitProject = func(p project) any {
		return p.OrchestrationUsage.Registry.Use
	}
	val, err := fn.get(projectName)
	if err != nil {
		return false
	}
	return val.(bool)
}

func ProjectsOrchestrationUsage(projectName string) bo.OrchestrationUsage {
	return bo.OrchestrationUsage{
		Cluster: bo.ClusterUsage{
			Use: IsOrchestrationUsed(projectName),
		},
		Registry: bo.RegistryUsage{
			Use: IsRegistryUsed(projectName),
		},
	}
}
