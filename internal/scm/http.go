package scm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

type HttpAuth struct {
	Username    string
	Password    string
	AccessToken string
}

type Client struct {
	Auth       HttpAuth
	HttpClient *http.Client
}

type HttpResponse struct {
	code int
	body []byte
}

type bitBucketAPIError struct {
	message string
}

type httpClient interface {
	Get(url url.URL) (*HttpResponse, error)
	Put(url url.URL, data io.Reader) (*HttpResponse, error)
	Delete(url url.URL) (*HttpResponse, error)
}

var _ httpClient = (*Client)(nil)

// create a new client object for an http connection
// auth should be pre-filled with valid credentials
// url should be a valid http address
func NewClient(a HttpAuth) *Client {
	return &Client{
		Auth:       a,
		HttpClient: new(http.Client),
	}
}

func (e *bitBucketAPIError) Error() string {
	return fmt.Sprintf("Bitbucket API error: %s", e.message)
}

func (hr *HttpResponse) parseError() error {
	if hr.code >= http.StatusOK && hr.code < http.StatusMultipleChoices {
		return nil
	}

	type bbAPIError struct {
		Message string `json:"message"`
	}

	hrBodyError := struct {
		Errors []bbAPIError `json:"errors"`
	}{}

	err := json.Unmarshal(hr.body, &hrBodyError)
	if err != nil {
		return &bitBucketAPIError{message: "Failed to parse the error"}
	}

	var errorsReport string
	for _, e := range hrBodyError.Errors {
		errorsReport += fmt.Sprintf("HTTP code %v - %s\n", hr.code, e.Message)
	}
	return &bitBucketAPIError{message: errorsReport}
}

func (c *Client) Get(url url.URL) (*HttpResponse, error) {
	req, err := http.NewRequest("GET", url.String(), nil)

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to create GET request to (%s)", url.String()), err)
	}

	if c.Auth != (HttpAuth{}) {
		if err = c.authenticateRequest(req); err != nil {
			return nil, cerr.AppendError("Failed to authenticate request", err)
		}
	} else {
		clog.Debug("The HTTP GET request at", url, "will be made without authentication !")
	}

	reader, code, err := c.doRequest(req, false)

	if err != nil {
		return &HttpResponse{body: nil, code: code}, cerr.AppendError(fmt.Sprintf("Failed to make GET request to (%s)", url.String()), err)
	}

	body, err := io.ReadAll(reader)

	if err != nil {
		return &HttpResponse{body: nil, code: code}, cerr.AppendError(fmt.Sprintf("Failed to read response to GET request to (%s)", url.String()), err)
	}

	response := &HttpResponse{body: body, code: code}

	return response, nil
}

func (c *Client) Delete(url url.URL) (*HttpResponse, error) {
	req, err := http.NewRequest("DELETE", url.String(), nil)

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to create GET request to (%s)", url.String()), err)
	}

	if c.Auth != (HttpAuth{}) {
		if err = c.authenticateRequest(req); err != nil {
			return nil, cerr.AppendError("Failed to authenticate request", err)
		}
	} else {
		clog.Debug("The HTTP PUT request at", url, "will be made without authentication !")
	}

	bodyOutput, code, err := c.doRequest(req, false)

	if err != nil {
		return &HttpResponse{body: nil, code: code}, cerr.AppendError(fmt.Sprintf("Failed to make GET request to (%s)", url.String()), err)
	}

	if code == http.StatusNoContent {
		return &HttpResponse{code: code}, nil
	}

	body, err := io.ReadAll(bodyOutput)

	if err != nil {
		return &HttpResponse{body: nil, code: code}, cerr.AppendError(fmt.Sprintf("Failed to read response to GET request to (%s)", url.String()), err)
	}

	response := &HttpResponse{body: body, code: code}

	return response, nil
}

func (c *Client) Put(url url.URL, data io.Reader) (*HttpResponse, error) {
	req, err := http.NewRequest("PUT", url.String(), data)

	if err != nil {
		return nil, cerr.AppendError(fmt.Sprintf("Failed to create GET request to (%s)", url.String()), err)
	}

	if c.Auth != (HttpAuth{}) {
		if err = c.authenticateRequest(req); err != nil {
			return nil, cerr.AppendError("Failed to authenticate request", err)
		}
	} else {
		clog.Debug("The HTTP PUT request at", url, "will be made without authentication !")
	}

	// TODO:Feature: add type for header input
	req.Header.Add("Content-Type", "application/json")

	bodyOutput, code, err := c.doRequest(req, false)

	if err != nil {
		return &HttpResponse{body: nil, code: code}, cerr.AppendError(fmt.Sprintf("Failed to make GET request to (%s)", url.String()), err)
	}

	body, err := io.ReadAll(bodyOutput)

	if err != nil {
		return &HttpResponse{body: nil, code: code}, cerr.AppendError(fmt.Sprintf("Failed to read response to GET request to (%s)", url.String()), err)
	}

	response := &HttpResponse{body: body, code: code}

	return response, nil
}

// fills an http.Request with credentials from a Client obj
func (c *Client) authenticateRequest(req *http.Request) error {
	// This is a workround for the CI (integration tests).
	// The robotic user does not rely on Bitbucket tokens, its password is used instead and is stored in Secret.json as a token (see space/secrets.go).
	if c.Auth.Username == os.Getenv("BITBUCKET_USER") {
		req.SetBasicAuth(c.Auth.Username, c.Auth.AccessToken)
	} else if len(c.Auth.Username) > 0 && len(c.Auth.Password) > 0 {
		req.SetBasicAuth(c.Auth.Username, c.Auth.Password)
	} else if len(c.Auth.AccessToken) > 0 {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.Auth.AccessToken))
	} else {
		return cerr.NewError("No authentication method available, aborting !")
	}

	return nil
}

func (c *Client) doRequest(req *http.Request, emptyResponse bool) (io.ReadCloser, int, error) {
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		if resp == nil {
			return nil, -1, cerr.AppendError("Failed to make HTTP request", err)
		} else {
			return nil, resp.StatusCode, cerr.AppendError("Failed to make HTTP request", err)
		}
	}

	// TODO:FixMe: proper handling of HTTP codes
	if resp.StatusCode > 499 {
		_ = resp.Body.Close()
		return nil, resp.StatusCode, cerr.NewError(fmt.Sprintf("Request failed with HTTP code: %v", resp.StatusCode))
	}

	if emptyResponse || resp.StatusCode == http.StatusNoContent {
		_ = resp.Body.Close()
		return nil, resp.StatusCode, nil
	}

	if resp.Body == nil {
		return nil, resp.StatusCode, cerr.NewError(fmt.Sprintf("Empty response to HTTP request (code %v)", resp.StatusCode))
	}

	return resp.Body, resp.StatusCode, nil
}
