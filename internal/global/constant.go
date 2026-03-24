package cg

import "io/fs"

const (
	EmptyStr       = ""
	SpaceRune      = ' '
	KLogTimeFormat = "Jan 02 06 15:04"
)

const (
	WINDOWS = "windows"
	DARWIN  = "darwin"
	LINUX   = "linux"
)

const (
	KPermFile      = fs.FileMode(0600)
	KPermDir       = fs.FileMode(0700)
	KPermExec      = fs.FileMode(0700)
	KOctalBase     = 8
	KInteger32Bits = 32
)

const (
	KLocalhost = "localhost"
)

const (
	KOrchestrationDefaultNamespace = "default"
	KContainerAuthFileName         = "auth.json"
)
