package output

// Mode represents the output rendering strategy for a CLI invocation.
type Mode int

const (
	// ModeInteractive renders rich terminal output: pterm spinners, tables, colors.
	ModeInteractive Mode = iota
	// ModePlain renders human-readable text without ANSI escape codes or spinners.
	ModePlain
	// ModeJSON renders structured JSON output (auto-detected when stdout is not a TTY).
	ModeJSON
	// ModeQuiet suppresses all output; only the exit code conveys the result.
	ModeQuiet
	// ModeUnknown is used when an invalid mode is specified.
	ModeUnknown
)

func (m Mode) String() string {
	switch m {
	case ModeInteractive:
		return "interactive"
	case ModePlain:
		return "plain"
	case ModeJSON:
		return "json"
	case ModeQuiet:
		return "quiet"
	case ModeUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}
