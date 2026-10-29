package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/containerconf"
)

// The staging directory holds, per project, the artifacts uploaded by the
// client for deploys: <stagingDir>/<project>/<normalized identifier>.

func projectStagingRoot(stagingDir, projectName string) (string, error) {
	if err := validateName("project_name", projectName); err != nil {
		return "", err
	}
	return filepath.Join(stagingDir, projectName), nil
}

// stagedArtifactPath normalizes identifier and returns it along with its
// location in the project staging area.
func stagedArtifactPath(stagingDir, projectName, identifier string) (normalized string, path string, err error) {
	root, err := projectStagingRoot(stagingDir, projectName)
	if err != nil {
		return "", "", err
	}

	normalized, err = containerconf.NormalizeIdentifier(identifier)
	if err != nil {
		return "", "", cerr.AppendErrorFmt("invalid artifact identifier %q", err, identifier)
	}

	path = filepath.Join(append([]string{root}, strings.Split(normalized, "/")...)...)

	// defense in depth: NormalizeIdentifier already rejects escaping identifiers
	relativePath, err := filepath.Rel(root, path)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", "", cerr.NewError(fmt.Sprintf("artifact identifier %q escapes the project staging directory", identifier))
	}
	return normalized, path, nil
}

// stagingResourceProvider implements the engine resource provider by reading
// the files staged for a project.
type stagingResourceProvider struct {
	stagingDir  string
	projectName string
}

func (p stagingResourceProvider) FetchFile(rt containerconf.ResourceType, identifier string) ([]byte, error) {
	if rt != containerconf.ResourceTypeFile {
		return nil, cerr.NewError(fmt.Sprintf("staging resource type %d is not supported", rt))
	}

	_, path, err := stagedArtifactPath(p.stagingDir, p.projectName, identifier)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, cerr.AppendErrorFmt("staging resource %q not found", err, identifier)
	}
	return data, nil
}

func (p stagingResourceProvider) FileExists(rt containerconf.ResourceType, identifier string) bool {
	if rt != containerconf.ResourceTypeFile {
		return false
	}

	_, path, err := stagedArtifactPath(p.stagingDir, p.projectName, identifier)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
