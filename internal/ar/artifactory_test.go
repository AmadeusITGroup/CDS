package ar

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

const (
	// versionResponse = `{
	// 	"version" : "7.19.13",
	// 	"revision" : "71913900",
	// 	"addons" : [ "ha", "build", "docker", "vagrant", "replication", "filestore", "plugins", "gems", "composer", "npm", "bower", "git-lfs", "nuget", "debian", "opkg", "rpm", "cocoapods", "conan", "vcs", "pypi", "release-bundle", "jf-event", "replicator", "keys", "alpine", "cargo", "chef", "cran", "federated", "git", "go", "helm", "rest", "conda", "tracker", "license", "puppet", "ldap", "sso", "layouts", "properties", "search", "securityresourceaddon", "filtered-resources", "p2", "watch", "webstart", "support", "xray" ],
	// 	"license" : "41b6b437bba94750a5e992cd0c84c3d1fe95dcce3"
	//   }`

	devenvGenericProdStorageResponse = `{
		"repo" : "fix-me",
		"path" : "/",
		"created" : "2020-11-26T10:49:43.523Z",
		"lastModified" : "2020-11-26T10:49:43.523Z",
		"lastUpdated" : "2020-11-26T10:49:43.523Z",
		"children" : [ {
		  "uri" : "/com",
		  "folder" : true
		}, {
		  "uri" : "/downloadOnLinux.sh",
		  "folder" : false
		}, {
		  "uri" : "/cds",
		  "folder" : true
		}, {
		  "uri" : "/public",
		  "folder" : true
		}, {
		  "uri" : "/downloadOnWindows.bat",
		  "folder" : false
		}, {
		  "uri" : "/tmp",
		  "folder" : true
		}, {
		  "uri" : "/-T",
		  "folder" : false
		}, {
		  "uri" : "/VSCode",
		  "folder" : true
		}, {
		  "uri" : "/ejtserver_linux_1_16_2.rpm",
		  "folder" : false
		}, {
		  "uri" : "/ejtserver_linux_1_13_2.rpm",
		  "folder" : false
		}, {
		  "uri" : "/xdlc",
		  "folder" : true
		}, {
		  "uri" : "/devbox",
		  "folder" : true
		} ],
		"uri" : "https://https://repository.rnd.fix.me:443/artifactory/api/storage/devenv-generic-prod-devenv-exp"
	  }`
	validToken        = "ACCESS_TOKEN"
	validRefreshToken = "REFRESH_TOKEN"
	fakeRepo          = "repo"
	fakePath          = "/path/to/file.txt"
)

// type authMock struct {
// 	usr string
// 	tkn string
// 	at  artifactoryToken
// }

// func (am *authMock) User(secreteName string) string {
// 	return am.usr
// }

// func (am *authMock) Password(secreteName string) []byte {
// 	return []byte("")
// }

// func (am *authMock) Retry(secreteName string) []byte {
// 	return []byte{}
// }

// func (am *authMock) Save(secreteName string, secret []byte) error {
// 	return nil
// }

// func (am *authMock) SaveInfo(secreteName string, secret []byte) error {
// 	return nil
// }

// func (am *authMock) Token(secreteName string) []byte {
// 	return []byte(am.tkn)
// }

// func (am *authMock) TokenInfo(secreteName string) []byte {
// 	raw, err := json.Marshal(am.at)
// 	if err != nil {
// 		return []byte{}
// 	}
// 	return raw
// }

// func authMiddlewareMock(r *http.Request) bool {

// 	authHeader, ok := r.Header["Authorization"]
// 	if !ok {
// 		return false
// 	}
// 	return cg.Any(authHeader, func(s string) bool { return strings.Contains(s, validToken) })
// }

var _ = ginkgo.Describe("net/artifactory", func() {
	// ginkgo.Describe("Using AR token", func() {
	// 	am := authMock{usr: "testuser",
	// 		tkn: cg.EmptyStr,
	// 		at:  artifactoryToken{RefreshToken: validRefreshToken, ExpiringDate: time.Now().Add(time.Hour)},
	// 	}
	// 	SetAuthenticationHandler(&am)
	// 	SetTokenHandler(&am)
	// 	ginkgo.BeforeEach(func() {
	// 		httpmock.RegisterResponder("GET", fmt.Sprintf("%v/%v%v", artifactoryUrl, fakeRepo, fakePath), func(r *http.Request) (*http.Response, error) {
	// 			if !authMiddlewareMock(r) {
	// 				return httpmock.NewStringResponse(401, fmt.Sprintf("Invalid token: %v", r.Header["Authorization"])), nil
	// 			}
	// 			return httpmock.NewStringResponse(200, "Some content for this file"), nil
	// 		})

	// 		httpmock.RegisterResponder("GET", fmt.Sprintf("%v/api/system/version", artifactoryUrl), func(r *http.Request) (*http.Response, error) {
	// 			return httpmock.NewStringResponse(200, versionResponse), nil
	// 		})

	// 		httpmock.RegisterResponder("GET", fmt.Sprintf("%v/api/storage/devenv-generic-prod-devenv-exp", artifactoryUrl), func(r *http.Request) (*http.Response, error) {
	// 			return httpmock.NewStringResponse(200, devenvGenericProdStorageResponse), nil
	// 		})
	// 	})

	// 	ginkgo.It("should be able to list files of the repo without token", func() {
	// 		am.tkn = ""
	// 		client, err := NewArtifactoryClient("rnd")
	// 		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	// 		fileList, err := client.ListDirectories("devenv-generic-prod-devenv-exp", "")
	// 		gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// 		gomega.Expect(fileList).To(gomega.ContainElement("xdlc"))
	// 	})

	// 	ginkgo.It("should be able to fetch file of the repo with token", func() {
	// 		am.tkn = validToken
	// 		client, err := NewArtifactoryClient("rnd")
	// 		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	// 		fileContent, err := client.FetchFile(fakeRepo, fakePath)
	// 		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	// 		gomega.Expect(string(fileContent)).To(gomega.ContainSubstring("content"))
	// 	})

	// 	ginkgo.It("shouldn't be able to fetch file of the repo with wrong token", func() {
	// 		am.tkn = "WRONG_TOKEN"
	// 		client, err := NewArtifactoryClient("rnd")
	// 		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	// 		fileContent, err := client.FetchFile(fakeRepo, fakePath)
	// 		gomega.Expect(err).To(gomega.HaveOccurred())
	// 		gomega.Expect(string(fileContent)).To(gomega.BeEmpty())
	// 	})
	// })
	ginkgo.Describe("Using mocking system", func() {
		ginkgo.BeforeEach(func() {
			fetchFile := func(s1, s2 string) ([]byte, error) { return []byte("byteArray"), nil }
			SetMock(fetchFile, nil, nil)
		})
		ginkgo.AfterEach(ClearMock)
		ginkgo.It("Fetchfile executes given mocked function", func() {
			client, err := NewArtifactoryClient("rnd")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			fileContent, err := client.FetchFile(fakeRepo, fakePath)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(string(fileContent)).To(gomega.ContainSubstring("Array"))
		})
		ginkgo.It("ListDirectories executes default mocked function", func() {
			client, err := NewArtifactoryClient("rnd")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			fileList, err := client.ListDirectories("fix-me", "")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(fileList).To(gomega.BeEmpty())
		})
	})
})
