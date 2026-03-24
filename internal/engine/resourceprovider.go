package engine

import "github.com/amadeusitgroup/cds/internal/containerconf"

// resourceProvider abstracts external resource access (embedded FS, Artifactory, gRPC stream).
// The engine uses this interface to fetch features, auth files, and Dockerfiles without
// knowing whether they come from a local cache, embedded assets, Artifactory, or a remote client.
type resourceProvider interface {
	// FetchFile returns file content by resource type and identifier.
	FetchFile(resourceType containerconf.ResourceType, identifier string) ([]byte, error)
	// FileExists checks if the file is fetchable
	FileExists(rt containerconf.ResourceType, id string) bool
}
