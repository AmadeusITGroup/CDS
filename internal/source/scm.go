package source

import (
	"github.com/amadeusitgroup/cds/internal/clog"
)

// scmSource implements Source backed by a git-based SCM repository.
type scmSource struct {
	baseSource
	repoUrl string
	ref     string
	path    string // path within the repository
}

// Compile-time check that scmSource satisfies Source.
var _ Source = (*scmSource)(nil)

// NewSCMSource creates a Source backed by a git repository at repoUrl
// checked out at the given ref (branch, tag, or commit SHA), pointing
// to the given path within the repository.
func NewSCMSource(repoUrl, ref, path string) (Source, error) {
	clog.Warn("source.NewSCMSource Not Implemented")
	return &scmSource{repoUrl: repoUrl, ref: ref, path: path}, nil
}

func (s *scmSource) Type() SourceType {
	return SCM
}

func (s *scmSource) Information() string {
	return s.repoUrl + "@" + s.ref + ":" + s.path
}
