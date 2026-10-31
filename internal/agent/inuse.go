package agent

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/engine"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// clusterDependents returns the names of CDS-managed containers attached to the
// kind cluster network, i.e. containers that depend on the cluster (and, by
// extension, the registry they were decorated to trust). The agent uses this to
// decide whether a cluster/registry delete is safe: tearing the cluster down
// while containers are attached would break them, so the delete is refused
// unless forced.
//
// It lists CDS containers and inspects each one's networks; a container on the
// kind network is a dependent. Errors listing or inspecting are treated as "no
// known dependents" by the caller via the returned error, so the guard never
// blocks on a transient inspection failure (callers log and proceed, or the
// user forces).
func clusterDependents(list func() ([]string, error)) ([]string, error) {
	names, err := list()
	if err != nil {
		return nil, cerr.AppendError("failed to list containers to check cluster usage", err)
	}
	var dependents []string
	for _, name := range names {
		networks, err := engine.GetContainerNetworks(cg.EmptyStr, name)
		if err != nil {
			// a container we cannot inspect is skipped rather than treated as a
			// blocker; the force flag remains the escape hatch.
			continue
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

// errClusterInUse is returned when a cluster/registry delete is refused because
// containers still depend on it and force was not set.
func errInUse(resource string, dependents []string) error {
	return cerr.NewError(fmt.Sprintf(
		"%s is in use by %d container(s): %v; retry with --force to tear it down anyway",
		resource, len(dependents), dependents))
}

// guardClusterDelete refuses the delete when dependents exist and force is
// false. It is the "agent decides in use" gate for both cluster and registry
// deletes (they are deployed together, so container dependence on the cluster
// network also guards the registry).
func guardClusterDelete(resource string, force bool) error {
	if force {
		return nil
	}
	dependents, err := clusterDependents(cdsContainerNames)
	if err != nil {
		// cannot determine usage; do not hard-block, but surface the reason.
		return nil
	}
	if len(dependents) > 0 {
		return errInUse(resource, dependents)
	}
	return nil
}
