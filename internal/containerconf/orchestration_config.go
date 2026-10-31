package containerconf

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
	// engine at all (the `orchestration` section is present).
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
	// Registry holds the secure-registry configuration requested alongside the
	// cluster.
	Registry RegistryConfig
}

// RegistryConfig is the registry part of an orchestration configuration.
type RegistryConfig struct {
	// Requested reports whether a registry was requested
	// (orchestration.registry is present).
	Requested bool
	// Secure reports whether the requested registry must be secured
	// (orchestration.registry.secure).
	Secure bool
	// Port is the host port the registry publishes, or 0 when unset (the agent
	// then picks/keeps a port).
	Port int
}

// OrchestrationConfig collects the orchestration configuration from the
// devcontainer configuration. It never returns nil: when the orchestration
// section is absent, the returned config has Requested == false and zero
// values everywhere else.
func (c *Config) OrchestrationConfig() KindOrchestrationConfig {
	cfg := KindOrchestrationConfig{
		Requested:        c.IsSet(KOrchestration),
		ConfigFile:       c.GetOrchestrationConfigFilePath(),
		PersistentVolume: c.boolAt(KOrchestration, KPersistentVolumeClaim),
		Registry:         c.registryConfig(),
	}
	if engine, ok := c.Get(KOrchestration, KOrchestrationEngine).(string); ok {
		cfg.Engine = engine
	}
	return cfg
}

// registryConfig collects the registry part of the orchestration configuration.
func (c *Config) registryConfig() RegistryConfig {
	return RegistryConfig{
		Requested: c.IsRegistryRequested(),
		Secure:    c.boolAt(KOrchestration, KOrchestrationRegistry, KOrchestrationRegistrySecure),
		Port:      c.intAt(KOrchestration, KOrchestrationRegistry, KOrchestrationRegistryPort),
	}
}

// boolAt returns the bool at the given key path, or false when unset or not a
// bool.
func (c *Config) boolAt(key ...string) bool {
	v, ok := c.Get(key...).(bool)
	return ok && v
}

// intAt returns the int at the given key path, or 0 when unset or not an int.
// JSON numbers decode as float64 through viper, so that shape is accepted too.
func (c *Config) intAt(key ...string) int {
	switch v := c.Get(key...).(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
