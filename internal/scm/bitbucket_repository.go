package scm

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"gopkg.in/ini.v1"
)

func getUrlRegexMatches(path, scheme string) (*regexp.Regexp, []string, error) {
	var subMatches []string
	var regexMatch *regexp.Regexp

	switch scheme {
	case "ssh":
		if !sshUrlRegex.MatchString(path) {
			return &regexp.Regexp{}, nil, cerr.NewError(fmt.Sprintf("Failed to parse project/repository for given path (%s)", path))
		}

		subMatches = sshUrlRegex.FindStringSubmatch(path)
		regexMatch = sshUrlRegex

	case "https":
		switch {
		case httpUrlGitRegex.MatchString(path):
			subMatches = httpUrlGitRegex.FindStringSubmatch(path)
			regexMatch = httpUrlGitRegex
		case httpUrlUiRegex.MatchString(path):
			subMatches = httpUrlUiRegex.FindStringSubmatch(path)
			regexMatch = httpUrlUiRegex
		case userUrlUiRegex.MatchString(path):
			subMatches = userUrlUiRegex.FindStringSubmatch(path)
			regexMatch = userUrlUiRegex
		default:
			return &regexp.Regexp{}, nil, cerr.NewError(fmt.Sprintf("Failed to parse project/repository for given path (%s)", path))
		}

	default:
		return &regexp.Regexp{}, nil, cerr.NewError(fmt.Sprintf("Unhandled transport scheme for bitbucket repository (%s)", scheme))
	}
	return regexMatch, subMatches, nil
}

func parseRepoPath(path, scheme string) (string, string, error) {
	var project, repository string
	regexMatch, subMatches, err := getUrlRegexMatches(path, scheme)
	if err != nil {
		return "", "", cerr.AppendError("Url regex matching failed", err)
	}

	indexProj := regexMatch.SubexpIndex(kProject)
	indexRepo := regexMatch.SubexpIndex(kRepository)
	indexUser := regexMatch.SubexpIndex(kUsername)

	if (indexProj == -1 && indexUser == -1) || indexRepo == -1 {
		return "", "", cerr.NewError(fmt.Sprintf("Failed to parse project/user/repository for given path with regex (%s)", scheme))
	}

	if indexProj != -1 {
		project = subMatches[indexProj]
		repository = subMatches[indexRepo]
	}

	if indexUser != -1 {
		project = "~" + subMatches[indexUser]
		repository = subMatches[indexRepo]
	}

	// remove trailing .git if present
	repository = strings.TrimSuffix(repository, ".git")
	return project, repository, nil
}

func parseFilePathFromUrl(path, scheme string) (string, error) {
	var subMatches []string
	var regexMatch *regexp.Regexp

	regexMatch, subMatches, err := getUrlRegexMatches(path, scheme)

	if err != nil {
		return "", cerr.AppendError("Url regex matching failed", err)
	}

	indexFilePath := regexMatch.SubexpIndex("FilePath")
	if indexFilePath == -1 {
		return "", cerr.NewError(fmt.Sprintf("Failed to parse project/repository for given path with regex (%s)", scheme))
	}
	filepath := subMatches[indexFilePath]

	return filepath, nil
}

var _ GitRepository = BitbucketRepository{}

type BitbucketRepository struct {
	Instance    scmInstance
	Project     string
	Repository  string
	GivenScheme string
}

func (br BitbucketRepository) String() string {
	return fmt.Sprintf("Bitbucket instance: %s, project: %s, repository %s", br.Instance.Name(), br.Project, br.Repository)
}

func (br BitbucketRepository) Name() string {
	return br.Repository
}

func (br BitbucketRepository) GetGitHttpUrl() string {
	return fmt.Sprintf("%s/%s/%s.git", br.Instance.HttpUrl(), br.Project, br.Repository)
}
func (br BitbucketRepository) GetFile(repoPath string, ref string) (string, error) {
	isSubmodule, err := br.isFileInSubmodule(repoPath, ref)
	if err != nil {
		return "", cerr.AppendError("Failed to check if file is in submodule", err)
	}

	if isSubmodule {
		submodule, err := br.getSubmoduleData(repoPath, ref)
		if err != nil {
			return "", cerr.AppendErrorFmt("Failed to get repository of submodule '%s'", err, repoPath)
		}
		brSubmodule, err := parseBitbucketUrl(submodule.Url)
		if err != nil {
			return "", cerr.AppendError("Failed to parse submodule url", err)
		}
		subRepoPath := strings.TrimPrefix(repoPath, fmt.Sprintf("%s/", cg.GetFirstParentDir(repoPath)))
		fileStr, err := brSubmodule.GetFile(subRepoPath, submodule.Branch)
		if err != nil {
			return "", cerr.AppendErrorFmt("Failed to get file '%s' in bitbucket repository '%s'", err, repoPath, br)
		}
		return fileStr, nil
	}
	fileStr, err := br.Instance.GetClient().fetchFile(br.Project, br.Repository, repoPath, ref)
	if err != nil {
		return "", cerr.AppendErrorFmt("Failed to get file '%s' in bitbucket repository '%s'", err, repoPath, br)
	}

	return fileStr, nil
}

