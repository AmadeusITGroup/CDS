package bundle

import (
	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	kSeparator = "/"
)

type entry struct {
	path string
}

func (e entry) String() string {
	return e.path
}

func NewEntryForRessource(fields ...string) entry {
	return entry{path: cg.VariadicJoin(kSeparator, fields...)}
}
