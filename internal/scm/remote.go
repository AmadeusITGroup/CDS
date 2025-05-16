package scm

import (
	"fmt"
	"net/url"
	"slices"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

const (
	kSsh   string = "ssh"
	kHttps string = "https"
)

func IsUrl(path string) bool {
	parsedUrl, err := url.Parse(path)
	return err == nil && parsedUrl.Host != ""
}

// Given a url to a remote file returns a string containing the content of the file
// TODO:Feature: add artifactory
func GetFileByUrl(urlToFile string) (string, error) {
	repo, err := ParseGitRepositoryUrl(urlToFile)
	if err != nil {
		return "", cerr.AppendError("Couldn't parse git repository url", err)
	}
	parsedUrl, err := url.Parse(urlToFile)
	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Couldn't parse url %v", urlToFile), err)
	}
	if !slices.Contains(knownSchemes, parsedUrl.Scheme) {
		return "", cerr.AppendError(fmt.Sprintf("Unknown scheme %v", parsedUrl.Scheme), err)
	}
	filepath, err := parseFilePathFromUrl(urlToFile, parsedUrl.Scheme)
	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Couldn't find filepath from url %v", urlToFile), err)
	}
	fileContent, err := repo.GetFile(filepath, "")
	if err != nil {
		return "", cerr.AppendError("Couldn't get file from repo", err)
	}
	return fileContent, nil
}

func IsBitbucketUrl(repoUrl string, scheme string) bool {
	switch scheme {
	case kSsh:
		if sshUrlRegex.MatchString(repoUrl) {
			return true
		}
	case kHttps:
		switch {
		case httpUrlGitRegex.MatchString(repoUrl), httpUrlUiRegex.MatchString(repoUrl), userUrlUiRegex.MatchString(repoUrl):
			return true
		default:
			clog.Warn(fmt.Sprintf("Failed to parse project/repository for given scm url (%s)", repoUrl))
			return false
		}
	default:
		clog.Warn(fmt.Sprintf("Unhandled transport scheme for given scm url (%s)", scheme))
		return false
	}
	return false
}

// To implement...
func IsArtifactoryUrl(url string, scheme string) bool {
	clog.Warn(fmt.Sprintf("CDS does not support the parsing of AR url yet '%s', should be done later!", url))
	return false
}
