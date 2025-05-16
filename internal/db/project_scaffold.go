package db

import (
	"fmt"
	"os"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/cos"
)

func GetProjectsUsingConfig(path string) []string {
	instance().Lock()
	defer instance().Unlock()
	projectNames := []string{}
	pathInfo, _ := cos.Fs.Stat(path)
	for _, proj := range instance().d.Projects {
		projInfo, _ := cos.Fs.Stat(proj.ConfDir)
		if os.SameFile(pathInfo, projInfo) {
			projectNames = append(projectNames, proj.Name)
		}
	}
	return projectNames
}

func DeleteProject(projectName string) (string, error) {
	instance().Lock()
	defer instance().Unlock()
	project, err := instance().d.getProject(projectName)
	if err != nil {
		return "", cerr.AppendErrorFmt(fmt.Sprintf("Failed to delete project %s: project not found", projectName), err)
	}
	var pathToDelete string
	if len(project.ConfDir) == 0 {

		switch {
		case len(project.Flavour.LocalConfDir) != 0:
			pathToDelete = project.Flavour.LocalConfDir
		case len(project.SrcRepo.LocalConfDir) != 0:
			pathToDelete = project.SrcRepo.LocalConfDir
		}
	}
	instance().d.removeProjectFromList(projectName)
	return pathToDelete, nil
}
