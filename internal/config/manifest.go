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
	if _, err := cliConfigSource(); err != nil {
		return err
	}
	if _, err := profileSource(); err != nil {
		return err
	}
	if _, err := DBSource(); err != nil {
		return err
	}
	return nil
}

func ProfileReader() (io.Reader, error) {
	src, err := profileSource()
	if err != nil {
		return nil, err
	}
	return src.Read()
}

// DBSource returns the resolved source for the state database.
// The db package accepts source.Source directly so callers can pass
// the return value through without importing source themselves.
func DBSource() (source.Source, error) {
	return dbSource()
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

func cliConfigSource() (source.Source, error) {
	src, err := manifestSource(SourceKeyCLIAgentConfig)
	if err != nil {
		return nil, cerr.AppendError("failed to resolve CLI agent config source", err)
	}
	if err := EnsureSourceWithDefault(src, defaultCLIAgentConfig(), cg.KPermFile); err != nil {
		return nil, cerr.AppendError("failed to ensure CLI agent config exists", err)
	}
	return src, nil
}

func profileSource() (source.Source, error) {
	src, err := manifestSource(SourceKeyProfile)
	if err != nil {
		return nil, cerr.AppendError("failed to resolve profile source", err)
	}
	return src, nil
}

func dbSource() (source.Source, error) {
	src, err := manifestSource(SourceKeyDB)
	if err != nil {
		return nil, cerr.AppendError("failed to resolve db source", err)
	}
	if err := EnsureSourceWithDefault(src, strings.NewReader("{}"), cg.KPermFile); err != nil {
		return nil, cerr.AppendError("failed to ensure db file exists", err)
	}
	return src, nil
}

func manifestSource(key string) (source.Source, error) {
	loadedManifest, err := LoadManifest()
	if err != nil {
		return nil, cerr.AppendError("failed to load manifest", err)
	}
	src, err := loadedManifest.Resolve(key)
	if err != nil {
		return nil, err
	}
	return src, nil
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
