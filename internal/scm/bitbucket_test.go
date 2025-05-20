package scm

// import (
// 	"encoding/json"
// 	"net/http"

// 	cg "github.com/amadeusitgroup/cds/internal/global"
// 	"github.com/jarcoal/httpmock"
// 	"github.com/onsi/ginkgo/v2"
// 	"github.com/onsi/gomega"
// )

// const (
// 	testrepoResponse = `{
// 		"slug": "testrepo",
// 		"id": 1,
// 		"name": "tesrepo",
// 		"description": "test repo",
// 		"scmId": "git",
// 		"state": "AVAILABLE",
// 		"statusMessage": "Available",
// 		"forkable": true,
// 		"project": {
// 		  "key": "testproject",
// 		  "id": 1000,
// 		  "name": "testproject",	
// 		  "public": false,
// 		  "type": "NORMAL",
// 		  "links": {
// 			"self": [
// 			  {
// 				"href": "https://enterprise.com/git/projects/testproject"
// 			  }
// 			]
// 		  }
// 		},
// 		"public": false,
// 		"links": {
// 		  "clone": [
// 			{
// 			  "href": "ssh://git@git.rnd.fix.me/testproject/testrepo.git",
// 			  "name": "ssh"
// 			},
// 			{
// 			  "href": "https://enterprise.com/git/scm/testproject/testrepo.git",
// 			  "name": "http"
// 			}
// 		  ],
// 		  "self": [
// 			{
// 			  "href": "https://enterprise.com/git/projects/testproject/repos/testrepo/browse"
// 			}
// 		  ]
// 		}
// 	  }`

// 	userInfosForValidate = `{
// 		"name": "testuser",
// 		"emailAddress": "testuser.testuser@company.com",
// 		"id": 100000,
// 		"displayName": "testuser testuser",
// 		"active": true,
// 		"slug": "testuser",
// 		"type": "NORMAL",
// 		"links": {
// 			"self": [
// 				{
// 					"href": "https://enterprise.com/git/users/testuser"
// 				}
// 			]
// 		}
// 	}`
// 	errorResponse = `{
// 		"errors": [
// 		  {
// 			"context": null,
// 			"message": "Repository testrepo/testerror does not exist.",
// 			"exceptionName": "com.atlassian.bitbucket.repository.NoSuchRepositoryException"
// 		  }
// 		]
// 	  }`

// 	errorResponseInexistentBranch = `{
// 		"errors": [
// 			{
// 			"context": null,
// 			"message":"The path \"testrepo/somefile\" does not exist at revision \"refs/heads/brancherr\"",
// 			"exceptionName":"com.atlassian.bitbucket.content.NoSuchPathException"
// 			}
// 		]
// 	}`

// 	errorResponseInexistentFile = `{
// 		"errors": [
// 			{
// 			"context": null,
// 			"message":"The path \"testrepo/somefileerr\" does not exist at revision \"refs/heads/testbranch\"",
// 			"exceptionName":"com.atlassian.bitbucket.content.NoSuchPathException"
// 			}
// 		]
// 	}`

// 	expectedFile = "some data in file"
// 	listFileResp = `{
// 		"values": [
// 		  ".gitignore",
// 		  "Jenkinsfile",
// 		  "README.md",
// 		  "commands/cds.go",
// 		  "commands/commands.go",
// 		  "commands/container.go",
// 		  "commands/containerConfig.go",
// 		  "commands/containerConfigCopy.go",
// 		  "commands/containerConfigInit.go",
// 		  "commands/containerProject.go",
// 		  "commands/containerRun.go",
// 		  "commands/space.go",
// 		  "commands/spaceConfig.go",
// 		  "commands/spaceConfigInit.go",
// 		  "commands/version.go",
// 		  "common/bitbucket.go",
// 		  "common/const.go",
// 		  "common/env.go",
// 		  "common/features.go",
// 		  "common/http.go",
// 		  "common/logging.go",
// 		  "common/remote.go",
// 		  "common/shell.go",
// 		  "common/shell_test.go",
// 		  "common/utils.go"
// 		],
// 		"size": 25,
// 		"isLastPage": false,
// 		"start": 0,
// 		"limit": 25,
// 		"nextPageStart": 25
// 	  }`
// )

