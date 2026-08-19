package db

import (
	"slices"
	"strings"
)

func normalizeHostName(hostName string) string {
	return strings.ToLower(strings.TrimSpace(hostName))
}

func normalizeProjectName(projectName string) string {
	return strings.TrimSpace(projectName)
}

func (d *data) normalize() {
	d.normalizeProjects()
	d.normalizeHosts()
	d.normalizeRegistries()
	d.normalizeContext()
}

func (d *data) normalizeContext() {
	projectName := normalizeProjectName(d.Context.ProjectContext)
	if projectName == "" {
		d.Context.ProjectContext = ""
		return
	}
	if _, err := d.getProject(projectName); err != nil {
		d.Context.ProjectContext = ""
		return
	}
	d.Context.ProjectContext = projectName
}

func (d *data) normalizeProjects() {
	normalizedProjects := make([]*project, 0, len(d.Projects))
	indexByName := make(map[string]int, len(d.Projects))
	for _, p := range d.Projects {
		if p == nil {
			continue
		}
		normalizedProject := cloneProject(p)
		normalizedProject.Name = normalizeProjectName(normalizedProject.Name)
		if normalizedProject.Name == "" {
			continue
		}
		normalizedProject.Host = normalizeHostName(normalizedProject.Host)
		normalizedProject.Containers = normalizeContainers(normalizedProject.Containers)
		if index, ok := indexByName[normalizedProject.Name]; ok {
			normalizedProjects[index] = mergeProject(normalizedProjects[index], normalizedProject)
			continue
		}
		indexByName[normalizedProject.Name] = len(normalizedProjects)
		normalizedProjects = append(normalizedProjects, normalizedProject)
	}
	d.Projects = normalizedProjects
}

func (d *data) normalizeHosts() {
	projectNames := d.projectNameSet()
	projectHosts := d.projectHostMap()
	normalizedHosts := make([]*host, 0, len(d.Hosts))
	indexByName := make(map[string]int, len(d.Hosts))
	defaultHostName := ""

	for _, h := range d.Hosts {
		if h == nil {
			continue
		}
		normalizedHost := cloneHost(h)
		normalizedHost.Name = normalizeHostName(normalizedHost.Name)
		if normalizedHost.Name == "" {
			continue
		}
		normalizedHost.Projects = normalizeProjectList(normalizedHost.Projects, projectNames)
		if normalizedHost.IsDefault {
			defaultHostName = normalizedHost.Name
		}
		if index, ok := indexByName[normalizedHost.Name]; ok {
			normalizedHosts[index] = mergeHost(normalizedHosts[index], normalizedHost)
			continue
		}
		indexByName[normalizedHost.Name] = len(normalizedHosts)
		normalizedHosts = append(normalizedHosts, normalizedHost)
	}

	for _, p := range d.Projects {
		if p.Host == "" {
			continue
		}
		if index, ok := indexByName[p.Host]; ok {
			normalizedHosts[index].Projects = appendProjectIfMissing(normalizedHosts[index].Projects, p.Name)
			continue
		}
		indexByName[p.Host] = len(normalizedHosts)
		normalizedHosts = append(normalizedHosts, &host{Name: p.Host, Projects: []string{p.Name}})
	}

	for _, h := range normalizedHosts {
		h.Projects = normalizeProjectList(h.Projects, projectNames)
		h.Projects = filterHostProjectList(h.Name, h.Projects, projectHosts)
		h.InUse = len(h.Projects) > 0
		h.IsDefault = h.Name == defaultHostName
	}

	if len(normalizedHosts) == 0 {
		d.Hosts = nil
		return
	}
	d.Hosts = normalizedHosts
}

func (d *data) normalizeRegistries() {
	registries := make([]*registryInstance, 0, len(d.Instances))
	indexByName := make(map[string]int, len(d.Instances))
	for _, registry := range d.Instances {
		if registry == nil {
			continue
		}
		normalizedRegistry := *registry
		normalizedRegistry.Name = strings.TrimSpace(normalizedRegistry.Name)
		if normalizedRegistry.Name == "" {
			continue
		}
		if index, ok := indexByName[normalizedRegistry.Name]; ok {
			registries[index] = &normalizedRegistry
			continue
		}
		indexByName[normalizedRegistry.Name] = len(registries)
		registries = append(registries, &normalizedRegistry)
	}
	if len(registries) == 0 {
		d.Instances = nil
		return
	}
	d.Instances = registries
}

