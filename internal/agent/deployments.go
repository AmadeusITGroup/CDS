package agent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/api/v1/cdspb"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"google.golang.org/protobuf/encoding/protojson"
)

// deploymentStore records, per container, the deploy request that created it
// so that the agent can rebuild the container without the client resending
// its configuration. Staged artifacts remain in the project staging area.
type deploymentStore struct {
	dir string
}

// errDeploymentNotFound is returned when no deployment is recorded for a container.
var errDeploymentNotFound = errors.New("deployment not found")

func (d deploymentStore) path(containerName string) (string, error) {
	if err := validateName("container_name", containerName); err != nil {
		return cg.EmptyStr, err
	}
	return filepath.Join(d.dir, containerName+".json"), nil
}

func (d deploymentStore) save(req *cdspb.DeployContainerRequest) error {
	path, err := d.path(req.GetContainerName())
	if err != nil {
		return err
	}
	data, err := protojson.Marshal(req)
	if err != nil {
		return cerr.AppendErrorFmt("failed to serialize deployment of %s", err, req.GetContainerName())
	}
	if err := os.MkdirAll(d.dir, cg.KPermDir); err != nil {
		return cerr.AppendErrorFmt("failed to create deployments directory %s", err, d.dir)
	}
	if err := os.WriteFile(path, data, cg.KPermFile); err != nil {
		return cerr.AppendErrorFmt("failed to record deployment of %s", err, req.GetContainerName())
	}
	return nil
}

// load returns the deployment recorded for containerName, or an error matching
// errDeploymentNotFound (errors.Is) when there is none.
func (d deploymentStore) load(containerName string) (*cdspb.DeployContainerRequest, error) {
	path, err := d.path(containerName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errDeploymentNotFound
	}
	if err != nil {
		return nil, cerr.AppendErrorFmt("failed to read deployment of %s", err, containerName)
	}
	req := &cdspb.DeployContainerRequest{}
	if err := protojson.Unmarshal(data, req); err != nil {
		return nil, cerr.AppendErrorFmt("failed to parse deployment of %s", err, containerName)
	}
	return req, nil
}

func (d deploymentStore) remove(containerName string) error {
	path, err := d.path(containerName)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return cerr.AppendErrorFmt("failed to remove deployment of %s", err, containerName)
	}
	return nil
}

// rename moves the deployment of containerName, if any, to newContainerName.
func (d deploymentStore) rename(containerName, newContainerName string) error {
	req, err := d.load(containerName)
	if errors.Is(err, errDeploymentNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	req.ContainerName = newContainerName
	if err := d.save(req); err != nil {
		return err
	}
	return d.remove(containerName)
}