// var (
// 	expectedListFiles = []string{
// 		".gitignore",
// 		"Jenkinsfile",
// 		"README.md",
// 		"commands/cds.go",
// 		"commands/commands.go",
// 		"commands/container.go",
// 		"commands/containerConfig.go",
// 		"commands/containerConfigCopy.go",
// 		"commands/containerConfigInit.go",
// 		"commands/containerProject.go",
// 		"commands/containerRun.go",
// 		"commands/space.go",
// 		"commands/spaceConfig.go",
// 		"commands/spaceConfigInit.go",
// 		"commands/version.go",
// 		"common/bitbucket.go",
// 		"common/const.go",
// 		"common/env.go",
// 		"common/features.go",
// 		"common/http.go",
// 		"common/logging.go",
// 		"common/remote.go",
// 		"common/shell.go",
// 		"common/shell_test.go",
// 		"common/utils.go",
// 	}
// )

// type authMock struct {
// 	usr string
// 	tkn string
// 	pwd string
// 	bt  bitbucketToken
// }

// func (am authMock) User(secreteName string) string {
// 	return am.usr
// }

// func (am authMock) Password(secreteName string) []byte {
// 	return []byte(am.pwd)
// }

// func (am authMock) Retry(secreteName string) []byte {
// 	return []byte{}
// }

// func (am authMock) Save(secreteName string, secret []byte) error {
// 	return nil
// }

// func (am authMock) SaveInfo(secreteName string, secret []byte) error {
// 	return nil
// }

// func (am authMock) Token(secreteName string) []byte {
// 	return []byte(am.tkn)
// }

// func (am authMock) TokenInfo(secreteName string) []byte {
// 	raw, err := json.Marshal(am.bt)
// 	if err != nil {
// 		return []byte{}
// 	}
// 	return raw
// }

// func isAuthValid(req *http.Request) bool {
// 	isTokenValid := false
// 	isPasswordValid := false

// 	token := req.Header.Get("Authorization")
// 	isTokenValid = token == "Bearer SOME_VALID_TOKEN"
// 	user, pass, ok := req.BasicAuth()
// 	isPasswordValid = ok && pass == "testpassword" && user == "testuser"

// 	return isPasswordValid || isTokenValid
// }

// var _ = ginkgo.Describe("net/bitbucket", ginkgo.Ordered, func() {
// 	shouldCreateToken = false
// 	ginkgo.Describe("Using bitbucket token", ginkgo.Ordered, func() {
// 		ginkgo.BeforeEach(func() {
// 			httpmock.RegisterResponder("GET", "https://test.net.bitbucket.localhost:443/git/rest/api/1.0/users/testuser",
// 				func(r *http.Request) (*http.Response, error) {
// 					if !isAuthValid(r) {
// 						return httpmock.NewStringResponse(http.StatusUnauthorized, "denied"), nil
// 					}

// 					return httpmock.NewStringResponse(200, userInfosForValidate), nil
// 				},
// 			)
// 		})

// 		ginkgo.It("should be able to instantiate a client with a valid token", func() {
// 			am := authMock{usr: "testuser", tkn: "SOME_VALID_TOKEN", bt: bitbucketToken{}}
// 			SetAuthenticationHandler(am)
// 			SetTokenHandler(am)
// 			_, isValid, err := newClientUsingToken(bitbucketInstance{
// 				name:     "test",
// 				httpHost: "test.net.bitbucket.localhost",
// 				httpPort: 443,
// 			})
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(isValid).To(gomega.BeTrue())
// 		})

// 		ginkgo.It("should validate that a token is invalid", func() {
// 			am := authMock{usr: "testuser", tkn: "SOME_INVALID_TOKEN", bt: bitbucketToken{}}
// 			SetAuthenticationHandler(am)
// 			SetTokenHandler(am)
// 			_, isValid, err := newClientUsingToken(bitbucketInstance{
// 				name:     "test",
// 				httpHost: "test.net.bitbucket.localhost",
// 				httpPort: 443,
// 			})
// 			gomega.Expect(err).To(gomega.HaveOccurred())
// 			gomega.Expect(isValid).To(gomega.BeFalse())
// 		})

