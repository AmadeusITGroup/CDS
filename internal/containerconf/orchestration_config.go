package containerconf

import (
	"fmt"

	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

// KindOrchestrationConfig is the orchestration configuration the agent collects
// from a devcontainer configuration to deploy (and decorate a container with) a
// kind cluster and its optional secure registry.
//
// It is a flat snapshot of the `orchestration` section of devcontainer.json,
// resolved once from the request-scoped Config so the deploy path does not
// scatter Get/IsSet lookups. Zero values mean "not requested": an unset
// orchestration section yields Requested == false and the deploy path treats
// the container as a plain devcontainer.
type KindOrchestrationConfig struct {
	// Requested reports whether the devcontainer asked for an orchestration
	// engine at all (the `orchestration` section is a JSON object; false or
	// null requests nothing, CDS-073).
	Requested bool
	// Engine is the requested orchestration engine name, e.g. "kind". Empty
	// when the section is present but no engine is named.
	Engine string
	// ConfigFile is an optional path to an engine-specific configuration file
	// (e.g. a kind cluster config). Empty when not provided.
	ConfigFile string
	// PersistentVolume reports whether a persistent volume claim mount was
	// requested (orchestration.pvc).
	PersistentVolume bool
	// Ingress reports whether an nginx ingress controller should be installed
	// on the cluster (orchestration.ingress).
	Ingress bool
	// Registry holds the secure-registry configuration requested alongside the
	// cluster.
	Registry RegistryConfig
	// Mirrors are registry pull-through mirrors to install on the cluster nodes
	// (containerd hosts.toml), so image pulls for the listed registries go
	// through the given upstream endpoint. Empty when none are configured.
	Mirrors []Mirror
}

// Mirror is a single registry pull-through mirror: pulls for Registry (e.g.
// "docker.io") are served from Endpoint (an upstream mirror URL).
type Mirror struct {
	Registry string
	Endpoint string
}

// RegistryConfig is the registry part of an orchestration configuration.
type RegistryConfig struct {
	// Requested reports whether a registry was requested
	// (orchestration.registry is a JSON object, CDS-073).
	Requested bool
	// InsecureRequested reports whether orchestration.registry.secure is set
	// to anything but true. The registry is always secured, so such a request
	// is rejected; an absent key is fine (CDS-034).
	InsecureRequested bool
}

// kOrchestrationRegistryIgnoredPort is the former orchestration.registry.port
// key. The registry always listens on registry.DefaultPort (one registry per
// host, reached in-network on its container port), so the key is ignored and
// only detected to warn about it (CDS-033).
const kOrchestrationRegistryIgnoredPort = "port"

// RegistryPortIgnored reports whether the configuration still sets the removed
// orchestration.registry.port key, which has no effect (CDS-033).
func (c *Config) RegistryPortIgnored() bool {
	return c.IsSet(KOrchestration, KOrchestrationRegistry, kOrchestrationRegistryIgnoredPort)
}

// OrchestrationConfig collects the orchestration configuration from the
// devcontainer configuration. It never returns nil: when the orchestration
// section is absent, the returned config has Requested == false and zero
// values everywhere else.
func (c *Config) OrchestrationConfig() KindOrchestrationConfig {
	cfg := KindOrchestrationConfig{
		Requested:        c.isObject(KOrchestration),
		ConfigFile:       c.GetOrchestrationConfigFilePath(),
		PersistentVolume: c.boolAt(KOrchestration, KPersistentVolumeClaim),
		Ingress:          c.boolAt(KOrchestration, KOrchestrationIngress),
		Registry:         c.registryConfig(),
		Mirrors:          c.mirrors(),
	}
	if engine, ok := c.Get(KOrchestration, KOrchestrationEngine).(string); ok {
		cfg.Engine = engine
	}
	return cfg
}

// ValidateOrchestrationSections rejects an orchestration or
// orchestration.registry section that is set but not a JSON object (e.g.
// false): such a section requests nothing, so the user is told to remove it
// rather than having it silently ignored. null counts as absent (CDS-073).
func (c *Config) ValidateOrchestrationSections() error {
	for _, key := range [][]string{{KOrchestration}, {KOrchestration, KOrchestrationRegistry}} {
		if v := c.Get(key...); v != nil && !c.isObject(key...) {
			name := cg.VariadicJoin(".", key...)
			return cerr.NewError(fmt.Sprintf("%s must be an object, got %T (%v); remove it to turn it off", name, v, v))
		}
	}
	return nil
}

// registryConfig collects the registry part of the orchestration configuration.
func (c *Config) registryConfig() RegistryConfig {
	return RegistryConfig{
		Requested: c.IsRegistryRequested(),
		InsecureRequested: c.IsSet(KOrchestration, KOrchestrationRegistry, KOrchestrationRegistrySecure) &&
			!c.boolAt(KOrchestration, KOrchestrationRegistry, KOrchestrationRegistrySecure),
	}
}

// mirrors collects the configured registry pull-through mirrors. Each entry is
// an object with a registry and an endpoint; malformed entries are skipped.
func (c *Config) mirrors() []Mirror {
	raw, ok := c.Get(KOrchestration, KOrchestrationMirrors).([]interface{})
	if !ok {
		return nil
	}
	var mirrors []Mirror
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		registry, _ := m[KOrchestrationMirrorRegistry].(string)
		endpoint, _ := m[KOrchestrationMirrorEndpoint].(string)
		if registry == "" || endpoint == "" {
			continue
		}
		mirrors = append(mirrors, Mirror{Registry: registry, Endpoint: endpoint})
	}
	return mirrors
}

// boolAt returns the bool at the given key path, or false when unset or not a
// bool.
func (c *Config) boolAt(key ...string) bool {
	v, ok := c.Get(key...).(bool)
	return ok && v
}
