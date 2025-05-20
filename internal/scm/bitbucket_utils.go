package scm

import (
	"fmt"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

// define helper function for bitbucket, bitbucket.go contains only primitives

type DirContentList struct {
	FileNames      []string
	DirectoryNames []string
}

// list content of a directory on Bitbucket
// TODO: refactory: Shouldn't be public as Public functions should be owned by Repository
func (bc *bitbucketClient) ListDirectory(project string, repo string, filepath string, ref string) (DirContentList, error) {
	containerBlob, errFetch := bc.fetchFile(project, repo, filepath, ref)

	if errFetch != nil {
		return DirContentList{}, cerr.AppendError("Failed get list of features from Bitbucket", errFetch)
	}

	filesNames := []string{}
	directoryNames := []string{}

	for _, entry := range strings.Split(containerBlob, "\n") {
		if len(entry) == 0 {
			continue
		}
		// example of entry: "040000 tree 37fd16710b7cee37064e98495f3ea8411320609b    mvn"
		var perms, entryType, hash, name string
		_, errParse := fmt.Sscanf(entry, "%s %s %s\t%s", &perms, &entryType, &hash, &name)

		if errParse != nil {
			return DirContentList{}, cerr.AppendError("Failed parse list of features from Bitbucket", errParse)
		}

		switch entryType {
		case "tree":
			directoryNames = append(directoryNames, name)
		case "blob":
			filesNames = append(filesNames, name)
		default:
			clog.Debug(fmt.Sprintf("Unknown file type found during list of flavor (type: %s, filename: %s)", entryType, name))
		}
	}

	return DirContentList{filesNames, directoryNames}, nil
}

// return list of files in a repository, default branch & latest commit
// func (bc *bitbucketClient) listFilesRepo(project string, repo string) ([]string, error) {
// 	return bc.listFiles(project, repo, "", "")
// }