// 		ginkgo.It("should be able to instantiate a new client using valid token", func() {
// 			am := authMock{usr: "testuser", tkn: "SOME_VALID_TOKEN", bt: bitbucketToken{}}
// 			SetAuthenticationHandler(am)
// 			SetTokenHandler(am)
// 			bc, err := newBitbucketClient(bitbucketInstance{
// 				name:     "test",
// 				httpHost: "test.net.bitbucket.localhost",
// 				httpPort: 443,
// 			})
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			authValid, err := bc.ValidateAuthentication()
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(authValid).To(gomega.BeTrue())
// 		})
// 	})

// 	ginkgo.Describe("Using bitbucket password", ginkgo.Ordered, func() {
// 		ginkgo.BeforeAll(func() {
// 			am := authMock{usr: "testuser", tkn: cg.EmptyStr, bt: bitbucketToken{}}
// 			SetAuthenticationHandler(am)
// 			SetTokenHandler(am)

// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/users/testuser",
// 				func(r *http.Request) (*http.Response, error) {
// 					if !isAuthValid(r) {
// 						return httpmock.NewStringResponse(403, "denied"), nil
// 					}

// 					return httpmock.NewStringResponse(200, userInfosForValidate), nil
// 				},
// 			)
// 		})
// 	})

// 	ginkgo.Describe("Using bitbucket client", func() {
// 		ginkgo.BeforeAll(func() {
// 			am := authMock{usr: "testuser", pwd: "testpassword", tkn: cg.EmptyStr, bt: bitbucketToken{}}
// 			SetAuthenticationHandler(am)
// 			SetTokenHandler(am)
// 			// we don't check auth here, but we need this endpoint to respond since client will try to validate auth
// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/users/testuser",
// 				httpmock.NewStringResponder(200, userInfosForValidate),
// 			)
// 		})

// 		ginkgo.It("should confirm that a repository exists", func() {
// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo",
// 				httpmock.NewStringResponder(200, testrepoResponse))

// 			exists, err := BitbucketInstanceFromName(KMainBitbucketInstanceName).GetClient().repositoryExists("testproject", "testrepo")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(exists).To(gomega.BeTrue())
// 		})

// 		ginkgo.It("should confirm that a repository doesn't exists", func() {
// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepoerr",
// 				httpmock.NewStringResponder(404, errorResponse))

// 			exists, err := BitbucketInstanceFromName(KMainBitbucketInstanceName).GetClient().repositoryExists("testproject", "testrepoerr")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(exists).To(gomega.BeFalse())
// 		})

// 		ginkgo.It("should be able to fetch a file from a repository", func() {
// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/raw/somefile",
// 				httpmock.NewStringResponder(200, expectedFile))

// 			file, err := BitbucketInstanceFromName(KMainBitbucketInstanceName).GetClient().fetchFile("testproject", "testrepo", "somefile", "")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(file).To(gomega.Equal(expectedFile))
// 		})

// 		ginkgo.It("should be able to fetch a file from a repository on a branch", func() {
// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/raw/somefile?at=refs%2Fheads%2Ftestbranch",
// 				httpmock.NewStringResponder(200, expectedFile))

// 			file, err := BitbucketInstanceFromName(KMainBitbucketInstanceName).GetClient().fetchFile("testproject", "testrepo", "somefile", "testbranch")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(file).To(gomega.Equal(expectedFile))
// 		})

// 		ginkgo.It("should not be able to fetch a file from a branch that does not exist", func() {
// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/raw/somefile?at=refs%2Fheads%2Ftestbrancherr",
// 				httpmock.NewStringResponder(404, errorResponseInexistentBranch))

// 			file, err := BitbucketInstanceFromName(KMainBitbucketInstanceName).GetClient().fetchFile("testproject", "testrepo", "somefile", "testbrancherr")
// 			gomega.Expect(err).To(gomega.HaveOccurred())
// 			gomega.Expect(file).To(gomega.BeEmpty())
// 		})

// 		ginkgo.It("should confirm that the file from a repository does not exist", func() {
// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/raw/somefileerr",
// 				httpmock.NewStringResponder(404, errorResponseInexistentFile))

// 			file, err := BitbucketInstanceFromName(KMainBitbucketInstanceName).GetClient().fetchFile("testproject", "testrepo", "somefileerr", "testbranch")
// 			gomega.Expect(err).To(gomega.HaveOccurred())
// 			gomega.Expect(file).To(gomega.BeEmpty())
// 		})

