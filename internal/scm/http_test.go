package scm

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/stretchr/testify/assert"
)

var testServerGlobal = httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
	switch req.RequestURI {
	case "/TestClient":
		res.WriteHeader(200)
		if _, err := res.Write([]byte("body")); err != nil {
			clog.Warn("", err)
		}
	case "/TestClientAuth":
		user, pass, ok := req.BasicAuth()
		if !ok || user != "testuser" || pass != "testpassword" {
			res.WriteHeader(403)
			if _, err := res.Write([]byte("denied")); err != nil {
				clog.Warn("", err)
			}

		} else {
			res.WriteHeader(200)
			if _, err := res.Write([]byte("body")); err != nil {
				clog.Warn("", err)
			}
		}
	}
}))

func TestClient(t *testing.T) {
	expectedBody := []byte("body")

	client := NewClient(HttpAuth{})
	req, err := http.NewRequest(http.MethodGet, testServerGlobal.URL+"/TestClient", nil)
	assert.NoError(t, err)

	ioReader, _, err := client.doRequest(req, false)

	assert.NoError(t, err)

	body, err := io.ReadAll(ioReader)

	assert.NoError(t, err)

	assert.Equal(t, expectedBody, body)
}

func TestClientAuth(t *testing.T) {
	auth := HttpAuth{Username: "testuser", Password: "testpassword"}
	expectedBody := []byte("body")

	client := NewClient(auth)
	req, err := http.NewRequest(http.MethodGet, testServerGlobal.URL+"/TestClientAuth", nil)
	assert.NoError(t, err)
	err = client.authenticateRequest(req)
	assert.NoError(t, err)

	ioReader, _, err := client.doRequest(req, false)

	assert.NoError(t, err)

	body, err := io.ReadAll(ioReader)

	assert.NoError(t, err)

	assert.Equal(t, expectedBody, body)
}

func TestBBAPIError(t *testing.T) {

	bbAPIError := bitBucketAPIError{message: "This is a test error"}
	expectedError := fmt.Sprintf("Bitbucket API error: %s", bbAPIError.message)

	assert.Equal(t, expectedError, bbAPIError.Error())
}

func TestParseError(t *testing.T) {
	hr := HttpResponse{code: 200, body: []byte("body")}
	err := hr.parseError()
	assert.NoError(t, err)

	hr = HttpResponse{code: 400, body: []byte("body")}
	err = hr.parseError()
	assert.Error(t, err)

	hr = HttpResponse{code: 400, body: []byte("")}
	err = hr.parseError()
	assert.Error(t, err)

	hr = HttpResponse{code: 400, body: []byte(`{"errors":[{"context":null,"message":"This is a test error","exceptionName":"com.bitbucket.api.error"}]}`)}
	err = hr.parseError()
	assert.Error(t, err)

	expectedError := "Bitbucket API error: Failed to parse the error"
	hr = HttpResponse{code: 400, body: []byte("")}
	err = hr.parseError()
	assert.Equal(t, expectedError, err.Error())

	expectedError = "Bitbucket API error: HTTP code 400 - This is a test error\n"
	hr = HttpResponse{code: 400, body: []byte(`{"errors":[{"context":null,"message":"This is a test error","exceptionName":"com.bitbucket.api.error"}]}`)}
	err = hr.parseError()
	assert.Equal(t, expectedError, err.Error())
}
