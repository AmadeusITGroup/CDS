package containerconf

import (
	"fmt"
	"path/filepath"
)

const (
	// SourceTypeLocalFS identifies a client-side artifact available on the local filesystem.
	SourceTypeLocalFS = "localfs"
	// SourceTypeInline identifies a generated artifact whose bytes are already available in SourceRef.Data.
	SourceTypeInline = "inline"
)

// SourceRef describes how the client can obtain an artifact before uploading it
// to the agent.
//
// Type selects the resolver implementation (for example localfs, and later
// potentially git, http, or oci). Ref is the primary locator understood by that
// resolver. Params is reserved for small resolver-specific fetch hints only,
// such as a git ref or OCI platform; it must not carry artifact semantics like
// destination paths, permissions, or secrets. The reference is intentionally
// opaque to containerconf beyond the local filesystem helper below so new
// source types can be introduced incrementally. Data carries inline generated
// source bytes when Type is SourceTypeInline.
type SourceRef struct {
	Type   string
	Ref    string
	Params map[string]string
	Data   []byte
}

// RequiredArtifact describes an upload candidate resolved from container
// configuration. Source is only meaningful on the client side; the agent only
// receives the identifier and file bytes.
type RequiredArtifact struct {
	Identifier string
	Source     SourceRef
}

// collectContext bundles the inputs every artifactCollector receives. Future
// inputs (profile, environment snapshots, agent capabilities) can be added here
// without changing the collector contract.
type collectContext struct {
	Config    *Config
	ConfigDir string
}

// artifactCollector produces zero or more RequiredArtifact values for a single
// concern of the container configuration. Collectors should only resolve paths
// and build identifiers unless they produce a generated inline source; reading
// local file bytes is otherwise the upload layer's responsibility.
type artifactCollector interface {
	Kind() string
	Collect(ctx collectContext) ([]RequiredArtifact, error)
}

// defaultCollectors is the ordered registry used by CollectArtifacts. It mixes
// config-derived artifacts (Dockerfile) and CDS default singleton artifacts
// expected by the engine (auth, SSH public key). Adding a new artifact kind is a
// single append: no branch in the orchestrator.
var defaultCollectors = []artifactCollector{
	newConfigFileCollector(KindDockerfile, []string{KBuild, KBuildDockerfile}, newDockerfileReference),
	newDefaultSourceCollector(KindAuthFile, defaultAuthFileSource),
	newDefaultSourceCollector(KindPubKey, defaultAuthorizedKeysSource),
}

// CollectArtifacts returns the client-side artifacts required by the provided
// container configuration and CDS default resources. It only includes files that
// must be transferred to the agent for deployment.
func CollectArtifacts(config *Config, configDir string) ([]RequiredArtifact, error) {
	if config == nil {
		return nil, fmt.Errorf("container configuration is required")
	}

	ctx := collectContext{Config: config, ConfigDir: configDir}
	return runCollectors(ctx, defaultCollectors)
}

func runCollectors(ctx collectContext, collectors []artifactCollector) ([]RequiredArtifact, error) {
	var all []RequiredArtifact
	seen := map[string]string{}

	for _, c := range collectors {
		got, err := c.Collect(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.Kind(), err)
		}
		for _, a := range got {
			if prev, ok := seen[a.Identifier]; ok {
				return nil, fmt.Errorf("artifact identifier %q produced by both %q and %q collectors", a.Identifier, prev, c.Kind())
			}
			seen[a.Identifier] = c.Kind()
			all = append(all, a)
		}
	}

	return all, nil
}

func resolveLocalArtifactSource(configDir, source string) (SourceRef, error) {
	if filepath.IsAbs(source) {
		return SourceRef{Type: SourceTypeLocalFS, Ref: filepath.Clean(source)}, nil
	}
	if configDir == "" {
		return SourceRef{}, fmt.Errorf("config directory is required to resolve relative artifact source %q", source)
	}
	return SourceRef{
		Type: SourceTypeLocalFS,
		Ref:  filepath.Clean(filepath.Join(configDir, source)),
	}, nil
}