// 		ginkgo.It("should be able to list files in a repository", func() {
// 			httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/files?limit=10000",
// 				httpmock.NewStringResponder(200, listFileResp))

// 			fileList, err := BitbucketInstanceFromName(KMainBitbucketInstanceName).GetClient().listFilesRepo("testproject", "testrepo")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(fileList).To(gomega.Equal(expectedListFiles))
// 		})
// 	})
// })

// var _ = ginkgo.Describe("net/bitbucket_instance", ginkgo.Ordered, func() {
// 	ginkgo.Describe("Using BitbucketInstanceFrom", ginkgo.Ordered, func() {
// 		ginkgo.It("should return a valid instance from name", func() {
// 			instance := BitbucketInstanceFromName("local_unittesting")
// 			gomega.Expect(instance.(bitbucketInstance).err).To(gomega.Not(gomega.HaveOccurred()))
// 		})
// 	})
// })

// var _ = ginkgo.Describe("net/bitbucket_utils", func() {
// 	ginkgo.Describe("Parse bitbucket repository urls", func() {
// 		ginkgo.It("should be able to parse url path", func() {
// 			var path, proj, repo string
// 			var err error

// 			path = "/git/projects/DEVENV/repos/cds/browse"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds"))

// 			path = "/git/projects/DEVENV/repos/cds.testing/browse"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds.testing"))

// 			path = "/git/projects/DEVENV/repos/cds"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds"))
// 			filePath, _ := parseFilePathFromUrl(path, "https")
// 			gomega.Expect(filePath).To(gomega.Equal(""))

// 			path = "/git/projects/DEVENV/repos/cds.testing"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds.testing"))
// 			filePath, _ = parseFilePathFromUrl(path, "https")
// 			gomega.Expect(filePath).To(gomega.Equal(""))

// 			path = "/git/projects/DEVENV/repos/cds.testing-2"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds.testing-2"))
// 			filePath, _ = parseFilePathFromUrl(path, "https")
// 			gomega.Expect(filePath).To(gomega.Equal(""))

// 			path = "/git/projects/DEVENV/repos/cds/browse/test/subdir"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds"))
// 			filePath, err = parseFilePathFromUrl(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(filePath).To(gomega.Equal("test/subdir"))

// 			path = "/git/scm/DEVENV/cds.git"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds"))

// 			path = "/git/scm/DEVENV/cds.testing.git"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds.testing"))

// 			path = "/git/scm/DEVENV/cds.testing-2.git"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds.testing-2"))

// 			path = "/git/scm/DEVENV/cds"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds"))

// 			path = "/git/scm/DEVENV/cds.testing"
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds.testing"))

// 			path = "/DEVENV/cds.git"
// 			proj, repo, err = parseRepoPath(path, "ssh")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds"))

// 			path = "/DEVENV/cds.testing-1.git"
// 			proj, repo, err = parseRepoPath(path, "ssh")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds.testing-1"))

// 			path = "/DEVENV/cds"
// 			proj, repo, err = parseRepoPath(path, "ssh")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("DEVENV"))
// 			gomega.Expect(repo).To(gomega.Equal("cds"))

// 			path = `/git/scm/~jdoe/dummyobecmk.git`
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("~jdoe"))
// 			gomega.Expect(repo).To(gomega.Equal("dummyobecmk"))

// 			path = `/git/scm/~jdoe/standalone-devcontainer.git`
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("~jdoe"))
// 			gomega.Expect(repo).To(gomega.Equal("standalone-devcontainer"))

// 			path = `/git/scm/~j.doe/standalone-devcontainer.testing.git`
// 			proj, repo, err = parseRepoPath(path, "https")
// 			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 			gomega.Expect(proj).To(gomega.Equal("~j.doe"))
// 			gomega.Expect(repo).To(gomega.Equal("standalone-devcontainer.testing"))
// 		})

// 		ginkgo.It("should be able to determine a bitbucket repository", func() {
// 			var url string
// 			var isUrlresult, IsBBUrlResult bool

// 			url = "not/an/url"
// 			isUrlresult = IsUrl(url)
// 			IsBBUrlResult = IsBitbucketUrl(url, "https")
// 			gomega.Expect(isUrlresult).To(gomega.Equal(false))
// 			gomega.Expect(IsBBUrlResult).To(gomega.Equal(false))