func (br BitbucketRepository) ListFiles(repoPath string, ref string) ([]string, error) {
	isSubmodule, err := br.isFileInSubmodule(repoPath, ref)
	if err != nil {
		return nil, cerr.AppendError("Failed to check if file is in submodule", err)
	}
	if isSubmodule {
		submodule, err := br.getSubmoduleData(repoPath, ref)
		if err != nil {
			return nil, cerr.AppendErrorFmt("Failed to get repository of submodule '%s'", err, repoPath)
		}
		brSubmodule, err := parseBitbucketUrl(submodule.Url)
		if err != nil {
			return nil, cerr.AppendError("Failed to parse submodule url", err)
		}
		subRepoPath := strings.TrimPrefix(repoPath, path.Join("/", cg.GetFirstParentDir(repoPath)))
		files, err := brSubmodule.ListFiles(subRepoPath, submodule.Branch)
		if err != nil {
			return nil, cerr.AppendErrorFmt("Failed to list files under folder '%s' in bitbucket repository '%s' at '%s'", err, repoPath, br, ref)
		}
		return files, nil
	}
	files, err := br.Instance.GetClient().listFiles(br.Project, br.Repository, repoPath, ref)
	if err != nil {
		return nil, cerr.AppendErrorFmt("Failed to list files under folder '%s' in bitbucket repository '%s' at reference '%s'", err, repoPath, br, ref)
	}

	return files, nil
}

func (br BitbucketRepository) ShallowClone(ref, path string) error {
	err := br.Instance.GetClient().shallowClone(br.GetGitHttpUrl(), ref, path)
	if err != nil {
		return cerr.AppendErrorFmt("Failed to shallow clone bitbucket repository '%s'", err, br.GetGitHttpUrl())
	}

	return nil
}

func (br BitbucketRepository) HasChangedSince(ref, since, path string) (bool, error) {
	commits, err := br.Instance.GetClient().getCommits(br.Project, br.Repository, ref, since, path)
	if err != nil {
		return false, cerr.AppendErrorFmt("Failed to get last commit bitbucket repository '%s' at ref %s, path %s", err, br.GetGitHttpUrl(), ref, path)
	}

	return len(commits) != 0, nil
}

func parseBitbucketUrl(repoUrl string) (BitbucketRepository, error) {
	netUrl, errParse := url.Parse(repoUrl)
	if errParse != nil {
		return BitbucketRepository{}, cerr.AppendError("Failed to parse repository url", errParse)
	}

	repo := BitbucketRepository{}

	instance, errInst := bitbucketInstanceFromHostname(netUrl.Hostname())

	if errInst != nil {
		return BitbucketRepository{}, cerr.AppendErrorFmt("Failed to determine bitbucket instance from given url (%s)", errInst, repoUrl)
	}

	repo.Instance = instance

	if !slices.Contains(knownSchemes, netUrl.Scheme) {
		return BitbucketRepository{}, cerr.AppendErrorFmt("Unknown transport protocol (%s) for remote git repository (%s)", errInst, netUrl.Scheme, repoUrl)
	}

	var errParsePath error
	repo.Project, repo.Repository, errParsePath = parseRepoPath(netUrl.Path, netUrl.Scheme)
	if errParsePath != nil {
		return BitbucketRepository{}, cerr.AppendErrorFmt("Failed to parse bitbucket repository path given (%s)", errParsePath, netUrl.Path)
	}

	repo.GivenScheme = netUrl.Scheme

	return repo, nil
}

func (br BitbucketRepository) isFileInSubmodule(repoPath, ref string) (bool, error) {
	baseDir := cg.GetFirstParentDir(repoPath)
	if slices.Contains([]string{"", ".", "/"}, baseDir) {
		return false, nil
	}

	fileType, err := br.Instance.GetClient().getFileType(br.Project, br.Repository, baseDir, ref)
	if err != nil {
		clog.Debug("Failed to get file type", err)
		return false, cerr.AppendErrorFmt("Failed to get file type of '%s' in bitbucket repository '%s'", err, repoPath, br)
	}
	return fileType == "SUBMODULE", nil
}

func (br BitbucketRepository) getSubmoduleData(repoPath, ref string) (bitbucketSubmodule, error) {
	baseDir := cg.GetFirstParentDir(repoPath)
	gitModules, err := br.Instance.GetClient().fetchFile(br.Project, br.Repository, kGitmoduleFilename, ref)
	if err != nil {
		return bitbucketSubmodule{}, cerr.AppendErrorFmt("Failed to get .gitmodules file from bitbucket repository '%s'", err, br)
	}
	submodules, err := parseGitModules(gitModules)
	if err != nil {
		return bitbucketSubmodule{}, cerr.AppendError("Failed to parse .gitmodules file", err)
	}
	submodule, ok := submodules[baseDir]
	if !ok {
		return bitbucketSubmodule{}, cerr.NewError(fmt.Sprintf("Failed to find submodule '%s' in .gitmodules file", baseDir))
	}
	return submodule, nil
}

func parseGitModules(gitModules string) (map[string]bitbucketSubmodule, error) {
	submodules := make(map[string]bitbucketSubmodule)
	parsedModules, err := ini.Load([]byte(gitModules))
	if err != nil {
		return nil, cerr.AppendError("Failed to parse .gitmodules file", err)
	}
	for _, section := range parsedModules.Sections() {
		if len(section.Name()) == 0 {
			continue
		}
		submodule := bitbucketSubmodule{}
		submodule.Path = section.Key("path").String()
		submodule.Url = section.Key("url").String()
		submodule.Branch = section.Key("branch").String()
		submodules[submodule.Path] = submodule
	}
	return submodules, nil
}
