package engine

import (
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/containerconf"
)

type emptyProvider struct{}

func newUnimplementedResourceProvider() emptyProvider {
	return emptyProvider{}
}

var _ resourceProvider = emptyProvider{}

func (p emptyProvider) FetchFile(rt containerconf.ResourceType, id string) ([]byte, error) {
	return nil, cerr.NewError("Unimplemented resource provider")
}

func (p emptyProvider) FileExists(rt containerconf.ResourceType, id string) bool {
	return false
}
