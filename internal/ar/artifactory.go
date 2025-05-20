package ar

import (
	"fmt"
	"net/http"
	"time"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/config"
)

// const (
// 	artifactoryUrl = "https://repository.rnd.fix.me"
// )

// callbacks to obtain credentials from outside of this package
var (
	mock         bool
	mockedFetch  func(string, string) ([]byte, error)   = func(s1, s2 string) ([]byte, error) { return nil, nil }
	mockedList   func(string, string) ([]string, error) = func(s1, s2 string) ([]string, error) { return nil, nil }
	mockedExists func(string, string) (bool, error)     = func(s1, s2 string) (bool, error) { return false, nil }
)

type artifactoryClient struct {
	serviceManager artifactory.ArtifactoryServicesManager
	err            error
}

type IArtifactoryClient interface {
	FetchFile(string, string) ([]byte, error)
	ListDirectories(string, string) ([]string, error)
	FileExists(string, string) (bool, error)
}

var _ IArtifactoryClient = (*artifactoryClient)(nil)

type artifactoryToken struct {
	token        string
	RefreshToken string    `json:"refresh_token"`
	ExpiringDate time.Time `json:"expiringDate"`
}

func (at artifactoryToken) String() string {
	return fmt.Sprintf("at: token=%s, refresh=%s, expire=%v", at.token, at.RefreshToken, at.ExpiringDate)
}

// instantiate a AR client, factory for artifactoryClient
func NewArtifactoryClient(instanceName string) (IArtifactoryClient, error) {
	if mock {
		clog.Debug("Artifactory clients will be generated in mocked mode")
		return mockedARClient{}, nil
	}

	instance := ArtifactoryInstanceFromName(instanceName)
	rtDetails := auth.NewArtifactoryDetails()
	rtDetails.SetUrl(instance.url)
	user, _ := getArtifactoryUser(instanceName)
	rtDetails.SetUser(user)
	// setAuthMethod
	token, errToken := getValidArtifactoryToken(instanceName)
	if errToken != nil {
		password, errPass := getArtifactoryPassword(instanceName)
		if errPass != nil {
			return nil, cerr.AppendError("Failed getting password", errPass)
		}
		rtDetails.SetPassword(password)
	}
	if token.token != "" {
		rtDetails.SetAccessToken(token.token)
	}
	serviceConfig, errConfig := config.NewConfigBuilder().
		SetServiceDetails(rtDetails).
		SetHttpClient(http.DefaultClient).
		SetDryRun(false).
		Build()
	if errConfig != nil {
		return nil, cerr.AppendError("Couldn't generate service config to create new AR client", errConfig)
	}

	rtManager, err := artifactory.New(serviceConfig)
	return &artifactoryClient{serviceManager: rtManager}, err
}

func getValidArtifactoryToken(instanceName string) (artifactoryToken, error) {
	token, errToken := getArtifactoryToken(instanceName)
	if errToken != nil {
		return generateNewToken(instanceName)
	}

	if !token.isExpired() {
		return token, nil
	}

	token, errRefresh := refreshToken(instanceName, token)
	if errRefresh != nil {
		return generateNewToken(instanceName)
	}
	return token, nil
}

