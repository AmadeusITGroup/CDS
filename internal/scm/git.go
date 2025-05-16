package scm

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/go-git/go-git/v5"
)

type scmInstance interface {
	error
	Name() string
	HttpUrl() string
	SshUrl() string
	// getter on client singletons, will create a new client
	// if no client is available for this instance
	// TODO: refactor: Client shouldn't be accessed directly
	GetClient() *bitbucketClient
}

// type scmClient interface {
// }

type GitRepository interface {
	Name() string
	GetFile(repoPath string, ref string) (string, error)
	// list all files under path
	ListFiles(path string, ref string) ([]string, error)
	// shallow clone repository locally
	ShallowClone(path string, ref string) error
	// get a valid https url that git recognize
	GetGitHttpUrl() string
}

const (
	kGitmoduleFilename = ".gitmodules"
)

func ParseGitRepositoryUrl(repoUrl string) (GitRepository, error) {
	br, err := parseBitbucketUrl(repoUrl)
	if err == nil {
		return br, nil
	}

	clog.Debug(fmt.Sprintf("Given git repository (%s) is not a bitbucket repository", repoUrl))

	// other scm ...

	return nil, cerr.NewError(fmt.Sprintf("Failed to identify git repository '%s'", repoUrl))
}

func GetLocalGitRepoHeadHash(path string) (string, error) {
	repo, errOpen := git.PlainOpen(path)
	if errOpen != nil {
		return "", cerr.AppendError("Failed to open cloned git repository", errOpen)
	}

	head, errHead := repo.Head()
	if errHead != nil {
		return "", cerr.AppendError("Failed to get cloned repo head", errHead)
	}

	return head.Hash().String(), nil
}
