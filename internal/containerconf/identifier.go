package containerconf

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// resourceIdentifier returns the canonical identifier for one logical deploy
// resource. The logical name is path-escaped so config paths such as
// "../Dockerfile" remain stable without becoming staging filesystem traversal.
func resourceIdentifier(kind, logicalName string) (string, error) {
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
	return resourceIdentifier(kind, kind)
}

// DockerfileIdentifier returns the canonical identifier for the dockerfile
// declared in the given devcontainer configuration.
//
// This function is exported because both the client-side artifact collector and
// the server-side engine must derive the same identifier independently. It lives
// with the identifier helpers rather than the dockerfile collector because it
// defines naming policy only; collecting the source file is a separate concern.
func DockerfileIdentifier(config *Config) (string, error) {
	dockerfilePath, err := configString(config, KBuild, KBuildDockerfile)
	if err != nil {
		return "", err
	}

	reference, err := newDockerfileReference(dockerfilePath)
	if err != nil {
		return "", err
	}
	return reference.identifier, nil
}

func newDockerfileReference(dockerfilePath string) (localArtifactReference, error) {
	logicalName := path.Base(strings.ReplaceAll(dockerfilePath, "\\", "/"))
	if logicalName == "." || logicalName == "/" || strings.TrimSpace(logicalName) == "" {
		return localArtifactReference{}, fmt.Errorf("%s.%s must resolve to a file name", KBuild, KBuildDockerfile)
	}

	identifier, err := resourceIdentifier(KindDockerfile, logicalName)
	if err != nil {
		return localArtifactReference{}, err
	}

	return localArtifactReference{sourcePath: dockerfilePath, identifier: identifier}, nil
}

// normalizeIdentifier validates and canonicalizes a logical artifact identifier.
func normalizeIdentifier(identifier string) (string, error) {
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
		return resourceIdentifier(kind, logicalName)
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