// 			url = "https://enterprise.com/git/not/a/real/BB/repo"
// 			isUrlresult = IsUrl(url)
// 			IsBBUrlResult = IsBitbucketUrl(url, "https")
// 			gomega.Expect(isUrlresult).To(gomega.Equal(true))
// 			gomega.Expect(IsBBUrlResult).To(gomega.Equal(false))

// 			url = "https://enterprise.com/git/projects/DEVENV/repos/cds/browse"
// 			isUrlresult = IsUrl(url)
// 			IsBBUrlResult = IsBitbucketUrl(url, "https")
// 			gomega.Expect(isUrlresult).To(gomega.Equal(true))
// 			gomega.Expect(IsBBUrlResult).To(gomega.Equal(true))

// 			url = "https://enterprise.com/git/users/dummyuser/repos/cds/browse"
// 			isUrlresult = IsUrl(url)
// 			IsBBUrlResult = IsBitbucketUrl(url, "https")
// 			gomega.Expect(isUrlresult).To(gomega.Equal(true))
// 			gomega.Expect(IsBBUrlResult).To(gomega.Equal(true))

// 			url = "ssh://git@git.rnd.fix.me/devenv/cds.git"
// 			isUrlresult = IsUrl(url)
// 			IsBBUrlResult = IsBitbucketUrl(url, "https")
// 			gomega.Expect(isUrlresult).To(gomega.Equal(true))
// 			gomega.Expect(IsBBUrlResult).To(gomega.Equal(false))

// 			IsBBUrlResult = IsBitbucketUrl(url, "ssh")
// 			gomega.Expect(IsBBUrlResult).To(gomega.Equal(true))

// 		})

// 		// ginkgo.It("should be able to determine bitbucket instance based on host", func() {
// 		// 	var bi scmInstance
// 		// 	var err error

// 		// 	bi, err = bitbucketInstanceFromHostname("git-ssp.cicd.rnd.fix.me")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(bi.Name()).To(gomega.Equal("ssp"))

// 		// 	bi, err = bitbucketInstanceFromHostname("enterprise.com")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(bi.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	bi, err = bitbucketInstanceFromHostname("git.rnd.fix.me")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(bi.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	bi, err = bitbucketInstanceFromHostname("GIT.RND.fix.me")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(bi.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	bi, err = bitbucketInstanceFromHostname("github.com")
// 		// 	gomega.Expect(err).To(gomega.HaveOccurred())
// 		// 	gomega.Expect(bi.Name()).To(gomega.Equal(""))
// 		// })

// 		// https://git-ssp.cicd.rnd.fix.me/git/dashboard
// 		// ssh://git@git-ssp.cicd.rnd.fix.me:7999/...
// 		// "https://enterprise.com/git/projects/DEVENV/repos/cds/browse"
// 		// "https://enterprise.com/git/projects/PROJ/repos/REPO/browse"
// 		// https://enterprise.com/git/scm/devenv/cds.git
// 		// ssh://git@git.rnd.fix.me/devenv/dummyobecmk.git
// 		// ginkgo.It("should be able to parse full url to bitbucket repository", func() {
// 		// 	var br BitbucketRepository
// 		// 	var err error

// 		// 	br, err = parseBitbucketUrl("https://enterprise.com/git/scm/devenv/cds.git")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	br, err = parseBitbucketUrl("https://enterprise.com/git/scm/devenv/cds.testing-1.git")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds.testing-1"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	br, err = parseBitbucketUrl("https://enterprise.com/git/projects/devenv/repos/cds/browse")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	br, err = parseBitbucketUrl("https://enterprise.com/git/projects/devenv/repos/cds.testing-1/browse")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds.testing-1"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	br, err = parseBitbucketUrl("ssh://git@git.rnd.fix.me/devenv/cds.git")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("ssh"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	br, err = parseBitbucketUrl("ssh://git@git.rnd.fix.me/devenv/cds.testing-1.git")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds.testing-1"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("ssh"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	br, err = parseBitbucketUrl("https://git-ssp.cicd.rnd.fix.me/git/scm/devenv/cds.git")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal("ssp"))

// 		// 	br, err = parseBitbucketUrl("https://git-ssp.cicd.rnd.fix.me/git/projects/devenv/repos/cds/browse")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal("ssp"))

