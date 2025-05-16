package db

import (
	"encoding/json"
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

var (
	sStore *store
)

/***********************************************************/
/*                                                         */
/*                           API                           */
/*                                                         */
/***********************************************************/

func Load() error {
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
	return &store{
		d: data{
			Profile: profilePath{Path: getPathToDefaultProfile()},
		},
	}
}

func resetContent() {
	sStore = nil
}

func getDBContent() (*store, error) {
	if sStore == nil {
		sStore = newDB()
		sStore.Lock()
		defer sStore.Unlock()
		dbFilePath := getPathToCdsDBFile()
		if cos.NotExist(dbFilePath) {
			clog.Debug(fmt.Sprintf("%s file does not exist! Assuming a cds space init use case", dbFilePath))
			return sStore, nil
		}

		if err := parseFile(dbFilePath, sStore); err != nil {
			return sStore, cerr.AppendError("Failed to parse CDS database file", err)
		}
	}
	return sStore, nil
}

func saveDBContent() error {
	instance().Lock()
	defer instance().Unlock()

	bytes, jsonErr := json.MarshalIndent(instance().d, "", "  ")
	if jsonErr != nil {
		return cerr.AppendError("Failed serialize configuration", jsonErr)
	}
	if ioErr := cos.WriteFile(getPathToCdsDBFile(), bytes, cg.KPermFile); ioErr != nil {
		return cerr.AppendError("Failed write configuration", ioErr)
	}
	return nil
}

func parseFile(path string, b bom) error {
	if err := b.unmarshall(path); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to deserialize file (%v)", path), err)
	}
	return nil
}

const (
	kCdsStateFile = "db.json"
	kProfileFile  = "profile.json"
)

func getPathToCdsDBFile() string {
	return cenv.ConfigFile(kCdsStateFile)
}

func getPathToDefaultProfile() string {
	return cenv.ConfigFile(kProfileFile)
}
