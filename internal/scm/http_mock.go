package scm

import (
	"io"
	"net/http"
	"net/url"
)

type mockedClient struct {
	Auth       HttpAuth
	HttpClient *http.Client
	DeleteFn   func(url url.URL) (*HttpResponse, error)
	GetFn      func(url url.URL) (*HttpResponse, error)
	PutFn      func(url url.URL, data io.Reader) (*HttpResponse, error)
}

var _ httpClient = (*mockedClient)(nil)

// func newMockedClient(getFn, deleteFn func(url url.URL) (*HttpResponse, error), putFn func(url url.URL, data io.Reader) (*HttpResponse, error)) *mockedClient {
// 	return &mockedClient{GetFn: getFn, Auth: HttpAuth{}, HttpClient: nil, DeleteFn: deleteFn, PutFn: putFn}
// }

func (c *mockedClient) Delete(url url.URL) (*HttpResponse, error) {
	if c.DeleteFn != nil {
		return c.DeleteFn(url)
	}
	return nil, nil
}

func (c *mockedClient) Get(url url.URL) (*HttpResponse, error) {
	if c.GetFn != nil {
		return c.GetFn(url)
	}
	return nil, nil
}

func (c *mockedClient) Put(url url.URL, data io.Reader) (*HttpResponse, error) {
	if c.PutFn != nil {
		return c.PutFn(url, data)
	}
	return nil, nil
}
