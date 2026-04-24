package containerconf

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// ResourceIdentifier returns the canonical identifier for one logical deploy
// resource. The logical name is path-escaped so config paths such as
// "../Dockerfile" remain stable without becoming staging filesystem traversal.
func ResourceIdentifier(kind, logicalName string) (string, error) {
	normalizedKind, err := normalizeResourceKind(kind)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(logicalName) == "" {
		return "", fmt.Errorf("artifact logical name is required")
	}
	return path.Join(ResourceNamespace, normalizedKind, url.PathEscape(logicalName)), nil
}

// SingletonIdentifier returns the canonical identifier for resources whose
// logical runtime name is the same as their kind, such as auth and SSH key
// artifacts.
func SingletonIdentifier(kind string) (string, error) {
	return ResourceIdentifier(kind, kind)
}

// DockerfileIdentifier returns the canonical identifier for the dockerfile
// declared in the given devcontainer configuration. The identifier is keyed by
// the dockerfile name consumed by runtime paths, not by the relative client-side
// source path used to locate the file on disk.
func DockerfileIdentifier(config *Config) (string, error) {
	if config == nil {
		return "", fmt.Errorf("container configuration is required")
	}

	dockerfilePath, ok := config.Get(KBuild, KBuildDockerfile).(string)
	if !ok {
		return "", fmt.Errorf("%s.%s must be a string", KBuild, KBuildDockerfile)
	}
	if strings.TrimSpace(dockerfilePath) == "" {
		return "", fmt.Errorf("%s.%s must not be empty", KBuild, KBuildDockerfile)
	}

	logicalName := path.Base(strings.ReplaceAll(dockerfilePath, "\\", "/"))
	if logicalName == "." || logicalName == "/" || strings.TrimSpace(logicalName) == "" {
		return "", fmt.Errorf("%s.%s must resolve to a file name", KBuild, KBuildDockerfile)
	}

	return ResourceIdentifier(KindDockerfile, logicalName)
}

// NormalizeIdentifier validates and canonicalizes a logical artifact identifier.
func NormalizeIdentifier(identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", fmt.Errorf("artifact identifier is required")
	}

	if strings.HasPrefix(identifier, ResourceNamespace+"/") {
		parts := strings.Split(identifier, "/")
		if len(parts) != 3 {
			return "", fmt.Errorf("artifact identifier %q must use %s/<kind>/<logical-name>", identifier, ResourceNamespace)
		}

		kind, err := normalizeResourceKind(parts[1])
		if err != nil {
			return "", err
		}
		logicalName, err := url.PathUnescape(parts[2])
		if err != nil {
			return "", fmt.Errorf("artifact identifier %q contains an invalid escaped logical name: %w", identifier, err)
		}
		if logicalName == "" {
			return "", fmt.Errorf("artifact logical name is required")
		}
		return ResourceIdentifier(kind, logicalName)
	}

	normalized := path.Clean(identifier)

	switch {
	case normalized == ".":
		return "", fmt.Errorf("artifact identifier is required")
	case strings.HasPrefix(normalized, "/"):
		return "", fmt.Errorf("artifact identifier %q must be relative", identifier)
	case normalized == ".." || strings.HasPrefix(normalized, "../"):
		return "", fmt.Errorf("artifact identifier %q escapes the staging directory", identifier)
	}

	return normalized, nil
}

func normalizeResourceKind(kind string) (string, error) {
	normalized := strings.TrimSpace(kind)
	switch {
	case normalized == "":
		return "", fmt.Errorf("artifact kind is required")
	case normalized == "." || normalized == "..":
		return "", fmt.Errorf("artifact kind %q is invalid", kind)
	case strings.Contains(normalized, "/"):
		return "", fmt.Errorf("artifact kind %q must not contain path separators", kind)
	}
	return normalized, nil
}
