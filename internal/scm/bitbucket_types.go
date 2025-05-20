package scm

import "github.com/amadeusitgroup/cds/internal/clog"

// this file is only meant to contains types for serializing/deserializing bitbucket API conversations
// a few helper functions to manipulate these type are also present

// most of the bitbucket API is under /rest/api/1.0, the authentication/token management is under /rest/access-tokens/1.0
// see https://docs.atlassian.com/bitbucket-server/rest/5.16.0/bitbucket-rest.html#idm8285279136
// and https://docs.atlassian.com/bitbucket-server/rest/5.16.0/bitbucket-access-tokens-rest.html
// (usage of access tokens doc: https://confluence.atlassian.com/bitbucketserver/http-access-tokens-939515499.html)

// latest API documentation: https://docs.atlassian.com/bitbucket-server/rest/7.16.0/bitbucket-rest.html#idp271

// partial response of GET /rest/api/1.0/project/{PROJECT}/repos/{REPO}
// type repository struct {
// 	Slug  string `json:"slug"`
// 	ScmId string `json:"scmId"`
// 	State string `json:"state"`
// }

// response of GET /rest/api/1.0/project/{PROJECT}/repos/{REPO}/files for file type
type RepositoryFileType struct {
	FileType string `json:"type"`
}

// response of GET /rest/api/1.0/project/{PROJECT}/repos/{REPO}/files
type RepositoryFileList struct {
	Values        []string `json:"values"`
	Size          int      `json:"size"`
	IsLastPage    bool     `json:"isLastPage"`
	Start         int      `json:"start"`
	Limit         int      `json:"limit"`
	NextPageStart int      `json:"nextPageStart"`
}

// response of GET /rest/access-tokens/1.0/users/{USER}/{TOKENID}
type TokenListing struct {
	ID                string   `json:"id"`
	CreatedDate       int64    `json:"createdDate"`
	LastAuthenticated int64    `json:"lastAuthenticated"`
	Name              string   `json:"name"`
	Permissions       []string `json:"permissions"`
	User              struct {
		Name         string `json:"name"`
		EmailAddress string `json:"emailAddress"`
		ID           int    `json:"id"`
		DisplayName  string `json:"displayName"`
		Active       bool   `json:"active"`
		Slug         string `json:"slug"`
		Type         string `json:"type"`
	} `json:"user"`
}

// just a list of token responses to allow hooking methods on this data type
type TokensListing []TokenListing

// request of a new token using PUT /rest/access-tokens/1.0/users/{USER}
type RequestAccessToken struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	ExpiryDays  int32    `json:"expiryDays"`
}

// response of PUT /rest/access-tokens/1.0/users/{USER}
type RequestAccessTokenResponse struct {
	TokenListing
	Token string `json:"token"`
}

// response of GET /rest/access-tokens/1.0/users/{USER}
type TokenListResponse struct {
	Size       int            `json:"size"`
	Limit      int            `json:"limit"`
	IsLastPage bool           `json:"isLastPage"`
	Values     []TokenListing `json:"values"`
	Start      int            `json:"start"`
}

func (tl TokensListing) hasCdsToken() bool {
	for _, t := range tl {
		if t.Name == "CDS-devenv" {
			return true
		}
	}

	return false
}

func (tl TokensListing) getCdsToken() TokenListing {
	for _, t := range tl {
		if t.Name == "CDS-devenv" {
			return t
		}
	}

	clog.Error("CDS token not found in ")
	return TokenListing{}
}

// response of GET /rest/api/1.0/users/{USER}
type bitbucketUserResponse struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
	ID           int    `json:"id"`
	DisplayName  string `json:"displayName"`
	Active       bool   `json:"active"`
	Slug         string `json:"slug"`
	Type         string `json:"type"`
	Links        struct {
		Self []struct {
			Href string `json:"href"`
		} `json:"self"`
	} `json:"links"`
}

// response of GET /rest/api/1.0/project/{project}/repos/{repoSlug}/commits
type bitbucketCommitSearch struct {
	Values        []bitbucketCommit `json:"values"`
	Size          int               `json:"size"`
	IsLastPage    bool              `json:"isLastPage"`
	Start         int               `json:"start"`
	Limit         int               `json:"limit"`
	NextPageStart any               `json:"nextPageStart"`
}

type bitbucketCommit struct {
	ID        string `json:"id"`
	DisplayID string `json:"displayId"`
	Author    struct {
		Name         string `json:"name"`
		EmailAddress string `json:"emailAddress"`
		ID           int    `json:"id"`
		DisplayName  string `json:"displayName"`
		Active       bool   `json:"active"`
		Slug         string `json:"slug"`
		Type         string `json:"type"`
		Links        struct {
			Self []struct {
				Href string `json:"href"`
			} `json:"self"`
		} `json:"links"`
	} `json:"author"`
	AuthorTimestamp int64 `json:"authorTimestamp"`
	Committer       struct {
		Name         string `json:"name"`
		EmailAddress string `json:"emailAddress"`
		ID           int    `json:"id"`
		DisplayName  string `json:"displayName"`
		Active       bool   `json:"active"`
		Slug         string `json:"slug"`
		Type         string `json:"type"`
		Links        struct {
			Self []struct {
				Href string `json:"href"`
			} `json:"self"`
		} `json:"links"`
	} `json:"committer"`
	CommitterTimestamp int64  `json:"committerTimestamp"`
	Message            string `json:"message"`
	Parents            []struct {
		ID        string `json:"id"`
		DisplayID string `json:"displayId"`
	} `json:"parents"`
}

type bitbucketSubmodule struct {
	Branch string
	Path   string
	Url    string
}
