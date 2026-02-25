package source

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

// SourceType represents the kind of backend a Source is backed by.
type SourceType int

const (
	Undefined   SourceType = iota
	LocalFS                // local filesystem
	SCM                    // git-based source control (Bitbucket, GitHub, …)
	Artifactory            // JFrog Artifactory
)

func (st SourceType) String() string {
	switch st {
	case Undefined:
		return "Undefined"
	case LocalFS:
		return "LocalFS"
	case SCM:
		return "SCM"
	case Artifactory:
		return "Artifactory"
	default:
		return "Unknown"
	}
}

// SourceTypeFromString returns the SourceType matching the given string Unrecognised values map to Undefined.
func SourceTypeFromString(s string) SourceType {
	switch strings.ToLower(s) {
	case "localfs":
		return LocalFS
	case "scm":
		return SCM
	case "artifactory":
		return Artifactory
	default:
		return Undefined
	}
}

// Source is a pointer to a single file or directory in some backend (local filesystem, SCM repository, Artifactory, …).
// A Source is always bound to one path. Children() is used to navigate into sub-entries.
type Source interface {
	// Type returns the SourceType of this source.
	Type() SourceType

	// Information returns a backend-specific description of this source.
	Information() string

	// Read returns an io.Reader over the content of the file this source points to. Returns an error if the source is a directory.
	Read() (io.Reader, error)

	// Write consumes data from r and writes it to the file this source points to with the given permissions. Read-only backends return an error by default via baseSource.
	Write(r io.Reader, perm os.FileMode) error

	// Children returns the immediate child entries of a directory source. Returns an error if the source does not exist.Returns nil without error if the source is a file.
	Children() ([]Source, error)

	// Exists reports whether the path this source points to exists.
	Exists() (bool, error)

	// IsDir reports whether this source points to a directory.
	IsDir() (bool, error)
}

// ---------------------------------------------------------------------------
// baseSource — default implementation
// ---------------------------------------------------------------------------

// baseSource provides default implementations for the Source interface.
// Concrete source types embed this struct and override only the methods they actually support.
type baseSource struct{}

func (b *baseSource) Type() SourceType {
	return Undefined
}

func (b *baseSource) Information() string {
	return ""
}

func (b *baseSource) Read() (io.Reader, error) {
	clog.Warn("Read Not Implemented")
	return nil, cerr.NewError(fmt.Sprintf("source type %q has no Read support", b.Type()))
}

func (b *baseSource) Write(r io.Reader, perm os.FileMode) error {
	return cerr.NewError(fmt.Sprintf("source type %q has no Write support", b.Type()))
}

func (b *baseSource) Children() ([]Source, error) {
	clog.Warn("Children Not Implemented")
	return nil, cerr.NewError(fmt.Sprintf("source type %q has no Children support", b.Type()))
}

func (b *baseSource) Exists() (bool, error) {
	clog.Warn("Exists Not Implemented")
	return false, cerr.NewError(fmt.Sprintf("source type %q has no Exists support", b.Type()))
}

func (b *baseSource) IsDir() (bool, error) {
	clog.Warn("IsDir Not Implemented")
	return false, cerr.NewError(fmt.Sprintf("source type %q has no IsDir support", b.Type()))
}
