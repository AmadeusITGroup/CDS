package orchestration

import (
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// Provider returns the cluster provider for the given orchestration engine
// name. It is the Strategy seam: kind is the only provider today, but adding
// another (k3s, a remote cluster, ...) is a new case here, with no change to
// the gRPC service or the agent coordinator. An empty engine name defaults to
// kind.
func Provider(engineName string) (ClusterProvider, error) {
	switch engineName {
	case cg.EmptyStr, kindEngineName:
		return NewKindProvider(), nil
	default:
		return nil, cerr.NewError("unsupported orchestration engine: " + engineName)
	}
}
