package command

import (
	"context"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/containerconf"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/host"
)

// sshTarget is what shexec needs to open an SSH session.
type sshTarget interface {
	FQDN() string
	HasPassword() bool
	Password() string
	PathToPrv() string
	PathToPub() string
	Port() int
	Username() string
}

// projectPrimaryContainer returns the container users access in the project.
func projectPrimaryContainer(projectName string) (string, error) {
	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("Project %q has no containers.\n%s", projectName, getTipRun(projectName)))
	}
	return containers[0], nil
}

// syncedProjectPrimaryContainer syncs the project with its agent and returns
// its primary container.
func syncedProjectPrimaryContainer(projectName string) (string, error) {
	if err := onProjectAgent(projectName, func(services agentServices, ctx context.Context) error {
		return syncProjectContainers(ctx, services.container, projectName)
	}); err != nil {
		return cg.EmptyStr, err
	}
	return projectPrimaryContainer(projectName)
}

// containerSSHTarget returns the SSH target of the container: its SSH port
// published on the agent host, accessed as user (the container remote user
// when empty) with the user default key pair installed at deploy time.
func containerSSHTarget(projectName, containerName, user string) (sshTarget, error) {
	port := db.ContainerSSHPort(projectName, containerName)
	if port <= 0 {
		return nil, cerr.NewError(fmt.Sprintf("Container %q does not publish its SSH port", containerName))
	}
	if user == cg.EmptyStr {
		user = db.ProjectContainerRemoteUser(projectName, containerName)
	}
	if user == cg.EmptyStr {
		return nil, cerr.NewError(fmt.Sprintf("Container %q does not have a configured remote user", containerName))
	}
	privateKey, publicKey, found := containerconf.DefaultSSHKeyPair()
	if !found {
		return nil, cerr.NewError("No SSH key pair found (~/.ssh/id_rsa or ~/.ssh/id_ed25519) to access containers")
	}

	return host.New(
		host.WithName(projectAgentHost(projectName)),
		host.WithUsername(user),
		host.WithPort(port),
		host.WithKeyPair(host.NewKeyPair(
			host.WithPathToPrv(privateKey),
			host.WithPathToPub(publicKey),
		)),
	), nil
}

// executeInContainer runs command in the container, failing on a non-zero exit code.
func executeInContainer(ctx context.Context, client cdspb.ContainerServiceClient, containerName, command, user string) (string, error) {
	response, err := client.ExecuteInContainer(ctx, &cdspb.ExecuteInContainerRequest{
		ContainerName: containerName,
		Command:       command,
		User:          user,
	})
	if err != nil {
		return cg.EmptyStr, cerr.AppendErrorFmt("Failed to execute command in container %s", err, containerName)
	}
	if response.GetExitCode() != 0 {
		return response.GetOutput(), cerr.NewError(fmt.Sprintf("command in container %s failed with exit code %d: %s", containerName, response.GetExitCode(), response.GetOutput()))
	}
	return response.GetOutput(), nil
}
