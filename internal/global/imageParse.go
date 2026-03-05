package cg

import (
	"strings"
)

var (
	kLegacyDefaultDomain = "index.docker.io"
	kDefaultDomain       = "docker.io"
	kOfficialRepoName    = "library"
)

const (
	KLatestTag = "latest"
)

type imageData struct {
	repo      string
	imageName string
	imageTag  string
}

func splitDockerDomain(name string) (domain, remainder string) {
	i := strings.IndexRune(name, '/')
	if i == -1 || (!strings.ContainsAny(name[:i], ".:") && name[:i] != "localhost") {
		domain, remainder = kDefaultDomain, name
	} else {
		domain, remainder = name[:i], name[i+1:]
	}
	if domain == kLegacyDefaultDomain {
		domain = kDefaultDomain
	}
	if domain == kDefaultDomain && !strings.ContainsRune(remainder, '/') {
		remainder = kOfficialRepoName + "/" + remainder
	}
	return
}

func ParseImageString(imageString string) *imageData {
	domain, remainder := splitDockerDomain(imageString)
	data := &imageData{repo: domain}
	i := strings.IndexRune(remainder, ':')
	if i == -1 {
		data.imageTag = KLatestTag
		data.imageName = remainder
	} else {
		data.imageTag = remainder[i+1:]
		data.imageName = remainder[:i]
	}
	return data
}

func (data *imageData) ToString() string {
	return data.repo + "/" + data.imageName + ":" + data.imageTag
}

func (data *imageData) OverrideTag(tag string) {
	data.imageTag = tag
}
