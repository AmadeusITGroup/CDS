// contains exported BitbucketClient methods
package scm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

const (
	kBranchRefPrefix    = "refs/heads/"
	kLimitFileInRequest = "10000"
)

// func (bc *bitbucketClient) repositoryExists(projectName string, repositoryName string) (bool, error) {
// 	if bc.err != nil {
// 		return false, cerr.AppendErrorFmt("Failed to check if repository '%s/%s' exists, an error occurred in bitbucket client", bc.err, projectName, repositoryName)
// 	}

// 	url, err := url.Parse(fmt.Sprintf(
// 		"%s/rest/api/1.0/projects/%s/repos/%s",
// 		bc.instancePath,
// 		projectName, repositoryName))

// 	if err != nil {
// 		return false, cerr.AppendError(fmt.Sprintf("Failed to build url to get repository status (%s/%s)", projectName, repositoryName), err)
// 	}

// 	resp, err := bc.httpClient.Get(*url)

// 	if err != nil {
// 		return false, cerr.AppendError(fmt.Sprintf("Failed to get repository status (%s/%s)", projectName, repositoryName), err)
// 	}

// 	if resp.code == http.StatusNotFound {
// 		return false, nil
// 	}

// 	repo := repository{}

// 	if err = json.Unmarshal(resp.body, &repo); err != nil {
// 		return false, cerr.AppendError("Failed to parse response to repository status request", err)
// 	}

// 	return repo.State == "AVAILABLE", nil
// }

// return list of files in a repository at the given path in the repo, default branch & latest commit
func (bc *bitbucketClient) listFiles(projectName string, repositoryName string, filepath string, ref string) ([]string, error) {
	if bc.err != nil {
		return nil, cerr.AppendErrorFmt("Failed to list files in repository '%s/%s:%s' exists, an error occurred in bitbucket client", bc.err, projectName, repositoryName, filepath)
	}

	urlFiles, err := url.Parse(fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/files%s",
		bc.instancePath,
		projectName, repositoryName, filepath,
	))

	urlFilesParams := url.Values{}
	// TODO:Analysis: proper paging handling
	urlFilesParams.Add("limit", kLimitFileInRequest)

	if len(ref) != 0 {
		urlFilesParams.Add("at", fmt.Sprintf("%s%s", kBranchRefPrefix, ref))
	}
	urlFiles.RawQuery = urlFilesParams.Encode()
	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to build url to list of files for repo %s/%s", projectName, repositoryName), err)
	}

	resp, err := bc.httpClient.Get(*urlFiles)
	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to list of files for repo %s/%s", projectName, repositoryName), err)
	}

	if resp.code != http.StatusOK {
		return nil, cerr.NewError(fmt.Sprintf("Failed to list of files for repo %s/%s, HTTP error code %v", projectName, repositoryName, resp.code))
	}

	fileList := RepositoryFileList{}

	if err = json.Unmarshal(resp.body, &fileList); err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("failed to decode list of files for repo %s/%s", projectName, repositoryName), err)
	}

	return fileList.Values, nil
}

func (bc *bitbucketClient) getFileType(projectName, repositoryName, filepath, ref string) (string, error) {
	if bc.err != nil {
		return "", cerr.AppendErrorFmt("Failed to get file type in repository '%s/%s:%s' exists, an error occurred in bitbucket client", bc.err, projectName, repositoryName, filepath)
	}

	urlFile, err := url.Parse(fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/browse/%s",
		bc.instancePath,
		projectName, repositoryName, filepath))
	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to build url to get file type for repo %s/%s", projectName, repositoryName), err)
	}

	urlFileParams := url.Values{}
	urlFileParams.Add("type", "true")

	if len(ref) != 0 {
		urlFileParams.Add("at", fmt.Sprintf("%s%s", kBranchRefPrefix, ref))
	}
	urlFile.RawQuery = urlFileParams.Encode()

	resp, err := bc.httpClient.Get(*urlFile)
	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to get file type for repo %s/%s", projectName, repositoryName), err)
	}

	if resp.code != http.StatusOK {
		return "", cerr.NewError(fmt.Sprintf("Failed to get file type for repo %s/%s, HTTP error code %v", projectName, repositoryName, resp.code))
	}

	fileType := RepositoryFileType{}
	if err = json.Unmarshal(resp.body, &fileType); err != nil {
		return "", cerr.AppendError(fmt.Sprintf("failed to decode file type for repo %s/%s", projectName, repositoryName), err)
	}

	return fileType.FileType, nil
}