func (d *data) projectNameSet() map[string]struct{} {
	projectNames := make(map[string]struct{}, len(d.Projects))
	for _, p := range d.Projects {
		if p == nil || p.Name == "" {
			continue
		}
		projectNames[p.Name] = struct{}{}
	}
	return projectNames
}

func (d *data) projectHostMap() map[string]string {
	projectHosts := make(map[string]string, len(d.Projects))
	for _, p := range d.Projects {
		if p == nil || p.Name == "" || p.Host == "" {
			continue
		}
		projectHosts[p.Name] = p.Host
	}
	return projectHosts
}

func filterHostProjectList(hostName string, projects []string, projectHosts map[string]string) []string {
	if projects == nil {
		return nil
	}
	filteredProjects := make([]string, 0, len(projects))
	for _, projectName := range projects {
		if projectHost, ok := projectHosts[projectName]; ok && projectHost != hostName {
			continue
		}
		filteredProjects = appendProjectIfMissing(filteredProjects, projectName)
	}
	if len(filteredProjects) == 0 {
		return []string{}
	}
	return filteredProjects
}

func normalizeContainers(containers []*containerInfo) []*containerInfo {
	if containers == nil {
		return nil
	}
	normalizedContainers := make([]*containerInfo, 0, len(containers))
	indexByName := make(map[string]int, len(containers))
	for _, c := range containers {
		if c == nil {
			continue
		}
		normalizedContainer := cloneContainer(c)
		normalizedContainer.Name = strings.TrimSpace(normalizedContainer.Name)
		if normalizedContainer.Name == "" {
			continue
		}
		normalizedContainer.Id = strings.TrimSpace(normalizedContainer.Id)
		normalizedContainer.State = strings.ToLower(strings.TrimSpace(normalizedContainer.State))
		normalizedContainer.ExpectedState = strings.ToLower(strings.TrimSpace(normalizedContainer.ExpectedState))
		normalizedContainer.RemoteUser = strings.TrimSpace(normalizedContainer.RemoteUser)
		if index, ok := indexByName[normalizedContainer.Name]; ok {
			normalizedContainers[index] = mergeContainer(normalizedContainers[index], normalizedContainer)
			continue
		}
		indexByName[normalizedContainer.Name] = len(normalizedContainers)
		normalizedContainers = append(normalizedContainers, normalizedContainer)
	}
	if len(normalizedContainers) == 0 {
		return []*containerInfo{}
	}
	return normalizedContainers
}

func normalizeProjectList(projects []string, validProjects map[string]struct{}) []string {
	if projects == nil {
		return nil
	}
	normalizedProjects := make([]string, 0, len(projects))
	for _, projectName := range projects {
		normalizedName := normalizeProjectName(projectName)
		if normalizedName == "" {
			continue
		}
		if _, ok := validProjects[normalizedName]; !ok {
			continue
		}
		normalizedProjects = appendProjectIfMissing(normalizedProjects, normalizedName)
	}
	if len(normalizedProjects) == 0 {
		return []string{}
	}
	return normalizedProjects
}

func appendProjectIfMissing(projects []string, projectName string) []string {
	if slices.Contains(projects, projectName) {
		return projects
	}
	return append(projects, projectName)
}

func cloneProject(p *project) *project {
	clonedProject := *p
	if p.Containers != nil {
		clonedProject.Containers = make([]*containerInfo, 0, len(p.Containers))
		for _, c := range p.Containers {
			clonedProject.Containers = append(clonedProject.Containers, cloneContainer(c))
		}
	}
	return &clonedProject
}

func cloneContainer(c *containerInfo) *containerInfo {
	if c == nil {
		return nil
	}
	clonedContainer := *c
	return &clonedContainer
}

func cloneHost(h *host) *host {
	clonedHost := *h
	if h.Projects != nil {
		clonedHost.Projects = append([]string{}, h.Projects...)
	}
	return &clonedHost
}