// 		// 	br, err = parseBitbucketUrl("ssh://git@git-ssp.cicd.rnd.fix.me:7999/devenv/cds.git")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("devenv"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("ssh"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal("ssp"))

// 		// 	br, err = parseBitbucketUrl("https://enterprise.com/git/users/dummyuser/repos/cds/browse")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("~dummyuser"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal(KMainBitbucketInstanceName))

// 		// 	br, err = parseBitbucketUrl("https://git-ssp.cicd.rnd.fix.me/git/users/dummyuser/repos/cds/browse")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("~dummyuser"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal("ssp"))

// 		// 	br, err = parseBitbucketUrl("https://git-ssp.cicd.rnd.fix.me/git/users/dummy.user/repos/cds.testing-1/browse")
// 		// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		// 	gomega.Expect(br.Project).To(gomega.Equal("~dummy.user"))
// 		// 	gomega.Expect(br.Repository).To(gomega.Equal("cds.testing-1"))
// 		// 	gomega.Expect(br.GivenScheme).To(gomega.Equal("https"))
// 		// 	gomega.Expect(br.Instance.Name()).To(gomega.Equal("ssp"))

// 		// })

// 	})
// })

// var _ = ginkgo.Describe("net/bitbucket_repository", ginkgo.Ordered, func() {
// 	ginkgo.BeforeAll(func() {
// 		am := authMock{usr: "testuser", pwd: "testpassword", tkn: cg.EmptyStr, bt: bitbucketToken{}}
// 		SetAuthenticationHandler(am)
// 		SetTokenHandler(am)
// 		// we don't check auth here, but we need this endpoint to respond since client will try to validate auth
// 		httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/users/testuser",
// 			httpmock.NewStringResponder(200, userInfosForValidate),
// 		)
// 	})

// 	// test function ListFiles in case of submodule
// 	ginkgo.It("should be able to list files in a repository with submodule", func() {
// 		httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/browse/folderInModule?type=true",
// 			httpmock.NewStringResponder(200, "{\"type\": \"SUBMODULE\"}"))

// 		httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/raw/.gitmodules", httpmock.NewStringResponder(200, "[submodule \"folderInModule\"]\n\tpath = folderInModule\n\turl = https://enterprise.com/git/scm/testproject/testSubmodule.git"))
// 		httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testSubmodule/files?limit=10000",
// 			httpmock.NewStringResponder(200, listFileResp))

// 		repo := BitbucketRepository{
// 			Project:     "testproject",
// 			Repository:  "testrepo",
// 			Instance:    BitbucketInstanceFromName(KMainBitbucketInstanceName),
// 			GivenScheme: "https",
// 		}

// 		fileList, err := repo.ListFiles("/folderInModule", "")
// 		gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 		gomega.Expect(fileList).To(gomega.ConsistOf(expectedListFiles))
// 	})

// 	// // test function GetFile in case of submodule
// 	// ginkgo.It("should be able to fetch a file from a repository with submodule", func() {
// 	// 	httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/browse/folderInModule?type=true",
// 	// 		httpmock.NewStringResponder(200, "{\"type\": \"SUBMODULE\"}"))

// 	// 	httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testrepo/raw/.gitmodules", httpmock.NewStringResponder(200, "[submodule \"folderInModule\"]\n\tpath = folderInModule\n\turl = https://enterprise.com/git/scm/testproject/testSubmodule.git"))
// 	// 	httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testSubmodule/browse/somefile?type=true",
// 	// 		httpmock.NewStringResponder(200, "{\"type\": \"FILE\"}"))
// 	// 	httpmock.RegisterResponder("GET", "https://enterprise.com:443/git/rest/api/1.0/projects/testproject/repos/testSubmodule/raw/somefile",
// 	// 		httpmock.NewStringResponder(200, expectedFile))

// 	// 	repo := BitbucketRepository{
// 	// 		Project:     "testproject",
// 	// 		Repository:  "testrepo",
// 	// 		Instance:    BitbucketInstanceFromName(KMainBitbucketInstanceName),
// 	// 		GivenScheme: "https",
// 	// 	}

// 	// 	file, err := repo.GetFile("folderInModule/somefile", "")
// 	// 	gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
// 	// 	gomega.Expect(file).To(gomega.Equal(expectedFile))
// 	// })
// })