func generateNewToken(instanceName string) (artifactoryToken, error) {
	instance := ArtifactoryInstanceFromName(instanceName)
	rtDetails := auth.NewArtifactoryDetails()
	rtDetails.SetUrl(instance.url)
	user, errUser := getArtifactoryUser(instanceName)
	if errUser != nil {
		return artifactoryToken{}, errUser
	}
	rtDetails.SetUser(user)
	password, errPass := getArtifactoryPassword(instanceName)
	if errPass != nil {
		return artifactoryToken{}, errPass
	}
	rtDetails.SetPassword(password)

	serviceConfig, errConfig := config.NewConfigBuilder().
		SetServiceDetails(rtDetails).
		SetDryRun(false).
		Build()
	if errConfig != nil {
		return artifactoryToken{}, cerr.AppendError("Couldn't generate service config to generate new token", errConfig)
	}

	rtManager, errManager := artifactory.New(serviceConfig)
	if errManager != nil {
		return artifactoryToken{}, cerr.AppendError("Couldn't generate manager to generate new token", errManager)
	}
	params := services.NewCreateTokenParams()
	params.Scope = "api:* member-of-groups:readers"
	params.Username, _ = getArtifactoryUser(instanceName)
	params.Refreshable = true

	results, errCreate := rtManager.CreateToken(params)
	if errCreate != nil {
		return artifactoryToken{}, cerr.AppendError("Couldn't refresh token", errCreate)
	}
	token := artifactoryToken{token: results.AccessToken, RefreshToken: results.RefreshToken, ExpiringDate: time.Now().Add(time.Hour)}
	if errSet := setArtifactoryToken(instanceName, token); errSet != nil {
		clog.Warn("Couldn't set generated token", errSet)
	}
	return token, nil
}

func refreshToken(instanceName string, token artifactoryToken) (artifactoryToken, error) {
	params := services.NewArtifactoryRefreshTokenParams()
	params.AccessToken = token.token
	params.RefreshToken = token.RefreshToken
	params.Token.Scope = "api:*"
	params.Token.ExpiresIn = 3600
	instance := ArtifactoryInstanceFromName(instanceName)
	rtDetails := auth.NewArtifactoryDetails()
	rtDetails.SetUrl(instance.url)
	user, _ := getArtifactoryUser(instanceName)
	rtDetails.SetUser(user)
	serviceConfig, errConfig := config.NewConfigBuilder().
		SetServiceDetails(rtDetails).
		SetDryRun(false).
		Build()

	if errConfig != nil {
		return artifactoryToken{}, cerr.AppendError("Couldn't generate service config to refresh token", errConfig)
	}

	rtManager, errManager := artifactory.New(serviceConfig)

	if errManager != nil {
		return artifactoryToken{}, cerr.AppendError("Couldn't generate manager to refresh token", errManager)
	}

	results, errRefresh := rtManager.RefreshToken(params)
	if errRefresh != nil {
		return artifactoryToken{}, cerr.AppendError("Couldn't refresh token", errRefresh)
	}
	newToken := artifactoryToken{token: results.AccessToken, RefreshToken: results.RefreshToken, ExpiringDate: time.Now().Add(time.Hour)}
	if errSet := setArtifactoryToken(instanceName, newToken); errSet != nil {
		clog.Warn("Couldn't set refreshed token", errSet)
	}
	return newToken, nil
}

func (token *artifactoryToken) isExpired() bool {
	return time.Now().After(token.ExpiringDate)
}

func SetMock(fetchFile func(string, string) ([]byte, error), listDirectories func(string, string) ([]string, error), fileExists func(string, string) (bool, error)) {
	mock = true
	if mockedFetch != nil {
		mockedFetch = fetchFile
	}
	if listDirectories != nil {
		mockedList = listDirectories
	}
	if fileExists != nil {
		mockedExists = fileExists
	}
}

func ClearMock() {
	mock = false
	mockedExists = nil
	mockedFetch = nil
	mockedList = nil
}

type mockedARClient struct{}

var _ IArtifactoryClient = (*mockedARClient)(nil)

func (client mockedARClient) FetchFile(repo, path string) ([]byte, error) {
	if mockedFetch != nil {
		return mockedFetch(repo, path)
	}
	return nil, nil
}
func (client mockedARClient) ListDirectories(repo, path string) ([]string, error) {
	if mockedList != nil {
		return mockedList(repo, path)
	}
	return nil, nil
}
func (client mockedARClient) FileExists(repo, path string) (bool, error) {
	if mockedExists != nil {
		return mockedExists(repo, path)
	}
	return false, nil
}
