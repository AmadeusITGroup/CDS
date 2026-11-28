package registry

// New returns the registry provider for the agent host. It mirrors
// orchestration.Provider: a single implementation today, behind a seam so the
// service layer stays provider-agnostic.
func New() Provider {
	return NewProvider()
}
