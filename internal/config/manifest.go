package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/source"
	"gopkg.in/yaml.v3"
)

const (
	SourceKeyCLIAgentConfig = "cliagentconfig"
	SourceKeyProfile        = "profile"
	SourceKeyDB             = "db"
)

const (
	kManifestFileName = "cdsconfig.yaml"
)

var (
	cliSource  source.Source
	profSource source.Source
	dbSource   source.Source
)

type SourceRef struct {
	Type string `yaml:"type"`
	Ref  string `yaml:"ref"`
}

// Manifest is the root configuration file that maps well-known keys to their SourceRef definitions. It is always stored locally at ~/.xcds/cdsconfig.yaml.
type Manifest struct {
	APIVersion string               `yaml:"apiVersion"`
	Sources    map[string]SourceRef `yaml:"sources"`
}

func InitCLIConfig() error {
	loadedManifest, err := LoadManifest()
	if err != nil {
		return cerr.AppendError("failed to load manifest", err)
	}

	cliSource, err = loadedManifest.Resolve(SourceKeyCLIAgentConfig)
	if err != nil {
		return cerr.AppendError("failed to resolve CLI agent config source", err)
	}
	if err := EnsureSourceWithDefault(cliSource, defaultCLIAgentConfig(), cg.KPermFile); err != nil {
		return cerr.AppendError("failed to ensure CLI agent config exists", err)
	}

	profSource, err = loadedManifest.Resolve(SourceKeyProfile)
	if err != nil {
		return cerr.AppendError("failed to resolve profile source", err)
	}

	dbSource, err = loadedManifest.Resolve(SourceKeyDB)
	if err != nil {
		return cerr.AppendError("failed to resolve db source", err)
	}
	if err := EnsureSourceWithDefault(dbSource, strings.NewReader("{}"), cg.KPermFile); err != nil {
		return cerr.AppendError("failed to ensure db file exists", err)
	}

	return nil
}

func ProfileReader() (io.Reader, error) {
	if profSource == nil {
		return nil, cerr.NewError("config.Init has not been called")
	}
	return profSource.Read()
}

// DBSource returns the resolved source for the state database.
// The db package accepts source.Source directly so callers can pass
// the return value through without importing source themselves.
func DBSource() source.Source {
	return dbSource
}

func LoadManifest() (Manifest, error) {
	manifestPath := cenv.ConfigFile(kManifestFileName)

	manifestSrc, err := source.NewLocalSource(manifestPath)
	if err != nil {
		return Manifest{}, cerr.AppendError("failed to create manifest source", err)
	}

	exists, err := manifestSrc.Exists()
	if err != nil {
		return Manifest{}, cerr.AppendError("failed to check manifest existence", err)
	}

	if !exists {
		m := defaultManifest()
		data, err := yaml.Marshal(m)
		if err != nil {
			return Manifest{}, cerr.AppendError("failed to serialize manifest", err)
		}
		if err := manifestSrc.Write(bytes.NewReader(data), cg.KPermFile); err != nil {
			return Manifest{}, cerr.AppendError("failed to bootstrap manifest", err)
		}
		return m, nil
	}

	r, err := manifestSrc.Read()
	if err != nil {
		return Manifest{}, cerr.AppendError("failed to read manifest file", err)
	}

	var m Manifest
	if err := yaml.NewDecoder(r).Decode(&m); err != nil {
		return Manifest{}, cerr.AppendError("failed to parse manifest", err)
	}
	return m, nil
}

func (m Manifest) Resolve(name string) (source.Source, error) {
	ref, ok := m.Sources[name]
	if !ok {
		return nil, cerr.NewError(fmt.Sprintf("manifest: unknown source key %q", name))
	}

	switch source.SourceTypeFromString(ref.Type) {
	case source.LocalFS:
		if !filepath.IsAbs(ref.Ref) {
			return nil, cerr.NewError(fmt.Sprintf("manifest: localfs ref must be an absolute path, got %q", ref.Ref))
		}
		return source.NewLocalSource(ref.Ref)

	case source.SCM:
		return source.NewSCMSource(ref.Ref, "", "")

	default:
		return nil, cerr.NewError(fmt.Sprintf("manifest: source type %q is not yet supported", ref.Type))
	}
}

func EnsureSourceWithDefault(src source.Source, defaultContent io.Reader, perm os.FileMode) error {
	exists, err := src.Exists()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	return src.Write(defaultContent, perm)
}

func defaultManifest() Manifest {
	return Manifest{
		APIVersion: "v1",
		Sources: map[string]SourceRef{
			SourceKeyCLIAgentConfig: {
				Type: "localfs",
				Ref:  cenv.ConfigFile("cliconfig.yaml"),
			},
			SourceKeyProfile: {
				Type: "localfs",
				Ref:  cenv.ConfigFile("profile.json"),
			},
			SourceKeyDB: {
				Type: "localfs",
				Ref:  cenv.ConfigFile("db.json"),
			},
		},
	}
}
