package features

import (
	"fmt"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// ExecPlan describes scripts to execute and which user to run them as.
type ExecPlan struct {
	Files []string `json:"files"`
	As    string   `json:"as"`
}

// ResolvedFeature is a fully-resolved feature ready for engine execution.
// Callers obtain these via ResolveFeatures and pass them to the engine.
type ResolvedFeature struct {
	Name        string
	Version     string
	OnHost      ExecPlan
	OnContainer ExecPlan
}

const kFeatureFileIdentifierSeparator = ";"

// featureFileIdentifier builds the composite identifier for feature_file lookups.
func FeatureFileIdentifier(name, version, relPath string) string {
	return cg.VariadicJoin(kFeatureFileIdentifierSeparator, name, version, relPath)
}

type featureFile struct{ name, version, relpath string }

func (fFile featureFile) String() string {
	return fmt.Sprintf("%s@%s:%s", fFile.name, fFile.version, fFile.relpath)
}

func parseFeatureFileIdentifier(featureIdentifier string) (featureFile, error) {
	fields := strings.Split(featureIdentifier, kFeatureFileIdentifierSeparator)
	if len(fields) != 3 {
		return featureFile{}, cerr.NewError(fmt.Sprintf("Could not parse following feature: %q", featureIdentifier))
	}

	return featureFile{name: fields[0], version: fields[1], relpath: fields[2]}, nil
}
