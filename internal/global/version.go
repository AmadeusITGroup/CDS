package cg

// Version is the version of the cds binaries (CLI and agent), shared so that
// the CLI can check that an agent runs the same version. It also namespaces the
// agent binary cache (~/.xcds/cache/agents/<version>) and the remote install
// path (~/.xcds-agent/bin/<version>), so two different releases must report two
// different versions for upgrades to be detected.
//
// Set at build time, e.g.
//
//	go build -ldflags="-X github.com/amadeusitgroup/cds/internal/global.Version=x.y.z"
//
// See the build targets in makefile and makefile.delivery.
var (
	Version    = "develop"
	BuildTime  = "unknown"
	CommitHash = "unknown"
)
