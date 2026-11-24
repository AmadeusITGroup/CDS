package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
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
// its primary container. It fails fast when the project has no container
// recorded, so commands like ssh/rsh/expose do not bootstrap an agent just to
// discover there is nothing to connect to.
func syncedProjectPrimaryContainer(projectName string) (string, error) {
	if len(db.ProjectContainersName(projectName)) == 0 {
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("Project %q has no containers.\n%s", projectName, getTipRun(projectName)))
	}
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
	privateKey, publicKey, err := ensureProjectContainerKeyPair(projectName)
	if err != nil {
		return nil, cerr.AppendError("Failed to resolve an SSH key pair to access containers", err)
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

// kSSHAuthFailure is the text golang.org/x/crypto/ssh puts in a handshake error
// when the server rejects every offered key; the library has no typed error.
const kSSHAuthFailure = "unable to authenticate"

// withRedeployTip adds a redeploy tip to err when the container rejected the
// project key. Containers deployed before the per-host key (id_rsa_<host>) only
// trust the old default key; by owner ruling (CDS-021) the client offers no
// fallback key, so the fix is to redeploy the container once.
func withRedeployTip(projectName string, err error) error {
	if err == nil || !isSSHAuthFailure(err) {
		return err
	}
	return cerr.AppendError("The container rejected the project SSH key.\n"+getTipRedeploy(projectName), err)
}

// isSSHAuthFailure reports whether any message in err's chain is an SSH
// authentication failure. It walks the chain so it does not depend on the
// cerr error detail setting.
func isSSHAuthFailure(err error) bool {
	ce, ok := err.(*cerr.Err)
	if !ok {
		return strings.Contains(err.Error(), kSSHAuthFailure)
	}
	for ; ce != nil; ce = ce.Cause {
		if strings.Contains(ce.Message, kSSHAuthFailure) {
			return true
		}
	}
	return false
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
