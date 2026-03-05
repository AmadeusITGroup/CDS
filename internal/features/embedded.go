package features

import (
	"embed"
	"path"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

//go:embed builtinfeatures
var builtinFS embed.FS

const kFeatureManifest = "manifest.json"

type embeddedFeatureFetcher struct{}

func NewEmbeddedFeatureFetcher() embeddedFeatureFetcher {
	return embeddedFeatureFetcher{}
}

func (p *embeddedFeatureFetcher) FetchFile(featureId string) ([]byte, error) {
	featureFile, err := parseFeatureFileIdentifier(featureId)
	if err != nil {
		return nil, cerr.AppendErrorFmt("Couldn't find feature (%q) in the embedded features", err, featureId)
	}
	clog.Debug("Embedded feature fetcher fetching for a file", clog.NewLoggable("featureId", featureFile.String()))
	return nil, cerr.NewError("Not Implemented yet")
}

func (p *embeddedFeatureFetcher) FileExists(featureId string) (bool, error) {
	featureFile, errParse := parseFeatureFileIdentifier(featureId)
	if errParse != nil {
		return false, cerr.AppendErrorFmt("Couldn't find feature (%q) in the embedded features", errParse, featureId)
	}
	manifestPath := path.Join("builtinfeatures", featureFile.name, kFeatureManifest)
	_, err := builtinFS.Open(manifestPath)
	if err != nil {
		return false, nil
	}
	return true, nil
}
