package command

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// The project state (containers, SSH config) mirrors what the agent reports.

func listAgentContainers(ctx context.Context, client cdspb.ContainerServiceClient) (map[string]*cdspb.Container, error) {
	response, err := client.ListContainers(ctx, &cdspb.ListContainerRequest{})
	if err != nil {
		return nil, cerr.AppendError("Failed to list containers from agent", err)
	}

	containers := make(map[string]*cdspb.Container, len(response.GetContainers()))
	for _, container := range response.GetContainers() {
		containers[container.GetName()] = container
	}
	return containers, nil
}

// syncProjectContainers aligns the project containers with the agent: known
// containers are updated, the ones absent on the agent host are forgotten.
func syncProjectContainers(ctx context.Context, client cdspb.ContainerServiceClient, projectName string) error {
	containerNames := db.ProjectContainersName(projectName)
	if len(containerNames) == 0 {
		clog.Info("No containers found in configuration, nothing to sync.")
		return nil
	}

	agentContainers, err := listAgentContainers(ctx, client)
	if err != nil {
		return err
	}

	for _, containerName := range containerNames {
		container, found := agentContainers[containerName]
		if !found {
			clog.Warn(fmt.Sprintf("Container %q is absent on agent host, removing it from the project.", containerName))
			if err := forgetProjectContainer(projectName, containerName); err != nil {
				return err
			}
			continue
		}
		// the listing is a summary, the container details (port mapping) are fetched
		remoteUser := db.ProjectContainerRemoteUser(projectName, containerName)
		if err := recordProjectContainer(ctx, client, projectName, containerName, remoteUser, bo.SContainerStatus(container.GetStatus())); err != nil {
			return err
		}
	}
	return nil
}

// changeProjectContainersState applies change to every project container
// whose status differs from target, then records the new statuses.
func changeProjectContainersState(ctx context.Context, client cdspb.ContainerServiceClient, projectName string, target bo.ContainerStatus, change func(containerName string) error) error {
	containerNames := db.ProjectContainersName(projectName)
	if len(containerNames) == 0 {
		return cerr.NewError(fmt.Sprintf("Project %q has no containers configured.\n%s", projectName, getTipRun(projectName)))
	}

	agentContainers, err := listAgentContainers(ctx, client)
	if err != nil {
		return err
	}

	for _, containerName := range containerNames {
		container, found := agentContainers[containerName]
		if !found {
			return cerr.NewError(fmt.Sprintf("Container %q was not found on agent host.\n%s", containerName, getTipSync(projectName)))
		}
		if bo.SContainerStatus(container.GetStatus()) == target {
			clog.Warn(fmt.Sprintf("Container %q is already %s. Skipping.", containerName, target.ToString()))
		} else if err := change(containerName); err != nil {
			return cerr.AppendErrorFmt("Failed to change state of container %s", err, containerName)
		}
		if err := recordProjectContainer(ctx, client, projectName, containerName, db.ProjectContainerRemoteUser(projectName, containerName), target); err != nil {
			return err
		}
	}
	return nil
}

// clearProjectContainers deletes the project containers on the agent host and
// forgets them, along with the artifacts staged for the project.
func clearProjectContainers(ctx context.Context, client cdspb.ContainerServiceClient, projectName string) error {
	containerNames := db.ProjectContainersName(projectName)
	if len(containerNames) == 0 {
		clog.Warn("No containers found in configuration, nothing to clear.")
		return nil
	}

	agentContainers, err := listAgentContainers(ctx, client)
	if err != nil {
		return err
	}

	var errs []error
	for _, containerName := range containerNames {
		if _, found := agentContainers[containerName]; !found {
			clog.Warn(fmt.Sprintf("Container %q is already absent on agent host. Skipping.", containerName))
		} else if _, err := client.DeleteContainer(ctx, &cdspb.DeleteContainerRequest{ContainerName: containerName, ProjectName: projectName}); err != nil {
			errs = append(errs, cerr.AppendErrorFmt("Failed to delete container %s", err, containerName))
			continue
		}
		if err := forgetProjectContainer(projectName, containerName); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return cerr.AppendMultipleErrors("Failed to clear all project containers", errs)
	}
	return nil
}

// recordProjectContainer records the container as reported by the agent, with
// the status the user expects it to be in.
func recordProjectContainer(ctx context.Context, client cdspb.ContainerServiceClient, projectName, containerName, remoteUser string, expected bo.ContainerStatus) error {
	response, err := client.GetContainer(ctx, &cdspb.GetContainerRequest{ContainerName: containerName})
	if err != nil {
		return cerr.AppendErrorFmt("Failed to get container %s", err, containerName)
	}

	info := protoContainerToBO(response.GetContainer(), remoteUser)
	info.ExpectedStatus = expected
	return saveProjectContainer(projectName, info)
}

// saveProjectContainer records the container in the project and its SSH access in the SSH config.
func saveProjectContainer(projectName string, info bo.Container) error {
	if err := db.UpsertProjectContainer(projectName, info); err != nil {
		return err
	}
	return upsertContainerSSHConfig(projectAgentHost(projectName), info)
}

// forgetProjectContainer removes the container from the project and the SSH config.
func forgetProjectContainer(projectName, containerName string) error {
	if err := db.RemoveProjectContainers(projectName, containerName); err != nil {
		return err
	}
	return removeContainerSSHConfig(containerName)
}

func protoContainerToBO(container *cdspb.Container, remoteUser string) bo.Container {
	portMapping := make(bo.PortMapping, len(container.GetPortMapping()))
	for containerPort, hostPort := range container.GetPortMapping() {
		portMapping[containerPort] = int(hostPort)
	}
	status := bo.SContainerStatus(container.GetStatus())
	return bo.Container{
		Id:             bo.ContainerID(container.GetId()),
		Name:           bo.ContainerName(container.GetName()),
		Pmapping:       portMapping,
		Status:         status,
		ExpectedStatus: status,
		RemoteUser:     bo.ContainerRemoteUser(remoteUser),
	}
}

func getTipSync(projectName string) string {
	extras := cg.EmptyStr
	if !db.IsCurrentProject(projectName) {
		extras = " " + projectName
	}
	return fmt.Sprintf(`  (use "cds project sync%s" to align the project with the agent host)`, extras)
}
