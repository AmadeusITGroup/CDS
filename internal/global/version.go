package cg

// Version is the version of the cds binaries (CLI and agent), shared so that
// the CLI can check that an agent runs the same version.
// Set at build time: go build -ldflags="-X github.com/amadeusitgroup/cds/internal/global.Version=x.y.z"
var Version = "develop"
