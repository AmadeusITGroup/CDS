package containerconf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cos"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	defaultAuthorizedKeysFileName = "authorized_keys"
	defaultPublicKeyFileName      = "id_rsa.pub"
	registryAuthFileEnv           = "REGISTRY_AUTH_FILE"
)

// sourceFactory resolves a default singleton artifact source. The bool return
// tells the collector whether the optional source exists and should be emitted.
type sourceFactory func(collectContext) (SourceRef, bool, error)

// sourceCollector emits one singleton resource from a default source that is not
// declared in devcontainer.json, such as registry auth or SSH access material.
type sourceCollector struct {
	kind      string
	newSource sourceFactory
}

func newDefaultSourceCollector(kind string, newSource sourceFactory) sourceCollector {
	return sourceCollector{kind: kind, newSource: newSource}
}

func (c sourceCollector) Kind() string { return c.kind }

func (c sourceCollector) Collect(ctx collectContext) ([]RequiredArtifact, error) {
	if c.newSource == nil {
		return nil, fmt.Errorf("collector source factory is required")
	}

	source, ok, err := c.newSource(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	identifier, err := SingletonIdentifier(c.kind)
	if err != nil {
		return nil, err
	}

	return []RequiredArtifact{{Identifier: identifier, Source: source}}, nil
}

func optionalLocalFileSource(sourcePath string) (SourceRef, bool, error) {
	return localFileSource(sourcePath, false)
}

func requiredLocalFileSource(sourcePath string) (SourceRef, bool, error) {
	return localFileSource(sourcePath, true)
}

func localFileSource(sourcePath string, required bool) (SourceRef, bool, error) {
	sourcePath = filepath.Clean(sourcePath)
	info, err := cos.Fs.Stat(sourcePath)
	if os.IsNotExist(err) {
		if required {
			return SourceRef{}, false, fmt.Errorf("default artifact source %q does not exist", sourcePath)
		}
		return SourceRef{}, false, nil
	}
	if err != nil {
		return SourceRef{}, false, err
	}
	if info.IsDir() {
		return SourceRef{}, false, fmt.Errorf("default artifact source %q is a directory", sourcePath)
	}
	return SourceRef{Type: SourceTypeLocalFS, Ref: sourcePath}, true, nil
}

// defaultAuthFileSource copies the client registry auth file when available.
// REGISTRY_AUTH_FILE is treated as an explicit required override; otherwise CDS
// uses its default client config auth.json path and skips the artifact if absent.
func defaultAuthFileSource(collectContext) (SourceRef, bool, error) {
	if sourcePath := strings.TrimSpace(os.Getenv(registryAuthFileEnv)); sourcePath != "" {
		return requiredLocalFileSource(sourcePath)
	}

	sourcePath, err := defaultAuthFilePath()
	if err != nil {
		return SourceRef{}, false, err
	}
	return optionalLocalFileSource(sourcePath)
}

func defaultAuthFilePath() (string, error) {
	return cenv.ConfigFile(cg.KContainerAuthFileName), nil
}

// defaultAuthorizedKeysSource generates an authorized_keys payload from the
// user's default public key. The engine consumes it through the pub_key singleton
// resource and installs the key into the container user's ~/.ssh/authorized_keys.
func defaultAuthorizedKeysSource(collectContext) (SourceRef, bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return SourceRef{}, false, err
	}

	// TODO: Feature: Generate the Key pair per host / per container, instead of relying on the user to have a pre-existing public key.
	publicKeyPath := filepath.Join(homeDir, ".ssh", defaultPublicKeyFileName)
	info, err := cos.Fs.Stat(publicKeyPath)
	if os.IsNotExist(err) {
		return SourceRef{}, false, nil
	}
	if err != nil {
		return SourceRef{}, false, err
	}
	if info.IsDir() {
		return SourceRef{}, false, fmt.Errorf("default SSH public key source %q is a directory", publicKeyPath)
	}

	data, err := cos.ReadFile(publicKeyPath)
	if err != nil {
		return SourceRef{}, false, err
	}

	line := strings.TrimSpace(string(data))
	if line == "" {
		return SourceRef{}, false, fmt.Errorf("default SSH public key source %q is empty", publicKeyPath)
	}

	return SourceRef{
		Type: SourceTypeInline,
		Ref:  defaultAuthorizedKeysFileName,
		Data: []byte(line + "\n"),
	}, true, nil
}
