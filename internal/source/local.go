package source

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/cos"
)

// localSource implements Source backed by the local filesystem.
// Each instance points to a single file or directory.
type localSource struct {
	path string // absolute path to the file or directory
}

var _ Source = (*localSource)(nil)

// NewLocalSource creates a Source pointing to the given path.
func NewLocalSource(path string) (Source, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, cerr.AppendError("failed to resolve absolute path", err)
	}

	return &localSource{path: abs}, nil
}

func (ls *localSource) Type() SourceType {
	return LocalFS
}

func (ls *localSource) Information() string {
	return ls.path
}

func (ls *localSource) Read() (io.Reader, error) {
	info, err := cos.Fs.Stat(ls.path)
	if err != nil {
		return nil, cerr.AppendError("failed to stat source", err)
	}
	if info.IsDir() {
		return nil, cerr.NewError("source is a directory, not a file: " + ls.path)
	}

	data, err := cos.ReadFile(ls.path)
	if err != nil {
		return nil, cerr.AppendError("failed to read file", err)
	}
	return bytes.NewReader(data), nil
}

func (ls *localSource) Write(r io.Reader, perm os.FileMode) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return cerr.AppendError("failed to read data from reader", err)
	}

	if err := cos.EnsureDir(ls.path, 0755); err != nil {
		return cerr.AppendError("failed to ensure parent directories existance", err)
	}

	if err := cos.WriteFile(ls.path, data, perm); err != nil {
		return cerr.AppendError("failed to write file", err)
	}
	return nil
}

func (ls *localSource) Children() ([]Source, error) {
	info, err := cos.Fs.Stat(ls.path)
	if err != nil {
		return nil, cerr.AppendError("failed to stat source", err)
	}
	if !info.IsDir() {
		return nil, cerr.NewError(fmt.Sprintf("Source is not a directory: %s - cannot list children", ls.path))
	}

	fileInfos, err := cos.ReadDir(ls.path)
	if err != nil {
		return nil, cerr.AppendError("failed to read directory", err)
	}

	children := make([]Source, 0, len(fileInfos))
	for _, fi := range fileInfos {
		childPath := filepath.Join(ls.path, fi.Name())
		children = append(children, &localSource{path: childPath})
	}
	return children, nil
}

func (ls *localSource) Exists() (bool, error) {
	return cos.Exists(ls.path), nil
}

func (ls *localSource) IsDir() (bool, error) {
	if cos.NotExist(ls.path) {
		return false, cerr.NewError("path does not exist: " + ls.path)
	}

	info, err := cos.Fs.Stat(ls.path)
	if err != nil {
		return false, cerr.AppendError("failed to stat path", err)
	}
	return info.IsDir(), nil
}
