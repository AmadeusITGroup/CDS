package command

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/db"
	"github.com/amadeusitgroup/cds/internal/host"
)

type projectSSHTarget interface {
	FQDN() string
	HasPassword() bool
	Password() string
	PathToPrv() string
	PathToPub() string
	Port() int
	Username() string
}

func syncProjectContainers(projectName string) error {
	containers := db.ProjectContainersName(projectName)
	return withProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		return syncProjectContainersFromAgent(ctx, services.container, projectName, containers, false)
	})
}

func projectPrimaryContainer(projectName string) (string, string, error) {
	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return "", "", cerr.NewError(fmt.Sprintf("Project %q has no containers configured", projectName))
	}

	containerName := containers[0]
	remoteUser := db.ProjectContainerRemoteUser(projectName, containerName)
	if remoteUser == "" {
		return "", "", cerr.NewError(fmt.Sprintf("Container %q does not have a configured remote user", containerName))
	}
	return containerName, remoteUser, nil
}

func projectContainerSSHTarget(projectName string) (projectSSHTarget, string, error) {
	containerName, remoteUser, err := projectPrimaryContainer(projectName)
	if err != nil {
		return nil, "", err
	}

	port := db.ContainerSSHPort(projectName, containerName)
	if port <= 0 {
		return nil, "", cerr.NewError(fmt.Sprintf("Container %q does not expose an SSH port", containerName))
	}

	privateKeyPath, publicKeyPath, err := projectSSHKeyPair(projectName)
	if err != nil {
		return nil, "", err
	}

	return host.New(
		host.WithName(projectAgentHost(projectName)),
		host.WithUsername(remoteUser),
		host.WithPort(port),
		host.WithKeyPair(host.NewKeyPair(
			host.WithPathToPrv(privateKeyPath),
			host.WithPathToPub(publicKeyPath),
		)),
	), containerName, nil
}

func executeProjectContainerCommand(ctx context.Context, client cdspb.ContainerServiceClient, projectName, command, user string) (string, error) {
	containerName, _, err := projectPrimaryContainer(projectName)
	if err != nil {
		return "", err
	}

	response, err := client.ExecuteInContainer(ctx, &cdspb.ExecuteInContainerRequest{
		ContainerName: containerName,
		Command:       command,
		User:          user,
	})
	if err != nil {
		return "", cerr.AppendErrorFmt("Failed to execute command in container %s", err, containerName)
	}
	if response.GetExitCode() != 0 {
		return response.GetOutput(), cerr.NewError(fmt.Sprintf("command in container %s failed with exit code %d: %s", containerName, response.GetExitCode(), response.GetOutput()))
	}
	return response.GetOutput(), nil
}

func setProjectContainerFromAgentCurrentStatus(ctx context.Context, client cdspb.ContainerServiceClient, projectName, containerName, remoteUser string) error {
	response, err := client.GetContainer(ctx, &cdspb.GetContainerRequest{ContainerName: containerName})
	if err != nil {
		return cerr.AppendErrorFmt("Failed to get container %s", err, containerName)
	}

	container := protoContainerToBO(response.GetContainer(), remoteUser)
	if err := db.UpsertProjectContainer(projectName, container); err != nil {
		return err
	}
	return upsertProjectContainerSSHConfig(projectName, container, false)
}
