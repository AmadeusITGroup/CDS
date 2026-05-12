package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/containerconf"
)

const stagingDirName = "staging"

// stagedArtifact binds a shared logical identifier to the project-scoped cache
// location used by the agent.
type stagedArtifact struct {
	identifier string
	path       string
}

func defaultArtifactStagingDir() string {
	return filepath.Join(cenv.CacheDir(), stagingDirName)
}

func resolveProjectArtifact(stagingDir, projectName, identifier string) (stagedArtifact, error) {
	if err := validateAgentName("project_name", projectName); err != nil {
		return stagedArtifact{}, err
	}

	normalized, err := containerconf.NormalizeIdentifier(identifier)
	if err != nil {
		return stagedArtifact{}, err
	}

	root := filepath.Join(stagingDir, projectName)
	segments := strings.Split(normalized, "/")
	resolvedPath := filepath.Join(append([]string{root}, segments...)...)

	relativePath, err := filepath.Rel(root, resolvedPath)
	if err != nil {
		return stagedArtifact{}, fmt.Errorf("resolve artifact staging path for %q: %w", identifier, err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return stagedArtifact{}, fmt.Errorf("artifact identifier %q escapes the project staging directory", identifier)
	}

	return stagedArtifact{
		identifier: normalized,
		path:       resolvedPath,
	}, nil
}