// return list of files in a repository, default branch & latest commit
func (bc *bitbucketClient) fetchFile(projectName string, repositoryName string, filepath string, ref string) (string, error) {
	if bc.err != nil {
		return "", cerr.AppendErrorFmt("Failed to fetch file in repository '%s/%s:%s' exists, an error occurred in bitbucket client", bc.err, projectName, repositoryName, filepath)
	}

	urlFile, err := url.Parse(fmt.Sprintf(
		"%s/rest/api/1.0/projects/%s/repos/%s/raw/%s",
		bc.instancePath,
		projectName, repositoryName, filepath))

	if len(ref) != 0 {
		urlFileParams := url.Values{}
		urlFileParams.Add("at", fmt.Sprintf("%s%s", kBranchRefPrefix, ref))
		urlFile.RawQuery = urlFileParams.Encode()
	}

	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to build url to fetch file %s/%s/%s", projectName, repositoryName, filepath), err)
	}

	resp, err := bc.httpClient.Get(*urlFile)

	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to fetch file from bitbucket %s/%s/%s", projectName, repositoryName, filepath), err)
	}

	if bbErr := resp.parseError(); bbErr != nil {
		return "", cerr.AppendError(fmt.Sprintf("Failed to fetch file from bitbucket %s/%s/%s", projectName, repositoryName, filepath), bbErr)
	}

	return string(resp.body), nil
}

// make a request to validate that the given credentials are valid
// for now, we get the user's information, which doesn't have any special access permissions
// TODO:refactor: Shouldn't be public
func (bc *bitbucketClient) ValidateAuthentication() (bool, error) {
	if bc.err != nil {
		return false, cerr.AppendError("Failed to validate authentication to bitbucket, an error occurred in bitbucket client", bc.err)
	}

	url, err := url.Parse(fmt.Sprintf(
		"%s/rest/api/1.0/users/%s",
		bc.instancePath, bc.getUsername()))

	if err != nil {
		return false, cerr.AppendError(fmt.Sprintf("Failed to build url to check bitbucket credentials %s", bc.getUsername()), err)
	}

	resp, err := bc.httpClient.Get(*url)

	if err != nil {
		return false, cerr.AppendError("Failed to determine credential validity, unable to perform a request against bitbucket !", err)
	}

	switch resp.code {
	case http.StatusOK, http.StatusNoContent:
		userInfo := bitbucketUserResponse{}
		err = json.Unmarshal(resp.body, &userInfo)

		if err != nil {
			return false, cerr.AppendError("Failed to decode response to authentication request", err)
		}

		if !userInfo.Active {
			return false, cerr.NewError("Specified user is not active on Bitbucket !")
		}
		return true, nil
	case http.StatusUnauthorized:
		return false, cerr.NewError("Failed to validate bitbucket credentials, HTTP code was 401")
	default:
		clog.Debug("Made GET request but HTTP code was", resp.code, ", server responded:", string(resp.body))
		return false, cerr.NewError(fmt.Sprintf("Failed to make HTTP GET request to validate credentials, HTTP code was %v", resp.code))
	}
}

func (bc *bitbucketClient) shallowClone(repoUrl, branch, cloneDir string) error {
	if bc.err != nil {
		return cerr.AppendErrorFmt("Failed to fetch file in repository '%s@%s' exists, an error occurred in bitbucket client", bc.err, repoUrl, branch)
	}

	_, errClone := git.PlainClone(cloneDir, false, &git.CloneOptions{
		URL:           repoUrl,
		SingleBranch:  true,
		Auth:          bc.getBitbucketAuthMethod(),
		ReferenceName: plumbing.NewBranchReferenceName(branch),
	})

	if errClone != nil {
		return cerr.AppendError("Failed to shallow clone bitbucket directory", errClone)
	}

	return nil
}

func (bc *bitbucketClient) getCommits(projectName, repositoryName, ref, since, path string) ([]bitbucketCommit, error) {
	if bc.err != nil {
		return nil, cerr.AppendErrorFmt("Failed to fetch commits in repository '%s/%s:%s', an error occurred in bitbucket client", bc.err, projectName, repositoryName, ref)
	}

	urlQuery, err := url.Parse(fmt.Sprintf(
		//until=refs/heads/develop&limit=0&start=0&path=containers/mvn
		"%s/rest/api/1.0/projects/%s/repos/%s/commits?&limit=0&start=0",
		bc.instancePath,
		projectName,
		repositoryName,
	))

	urlParams := url.Values{}

	if path != "" {
		urlParams.Add("path", path)
	}

	if ref == "" {
		ref = "HEAD"
		urlParams.Add("until", ref)
	}

	if since != "" {
		urlParams.Add("since", since)
	}

	urlQuery.RawQuery = urlParams.Encode()

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to build url to fetch last commit %s/%s:%s", projectName, repositoryName, ref), err)
	}

	resp, err := bc.httpClient.Get(*urlQuery)

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to fetch last commit from bitbucket %s/%s:%s", projectName, repositoryName, ref), err)
	}

	if resp.code != http.StatusOK {
		return nil, cerr.NewError(fmt.Sprintf("Failed to get last commit for repo %s/%s, HTTP error code %v", projectName, repositoryName, resp.code))
	}

	commitQuery := bitbucketCommitSearch{}

	if err = json.Unmarshal(resp.body, &commitQuery); err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("failed to decode last commit for repo %s/%s", projectName, repositoryName), err)
	}

	return commitQuery.Values, nil
}
