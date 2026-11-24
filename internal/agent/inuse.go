package agent

import (
	"errors"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// clusterDependents returns the names of CDS-managed containers attached to the
// kind cluster network, i.e. containers that depend on the cluster (and, by
// extension, the registry they were decorated to trust). The agent uses this to
// decide whether a cluster/registry delete is safe: tearing the cluster down
// while containers are attached would break them, so the delete is refused
// unless forced.
//
// It lists CDS containers and inspects each one's networks; a container on the
// kind network is a dependent. A listing or inspection error is returned: the
// delete cannot be undone, so the guard refuses it when usage is unknown and
// the force flag stays the escape hatch.
func clusterDependents(list func() ([]string, error), networksOf func(string) ([]string, error)) ([]string, error) {
	names, err := list()
	if err != nil {
		return nil, cerr.AppendError("failed to list containers to check cluster usage", err)
	}
	var dependents []string
	for _, name := range names {
		networks, err := networksOf(name)
		if err != nil {
			return nil, cerr.AppendError(fmt.Sprintf("failed to inspect container %s to check cluster usage", name), err)
		}
		for _, n := range networks {
			if n == engine.KindNetworkName {
				dependents = append(dependents, name)
				break
			}
		}
	}
	return dependents, nil
}

// cdsContainerNames lists the names of CDS-managed containers on the host.
func cdsContainerNames() ([]string, error) {
	containers, err := defaultContainerOps{}.ListContainers()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(containers))
	for _, c := range containers {
		names = append(names, string(c.Name))
	}
	return names, nil
}

// inUseError marks a delete refused because containers still depend on the
// resource: a refusal of the request, not an agent fault (CDS-032).
type inUseError struct{ error }

// errInUse is returned when a cluster/registry delete is refused because
// containers still depend on it and force was not set.
func errInUse(resource string, dependents []string) error {
	return inUseError{cerr.NewError(fmt.Sprintf(
		"%s is in use by %d container(s): %v; retry with --force to tear it down anyway",
		resource, len(dependents), dependents))}
}

// deleteStatus maps a delete failure to its gRPC status: FailedPrecondition
// when it was refused as in use, so clients can tell it from an outage.
func deleteStatus(what string, err error) error {
	code := codes.Internal
	if errors.As(err, new(inUseError)) {
		code = codes.FailedPrecondition
	}
	return status.Errorf(code, "%s: %v", what, err)
}

// guardClusterDelete refuses the delete when dependents exist and force is
// false. It is the "agent decides in use" gate for both cluster and registry
// deletes (they are deployed together, so container dependence on the cluster
// network also guards the registry).
func guardClusterDelete(resource string, force bool) error {
	return guardDelete(resource, force, func() ([]string, error) {
		return clusterDependents(cdsContainerNames, containerNetworks)
	})
}

// guardDelete is guardClusterDelete with the dependents lookup injected. When
// usage cannot be determined the delete is refused, never let through.
func guardDelete(resource string, force bool, dependentsOf func() ([]string, error)) error {
	if force {
		return nil
	}
	dependents, err := dependentsOf()
	if err != nil {
		return cerr.AppendError(fmt.Sprintf(
			"cannot tell whether the %s is in use, so it is not deleted; retry with --force to tear it down anyway",
			resource), err)
	}
	if len(dependents) > 0 {
		return errInUse(resource, dependents)
	}
	return nil
}

// containerNetworks returns the networks a host container is attached to.
func containerNetworks(name string) ([]string, error) {
	return engine.GetContainerNetworks(cg.EmptyStr, name)
}
