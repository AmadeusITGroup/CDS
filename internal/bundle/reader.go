package bundle

import (
	"archive/tar"
	"fmt"
	"io"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
)

type reader struct {
	r       *tar.Reader
	datamap map[entry][]byte
}

type readerOpt func(*reader)

func WithReader(r io.Reader) readerOpt {
	return func(reader *reader) {
		reader.r = tar.NewReader(r)
	}
}

func NewReader(opts ...readerOpt) (*reader, error) {
	r := &reader{
		datamap: make(map[entry][]byte),
	}
	for _, o := range opts {
		o(r)
	}

	if r.r == nil {
		return nil, cerr.NewError("cannot create a bundle reader with a nil reader")
	}
	if errLoad := r.load(); errLoad != nil {
		return nil, cerr.AppendError("failed during bundle loading", errLoad)
	}

	return r, nil
}

// load is a helper to read from the archive and prepare a map of all known entries
//
// TODO: Be smarter about our memory consumption as this loads everything in memory which could be problematic if we are transporting a lot of data in the bundle.
// For instance an unhinged user of userconfigfile could make the agent in a OOM scenario
// Maybe we can find a way to unload in filesystem and read from there with reduced i/o
func (r *reader) load() error {
	for {
		hdr, err := r.r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return cerr.AppendError("bundle: reading tar", err)
		}

		buf, err := io.ReadAll(r.r)
		if err != nil {
			return fmt.Errorf("bundle: reading %s: %w", hdr.Name, err)
		}
		path := strings.SplitN(hdr.Name, kSeparator, 3)
		if len(path) != 3 {
			return cerr.NewError("")
		}
		e := NewEntryForRessource(path[0], path[1], path[2])
		r.datamap[e] = buf
	}
	return nil
}

func (r *reader) Read(e entry) ([]byte, error) {
	if data, exists := r.datamap[e]; exists {
		return data, nil
	}
	return nil, cerr.NewError(fmt.Sprintf("couldn't find resource %q in bundle", e.String()))
}

func (r *reader) Exists(e entry) bool {
	_, ok := r.datamap[e]
	return ok
}
