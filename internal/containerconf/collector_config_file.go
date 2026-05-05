package containerconf

import (
	"fmt"
	"strings"
)

type localArtifactReference struct {
	sourcePath string
	identifier string
}

type localArtifactReferenceFactory func(sourcePath string) (localArtifactReference, error)

// configFileCollector emits one local file artifact declared by a string config
// value. The config path and identifier policy are provided by the registry.
type configFileCollector struct {
	kind         string
	configPath   []string
	newReference localArtifactReferenceFactory
}

func newConfigFileCollector(kind string, configPath []string, newReference localArtifactReferenceFactory) configFileCollector {
	return configFileCollector{
		kind:         kind,
		configPath:   configPath,
		newReference: newReference,
	}
}

func (c configFileCollector) Kind() string { return c.kind }

func (c configFileCollector) Collect(ctx collectContext) ([]RequiredArtifact, error) {
	if ctx.Config == nil {
		return nil, fmt.Errorf("container configuration is required")
	}
	if len(c.configPath) == 0 {
		return nil, fmt.Errorf("collector config path is required")
	}
	if c.newReference == nil {
		return nil, fmt.Errorf("collector reference factory is required")
	}
	if !ctx.Config.IsSet(c.configPath...) {
		return nil, nil
	}

	sourcePath, err := configString(ctx.Config, c.configPath...)
	if err != nil {
		return nil, err
	}

	reference, err := c.newReference(sourcePath)
	if err != nil {
		return nil, err
	}

	source, err := resolveLocalArtifactSource(ctx.ConfigDir, reference.sourcePath)
	if err != nil {
		return nil, err
	}

	return []RequiredArtifact{{Identifier: reference.identifier, Source: source}}, nil
}

func configString(config *Config, key ...string) (string, error) {
	if config == nil {
		return "", fmt.Errorf("container configuration is required")
	}

	name, err := configPathName(key...)
	if err != nil {
		return "", err
	}

	value, ok := config.Get(key...).(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must not be empty", name)
	}

	return value, nil
}

func configPathName(key ...string) (string, error) {
	if len(key) == 0 {
		return "", fmt.Errorf("config path is required")
	}
	for _, part := range key {
		if strings.TrimSpace(part) == "" {
			return "", fmt.Errorf("config path is required")
		}
	}
	return strings.Join(key, "."), nil
}
