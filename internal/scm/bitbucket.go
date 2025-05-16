// contains private BitbucketClient methods and helpers
package scm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

const (
	kExpiryDaysForToken = 365
)

type bitbucketClient struct {
	httpClient   *Client
	instancePath string
	instance     scmInstance
	token        bitbucketToken
	err          error
}

type bitbucketToken struct {
	Id    string `json:"id"`
	token string
}

func (bc *bitbucketClient) getUsername() string {
	return bc.httpClient.Auth.Username
}

func (bc *bitbucketClient) getPassword() string {
	return bc.httpClient.Auth.Password
}

func (bc *bitbucketClient) getToken() bitbucketToken {
	return bc.token
}

func (bc *bitbucketClient) setUsername(user string) {
	bc.httpClient.Auth.Username = user
}

func (bc *bitbucketClient) setPassword(password string) {
	bc.httpClient.Auth.Password = password
}

func (bc *bitbucketClient) setToken(token bitbucketToken) {
	bc.httpClient.Auth.AccessToken = token.token
	bc.token.Id = token.Id
	bc.token.token = token.token
}

func (bc *bitbucketClient) getBitbucketAuthMethod() *bitbucketGitAuthMethod {
	return &bitbucketGitAuthMethod{name: "https", bitbucketClient: bc}
}

func (bc *bitbucketClient) createToken() error {
	url, err := url.Parse(fmt.Sprintf(
		"%s/rest/access-tokens/1.0/users/%s",
		bc.instancePath, bc.getUsername()))

	if err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to build url to create token for user %s", bc.getUsername()), err)
	}

	// req to get token
	data := RequestAccessToken{Name: "CDS-devenv", Permissions: []string{"REPO_WRITE", "PROJECT_READ"}, ExpiryDays: kExpiryDaysForToken}
	dataBytes, err := json.Marshal(data)

	if err != nil {
		return cerr.AppendError("failed to encode token request to JSON", err)
	}

	resp, err := bc.httpClient.Put(*url, bytes.NewReader(dataBytes))

	if err != nil {
		return cerr.AppendError("Failed to get a new Bitbucket token", err)
	}

	if resp.code != http.StatusOK {
		clog.Debug("Failed to request a token at", url, ", HTTP code", resp.code, "\nwith data", string(dataBytes))
		return cerr.NewError(fmt.Sprintf("Failed to get a new Bitbucket token, http code: %v", resp.code))
	}

	tokenResponse := RequestAccessTokenResponse{}
	err = json.Unmarshal(resp.body, &tokenResponse)

	if err != nil {
		return cerr.AppendError("Failed to decode response to token creation", err)
	}

	bt := bitbucketToken{Id: tokenResponse.ID, token: tokenResponse.Token}

	bc.setToken(bt)

	return nil
}

// we cannot retrieve an existing token, we can only know one exists
func (bc *bitbucketClient) deleteTokenIfAny() error {
	tokens, err := bc.listTokens()

	if err != nil {
		return cerr.AppendError("Failed to get the list of token to check if CDS already has one", err)
	}

	if tokens.hasCdsToken() {
		clog.Warn("Bitbucket already has a token registered for CDS, it will be deleted and replaced by a new one !")
		cdsToken := tokens.getCdsToken()

		if err := bc.deleteToken(cdsToken); err != nil {
			return cerr.AppendError("Failed to delete CDS token", err)
		}
	}

	return nil
}

func (bc *bitbucketClient) listTokens() (TokensListing, error) {
	url, err := url.Parse(fmt.Sprintf(
		"%s/rest/access-tokens/1.0/users/%s?limit=1000",
		bc.instancePath, bc.getUsername()))

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to build url to create token for user %s", bc.getUsername()), err)
	}

	resp, err := bc.httpClient.Get(*url)

	if err != nil {
		return nil, cerr.AppendError("Failed to get a new Bitbucket token", err)
	}

	tokenList := TokenListResponse{}
	err = json.Unmarshal(resp.body, &tokenList)

	if err != nil {
		return nil, cerr.AppendError("Failed to decode response to token creation", err)
	}

	if tokenList.Size == tokenList.Limit {
		clog.Warn("Maximum limit of token count reached, some may be discarded !")
	}

	return tokenList.Values, nil
}

func (bc *bitbucketClient) deleteToken(t TokenListing) error {
	url, err := url.Parse(fmt.Sprintf(
		"%s/rest/access-tokens/1.0/users/%s/%s",
		bc.instancePath, bc.getUsername(), t.ID))

	if err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to build url to create token for user %s", bc.getUsername()), err)
	}

	resp, err := bc.httpClient.Delete(*url)

	if err != nil {
		return cerr.AppendError("Failed to get a new Bitbucket token", err)
	}

	if resp.code != http.StatusOK && resp.code != http.StatusNoContent {
		clog.Debug("Made DELETE request but HTTP code was", resp.code, ", server responded:", string(resp.body))
		return cerr.NewError("Failed to make HTTP DELETE request to delete token")
	}

	return nil
}

var (
	shouldCreateToken = true // TODO:Refactor: remove completely along the commented SetShouldCreateToken
)

// func SetShouldCreateToken(createToken bool) {
// 	shouldCreateToken = createToken
// }

