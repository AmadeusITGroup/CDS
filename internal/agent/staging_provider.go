package agent

import (
	"fmt"
	"os"

	"github.com/amadeusitgroup/cds/internal/containerconf"
)

// stagingResourceProvider implements the engine's resourceProvider interface
// by reading files from the artifact staging directory populated by UploadArtifact.
type stagingResourceProvider struct {
	stagingDir  string
	projectName string
}

func newStagingResourceProvider(stagingDir, projectName string) stagingResourceProvider {
	return stagingResourceProvider{
		stagingDir:  stagingDir,
		projectName: projectName,
	}
}

func (p stagingResourceProvider) path(identifier string) (string, error) {
	artifact, err := resolveProjectArtifact(p.stagingDir, p.projectName, identifier)
	if err != nil {
		return "", err
	}
	return artifact.path, nil
}

func (p stagingResourceProvider) FetchFile(rt containerconf.ResourceType, identifier string) ([]byte, error) {
	if rt != containerconf.ResourceTypeFile {
		return nil, fmt.Errorf("staging resource type %d is not supported", rt)
	}

	path, err := p.path(identifier)
	if err != nil {
		return nil, fmt.Errorf("invalid staging resource %q: %w", identifier, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("staging resource %q not found: %w", identifier, err)
	}
	return data, nil
}

func (p stagingResourceProvider) FileExists(rt containerconf.ResourceType, identifier string) bool {
	if rt != containerconf.ResourceTypeFile {
		return false
	}

	path, err := p.path(identifier)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
