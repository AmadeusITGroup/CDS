package db

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/source"
)

var (
	sStore *store
	dbSrc  source.Source
)

/***********************************************************/
/*                                                         */
/*                           API                           */
/*                                                         */
/***********************************************************/

func Load(src source.Source) error {
	dbSrc = src
	_, err := getDBContent()
	if err != nil {
		return cerr.AppendError("Failed to initialize cds configuration", err)
	}
	return nil
}

func Save() error {
	if err := saveDBContent(); err != nil {
		return cerr.AppendError("Failed to save configuration to disk", err)
	}
	return nil
}

/***********************************************************/
/*                                                         */
/*                         Helpers                         */
/*                                                         */
/***********************************************************/

func instance() *store {
	if sStore == nil {
		clog.Warn("Store object is not initialized. Creating a new one")
		sStore = newDB()
	}
	return sStore
}

func newDB() *store {
	return &store{}
}

func resetContent() {
	sStore = nil
	dbSrc = nil
}

func getDBContent() (*store, error) {
	if sStore == nil {
		sStore = newDB()
		sStore.Lock()
		defer sStore.Unlock()

		exists, err := dbSrc.Exists()
		if err != nil {
			return sStore, cerr.AppendError("Failed to check db source existence", err)
		}
		if !exists {
			clog.Debug(fmt.Sprintf("%s does not exist! Assuming a cds space init use case", dbSrc.Information()))
			return sStore, nil
		}

		if err := parseSource(sStore); err != nil {
			return sStore, cerr.AppendError("Failed to parse CDS database file", err)
		}
	}
	return sStore, nil
}

func saveDBContent() error {
	instance().Lock()
	defer instance().Unlock()

	data, jsonErr := json.MarshalIndent(instance().d, "", "  ")
	if jsonErr != nil {
		return cerr.AppendError("Failed serialize configuration", jsonErr)
	}
	if ioErr := dbSrc.Write(bytes.NewReader(data), cg.KPermFile); ioErr != nil {
		return cerr.AppendError("Failed write configuration", ioErr)
	}
	return nil
}

func parseSource(b bom) error {
	r, err := dbSrc.Read()
	if err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to read source (%v)", dbSrc.Information()), err)
	}
	if err := b.unmarshall(r); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to deserialize source (%v)", dbSrc.Information()), err)
	}
	return nil
}