// default factory to get a BitbucketClient, will look for tokens in secrets.json via getters sets in commands
// falls back on user/password via stdin if that fails
func newBitbucketClient(instance bitbucketInstance) (*bitbucketClient, error) {
	if instance.err != nil {
		return nil, cerr.AppendError("Failed to create new bitbucket client, an error occurred in the instance", instance.err)
	}

	var bc bitbucketClient
	var tokenClientValid bool
	var passwordClientValid bool
	var errNewClient error

	bc, tokenClientValid, errNewClient = newClientUsingToken(instance)
	if errNewClient != nil {
		clog.Warn(fmt.Sprintf("Failed to create new bitbucket client using token for url '%s'", instance.BaseHttpUrl()))
	}
	if !tokenClientValid {
		// try using password from callback before asking for interactive password
		bc, passwordClientValid, errNewClient = newClientUsingCallbackPassword(instance)
		if errNewClient != nil {
			return nil, cerr.AppendErrorFmt("Failed to create new bitbucket client using password for url '%s'", errNewClient, instance.BaseHttpUrl())
		}

		if passwordClientValid {
			if err := setBitbucketPassword(instance.Name(), bc.getPassword()); err != nil {
				return nil, cerr.AppendError("Failed to apply newly obtained token to conf", err)
			}
		}
	} else { // valid token, nothing left to do
		return &bc, nil
	}

	if !tokenClientValid && !passwordClientValid {
		return nil, cerr.NewError("Failed to create a new bitbucket client, all authentication methods failed to produce a valid client")
	}

	if !shouldCreateToken {
		return &bc, nil
	}

	if err := bc.deleteTokenIfAny(); err != nil {
		return nil, cerr.AppendError("Failed to clear state of CDS token", err)
	}

	if err := bc.createToken(); err != nil {
		return nil, cerr.AppendError("Failed to generate new Bitbucket token on the fly !", err)
	}

	clog.Info("New Bitbucket token created, stored in configuration for later use !")

	if err := setBitbucketToken(instance.Name(), bc.getToken()); err != nil {
		return nil, cerr.AppendError("Failed to apply newly obtained token to conf", err)
	}

	return &bc, nil
}

func newClientUsingToken(instance bitbucketInstance) (bitbucketClient, bool, error) {
	if instance.err != nil {
		return bitbucketClient{}, false, cerr.AppendError("Failed to create new bitbucket client, an error occurred in the instance", instance.err)
	}

	bc := bitbucketClient{instance: instance, instancePath: instance.BaseHttpUrl(), httpClient: NewClient(HttpAuth{})}

	user, err := getBitbucketUser(instance.Name())
	if err != nil {
		return bitbucketClient{}, false, cerr.AppendError("Failed to retrieve bitbucket user to instantiate a new client", err)
	}

	bc.setUsername(user)

	token, err := getBitbucketToken(instance.Name())
	if err != nil {
		return bitbucketClient{}, false, cerr.AppendError("Failed to retrieve bitbucket token to instantiate a new client", err)
	}

	authenticated := false
	if len(token.token) > 0 {
		bc.setToken(token)

		authenticated, err = bc.ValidateAuthentication()
		if err != nil {
			return bitbucketClient{}, false, cerr.AppendError("Failed to authenticate using provided token", err)
		}
	}

	return bc, authenticated, nil
}

func newClientUsingCallbackPassword(instance bitbucketInstance) (bitbucketClient, bool, error) {
	if instance.err != nil {
		return bitbucketClient{}, false, cerr.AppendError("Failed to create new bitbucket client, an error occurred in the instance", instance.err)
	}

	bc := bitbucketClient{instance: instance, instancePath: instance.BaseHttpUrl(), httpClient: NewClient(HttpAuth{})}

	user, err := getBitbucketUser(instance.Name())
	if err != nil {
		return bitbucketClient{}, false, cerr.AppendError("Failed to retrieve bitbucket user to instantiate a new client", err)
	}

	bc.setUsername(user)
	authenticated := false

	for i := 0; i < 3; i++ {
		pwd, errPwd := getBitbucketPassword(instance.Name())
		if errPwd != nil {
			return bitbucketClient{}, false, cerr.AppendError("Failed to instantiate bitbucket client, failed to get password for authentication", errPwd)
		}
		if len(pwd) == 0 {
			continue
		}

		bc.setPassword(pwd)

		authenticated, err = bc.ValidateAuthentication()

		if err != nil {
			return bitbucketClient{}, false, cerr.AppendError("Failed to authenticate using provided password", err)
		}

		if !authenticated {
			clog.Warn("Failed to authenticate to Bitbucket using password, try re-entering your password (Bitbucket authenticates against Active directory, too many password fails will lock your AD account !)")
		} else {
			break
		}
	}

	return bc, authenticated, nil
}

// implements transport.AuthMethod
type bitbucketGitAuthMethod struct {
	name            string
	bitbucketClient *bitbucketClient
}

func (bbAuth *bitbucketGitAuthMethod) SetAuth(r *http.Request) {
	errAuth := bbAuth.bitbucketClient.httpClient.authenticateRequest(r)
	if errAuth != nil {
		clog.Debug("Failed to authenticate bitbucket request", errAuth)
	}
}

func (bbAuth *bitbucketGitAuthMethod) String() string {
	return bbAuth.name
}

func (bbAuth *bitbucketGitAuthMethod) Name() string {
	return bbAuth.name
}

func GetBitbucketHostname(bbInstanceName string) (string, error) {
	bbInstance := BitbucketInstanceFromName(bbInstanceName)
	bbHttpUrl, err := url.Parse(bbInstance.HttpUrl())
	if err != nil {
		return "", cerr.AppendError(fmt.Sprintf("Could not setup entry for Bitbucket instance %s", bbInstanceName), err)
	}

	return bbHttpUrl.Hostname(), nil
}
