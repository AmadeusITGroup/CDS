package bundle

import (
	"archive/tar"
	"io"
	"sync"

	"github.com/amadeusitgroup/cds/internal/cerr"
)

type writer struct {
	w           *tar.Writer
	mu          *sync.Mutex
	entries     []inMemoryEntry
	seenEntries map[entry]any
	closed      bool
}

type inMemoryEntry struct {
	metadata entry
	data     []byte
}

type writerOpt func(*writer)

// WithWriter specifies the writer to the file archive of the prepared bundle
func WithWriter(ioWriter io.Writer) writerOpt {
	return func(w *writer) {
		w.w = tar.NewWriter(ioWriter)
	}
}

// NewWriter creates a new bundle writer that will be used to transfer data between client and agent
// meant to be later used by the bundle reader.
func NewWriter(opts ...writerOpt) (*writer, error) {
	nw := &writer{
		mu:          &sync.Mutex{},
		seenEntries: make(map[entry]any),
		entries:     make([]inMemoryEntry, 0),
	}
	for _, opt := range opts {
		opt(nw)
	}

	if nw.w == nil {
		return nil, cerr.NewError("cannot create a bundle writer without specifying an io.Writer before")
	}
	return nw, nil
}

func (w *writer) Add(entry entry, dataReader io.Reader) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return cerr.NewError("cannot add files to a closed bundle")
	}

	readData, errRead := io.ReadAll(dataReader)
	if errRead != nil {
		return cerr.AppendErrorFmt("couldn't read dataReader for entry %s", errRead, entry)
	}

	if _, seen := w.seenEntries[entry]; seen {
		return cerr.NewError("already given entry")
	}
	w.seenEntries[entry] = new(any)

	w.entries = append(w.entries, inMemoryEntry{metadata: entry, data: readData})
	return nil
}

func (w *writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return cerr.NewError("bundle already closed, cannot close again")
	}

	var saveErrs []error
	for _, inMemEntry := range w.entries {
		header := tar.Header{
			Name: inMemEntry.metadata.path,
			Size: int64(len(inMemEntry.data)),
		}
		if err := w.w.WriteHeader(&header); err != nil {
			saveErrs = append(saveErrs, err)
			continue
		}
		if _, err := w.w.Write(inMemEntry.data); err != nil {
			saveErrs = append(saveErrs, err)
			continue
		}
	}

	if len(saveErrs) > 0 {
		return cerr.AppendMultipleErrors("issues at bundle closure", saveErrs)
	}

	w.closed = true

	return nil
}