func mergeProject(existing, incoming *project) *project {
	mergedProject := cloneProject(incoming)
	mergedProject.ConfDir = firstNonEmpty(mergedProject.ConfDir, existing.ConfDir)
	mergedProject.Host = firstNonEmpty(mergedProject.Host, existing.Host)
	mergedProject.Flavour = mergeFlavour(existing.Flavour, mergedProject.Flavour)
	mergedProject.SrcRepo = mergeSrcRepo(existing.SrcRepo, mergedProject.SrcRepo)
	mergedProject.UseSshTunnel = existing.UseSshTunnel || mergedProject.UseSshTunnel
	mergedProject.NasRequested = existing.NasRequested || mergedProject.NasRequested
	mergedProject.OverrideImageTag = firstNonEmpty(mergedProject.OverrideImageTag, existing.OverrideImageTag)
	mergedProject.OrchestrationUsage.Cluster.Use = existing.OrchestrationUsage.Cluster.Use || mergedProject.OrchestrationUsage.Cluster.Use
	mergedProject.OrchestrationUsage.Registry.Use = existing.OrchestrationUsage.Registry.Use || mergedProject.OrchestrationUsage.Registry.Use
	mergedProject.Containers = normalizeContainers(append(existing.Containers, mergedProject.Containers...))
	return mergedProject
}

func mergeContainer(existing, incoming *containerInfo) *containerInfo {
	mergedContainer := cloneContainer(incoming)
	mergedContainer.Id = firstNonEmpty(mergedContainer.Id, existing.Id)
	mergedContainer.State = firstNonEmpty(mergedContainer.State, existing.State)
	mergedContainer.ExpectedState = firstNonEmpty(mergedContainer.ExpectedState, existing.ExpectedState)
	if mergedContainer.PortSSH == 0 {
		mergedContainer.PortSSH = existing.PortSSH
	}
	mergedContainer.RemoteUser = firstNonEmpty(mergedContainer.RemoteUser, existing.RemoteUser)
	return mergedContainer
}

func mergeHost(existing, incoming *host) *host {
	mergedHost := cloneHost(incoming)
	mergedHost.Username = firstNonEmpty(mergedHost.Username, existing.Username)
	mergedHost.PathToKey = firstNonEmpty(mergedHost.PathToKey, existing.PathToKey)
	mergedHost.PathToPubKey = firstNonEmpty(mergedHost.PathToPubKey, existing.PathToPubKey)
	mergedHost.UseKey = mergedHost.UseKey || existing.UseKey
	mergedHost.OrchestrationInfo = mergeOrchestrationInfo(existing.OrchestrationInfo, mergedHost.OrchestrationInfo)
	mergedHost.Projects = append([]string{}, existing.Projects...)
	for _, projectName := range incoming.Projects {
		mergedHost.Projects = appendProjectIfMissing(mergedHost.Projects, projectName)
	}
	return mergedHost
}

func mergeFlavour(existing, incoming flavourInfo) flavourInfo {
	return flavourInfo{
		Name:         firstNonEmpty(incoming.Name, existing.Name),
		OverrideDir:  firstNonEmpty(incoming.OverrideDir, existing.OverrideDir),
		LocalConfDir: firstNonEmpty(incoming.LocalConfDir, existing.LocalConfDir),
	}
}

func mergeSrcRepo(existing, incoming srcRepoInfo) srcRepoInfo {
	return srcRepoInfo{
		LocalConfDir: firstNonEmpty(incoming.LocalConfDir, existing.LocalConfDir),
		ToClone:      incoming.ToClone || existing.ToClone,
		URI:          firstNonEmpty(incoming.URI, existing.URI),
		Ref:          firstNonEmpty(incoming.Ref, existing.Ref),
	}
}

func mergeOrchestrationInfo(existing, incoming orchestrationInfo) orchestrationInfo {
	return orchestrationInfo{
		Name: firstNonEmpty(incoming.Name, existing.Name),
		RegistryInfo: registryInfo{
			State: firstNonEmpty(incoming.RegistryInfo.State, existing.RegistryInfo.State),
			Port:  firstNonZero(incoming.RegistryInfo.Port, existing.RegistryInfo.Port),
		},
		State: firstNonEmpty(incoming.State, existing.State),
	}
}

func firstNonEmpty(preferred, fallback string) string {
	if preferred != "" {
		return preferred
	}
	return fallback
}

func firstNonZero(preferred, fallback int) int {
	if preferred != 0 {
		return preferred
	}
	return fallback
}
