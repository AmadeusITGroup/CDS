package ar

import (
	"fmt"
	"io"

	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/artifactory/services/utils"
)

func (ac *artifactoryClient) FetchFile(repository string, path string) ([]byte, error) {
	reader, err := ac.serviceManager.ReadRemoteFile(fmt.Sprintf("%v%v", repository, path))
	if err != nil {
		return nil, cerr.AppendError("Failed reading remote file", err)
	}
	return io.ReadAll(reader)
}

func (ac *artifactoryClient) FileExists(repository, path string) (bool, error) {
	params := services.NewSearchParams()
	params.Pattern = fmt.Sprintf("%v%v", repository, path)
	params.Recursive = true

	reader, err := ac.serviceManager.SearchFiles(params)
	if err != nil {
		return false, cerr.AppendError("Couldn't search for file", err)
	}
	defer func() {
		if reader != nil {
			err = reader.Close()
		}
	}()

	return !reader.IsEmpty(), nil
}

func (ac *artifactoryClient) ListDirectories(repository, path string) ([]string, error) {
	folderInfo, err := ac.serviceManager.FolderInfo(fmt.Sprintf("%v%v", repository, path))
	if err != nil {
		return nil, cerr.AppendError("Couldn't get folder info", err)
	}

	folders := cg.Map(folderInfo.Children, func(child utils.FolderInfoChildren) string { return child.Uri[1:] })

	return folders, nil
}
