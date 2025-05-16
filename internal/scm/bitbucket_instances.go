package scm

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

const (
	kProject                   = "Project"
	kUsername                  = "Username"
	kRepository                = "Repository"
	kFilePath                  = "FilePath"
	KMainBitbucketInstanceName = "central"
	KMainBitbucketHttpHost     = "FixMe" // TODO: FixMe
	KSspBitbucketInstanceName  = "FixMe" // TODO: FixMe
	KSspBitbucketHttpHost      = "FixMe" // TODO: FixMe
)

var (
	// very good resource: https://www.debuggex.com/
	// https://github.com/google/re2/wiki/Syntax
	sshUrlRegex     = regexp.MustCompile(fmt.Sprintf(`/?(?P<%s>~?[\w.]+)/(?P<%s>[\w.-]+)`, kProject, kRepository))
	httpUrlGitRegex = regexp.MustCompile(fmt.Sprintf(`/?git/scm/(?P<%s>~?[\w.]+)/(?P<%s>[\w.-]+)`, kProject, kRepository))
	httpUrlUiRegex  = regexp.MustCompile(fmt.Sprintf(`/?git/projects/(?P<%s>~?[\w.]+)/repos\/(?P<%s>[\w.-]+)(/browse)?/?(?P<%s>(.+/)*.+\.?\w*)?`, kProject, kRepository, kFilePath))
	userUrlUiRegex  = regexp.MustCompile(fmt.Sprintf(`/?git/users/(?P<%s>~?[\w.]+)/repos\/(?P<%s>[\w.-]+)(/browse)?/?(?P<%s>(.+/)*.+\.?\w*)?`, kUsername, kRepository, kFilePath))

	knownSchemes = []string{"ssh", "https"}

	bitbucketClients = map[string]*bitbucketClient{}
	// TODO:Feature: refactor in conf files
	bitbucketInstances = []bitbucketInstance{
		{
			name:     KMainBitbucketInstanceName,
			httpHost: KMainBitbucketHttpHost,
			httpPort: 443,
			sshHost:  "git.rnd.fix.me",
			sshPort:  22,
		},
		{
			name:     KSspBitbucketInstanceName,
			httpHost: KSspBitbucketHttpHost,
			httpPort: 443,
			sshHost:  KSspBitbucketHttpHost,
			sshPort:  7999,
		},
		{
			name:     "local_unittesting",
			httpHost: "127.0.0.1",
			httpPort: 443,
			sshHost:  "127.0.0.1",
			sshPort:  22,
		},
	}
)

var _ scmInstance = bitbucketInstance{}

type bitbucketInstance struct {
	name     string
	httpHost string
	httpPort uint
	sshHost  string
	sshPort  uint
	err      error
}

func (bi bitbucketInstance) Name() string {
	return bi.name
}

func (bi bitbucketInstance) BaseHttpUrl() string {
	return fmt.Sprintf("https://%s:%d/git", bi.httpHost, bi.httpPort)
}

func (bi bitbucketInstance) HttpUrl() string {
	return fmt.Sprintf("%s/scm", bi.BaseHttpUrl())
}

func (bi bitbucketInstance) SshUrl() string {
	return fmt.Sprintf("ssh://git@%s:%d", bi.httpHost, bi.httpPort)
}

func (bi bitbucketInstance) hostBelongsToInstance(host string) bool {
	return strings.EqualFold(bi.httpHost, host) || strings.EqualFold(bi.sshHost, host)
}

func (bi bitbucketInstance) GetClient() *bitbucketClient {
	if bi.err != nil {
		return &bitbucketClient{err: bi.err}
	}

	var err error
	client, ok := bitbucketClients[bi.Name()]
	if !ok {
		client, err = newBitbucketClient(bi)
		if err != nil {
			clog.Error(fmt.Sprintf("Failed to instantiate bitbucket client for bitbucket instance (%s)", bi.httpHost), err)
		}

		bitbucketClients[bi.Name()] = client
	}

	return client
}

func (bi bitbucketInstance) Error() string {
	return bi.err.Error()
}

func bitbucketInstanceFromHostname(host string) (scmInstance, error) {
	for _, instance := range bitbucketInstances {
		if instance.hostBelongsToInstance(host) {
			return instance, nil
		}
	}

	return bitbucketInstance{}, cerr.NewError(fmt.Sprintf("Failed to identify bitbucket instance for host (%s)", host))
}

func BitbucketInstanceFromName(name string) scmInstance {
	for _, instance := range bitbucketInstances {
		if strings.EqualFold(name, instance.name) {
			return instance
		}
	}

	return bitbucketInstance{err: cerr.NewError(fmt.Sprintf("Failed to identify bitbucket instance (%s)", name))}
}

func SetClient(instanceName string, bc *bitbucketClient) error {
	if bc == nil {
		return cerr.NewError(fmt.Sprintf("Cannot set nil client to '%s' bitbucket instance", instanceName))
	}

	isInstanceKnown := false
	for _, i := range bitbucketInstances {
		isInstanceKnown = isInstanceKnown || i.name == instanceName
	}

	if !isInstanceKnown {
		return cerr.NewError(fmt.Sprintf("Invalid bitbucket instance name '%s', instance is unknown to cds", instanceName))
	}

	bitbucketClients[instanceName] = bc

	return nil
}
