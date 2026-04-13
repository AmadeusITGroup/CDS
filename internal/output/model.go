package output

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

type OutputOptions struct {
	mode      Mode
	writer    io.Writer
	errWriter io.Writer // default: os.Stderr
	width     int       // terminal width, 0 if unknown
	noColor   bool
	verbose   bool
	command   string // dot-separated command path (e.g. "project.list")
}

type renderer interface {
	// HumanReadable returns the prepared string describing the data
	HumanReadable(o OutputOptions) string
	// MachineReadable returns a fully loaded object ready to be Marshalled
	MachineReadable() any
}

func renderJSON(o OutputOptions, r renderer) error {
	enc := json.NewEncoder(o.writer)
	enc.SetIndent("", "\t")
	return enc.Encode(map[string]any{"data": r.MachineReadable()})
}

func renderHuman(o OutputOptions, r renderer) error {
	result := r.HumanReadable(o)
	_, err := fmt.Fprint(o.writer, result)
	return err
}

func Render(o OutputOptions, r renderer) error {
	switch o.mode {
	case ModeQuiet:
		return nil
	case ModeJSON:
		return renderJSON(o, r)
	case ModeInteractive, ModePlain:
		return renderHuman(o, r)
	default:
		return cerr.NewError(fmt.Sprintf("invalid mode given to render: %s", o.mode))
	}
}

type outputOptionsKey struct{}

// WithOutputOptions stores an output outputOptions in a context.Context.
func WithOutputOptions(ctx context.Context, octx *OutputOptions) context.Context {
	if octx == nil {
		return ctx
	}
	return context.WithValue(ctx, outputOptionsKey{}, octx)
}

type outputTransformers func(*OutputOptions)

func WithDetect(outputFlag string, quiet bool, verbose bool) outputTransformers {
	return func(oOpts *OutputOptions) {
		oOpts.verbose = verbose
		// Detect terminal width
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
			oOpts.width = w
		}

		// Detect color capability
		oOpts.noColor = isNoColorSet() || !isColorable()

		// Determine mode (priority order)
		switch {
		case quiet:
			oOpts.mode = ModeQuiet
		case resolveOutputFlag(outputFlag) == "json":
			oOpts.mode = ModeJSON
		case resolveOutputFlag(outputFlag) == "text":
			if oOpts.noColor {
				oOpts.mode = ModePlain
			} else {
				oOpts.mode = ModeInteractive
			}
		case !isColorable():
			// Auto-detect: non-TTY → JSON
			oOpts.mode = ModeJSON
		case oOpts.noColor:
			oOpts.mode = ModePlain
		default:
			oOpts.mode = ModeUnknown
		}
	}
}

func WithCommand(cmd string) outputTransformers {
	return func(oOpts *OutputOptions) {
		oOpts.command = cmd
	}
}

func NewOutputOptions(o ...outputTransformers) (*OutputOptions, error) {
	oOpts := &OutputOptions{
		writer:    os.Stdout,
		errWriter: os.Stderr,
	}
	for _, transformer := range o {
		transformer(oOpts)
	}

	if oOpts.mode == ModeUnknown {
		return nil, cerr.NewError(fmt.Sprintf("invalid output mode detected for command '%s'", oOpts.command))
	}

	return oOpts, nil
}

// FromContext retrieves the output outputOptions from a context.Context.
// Returns a sensible default if none is set.
func FromContext(ctx context.Context) OutputOptions {
	if ctx != nil {
		switch octx := ctx.Value(outputOptionsKey{}).(type) {
		case OutputOptions:
			return octx
		case *OutputOptions:
			if octx != nil {
				return *octx
			}
		}
	}
	return OutputOptions{
		mode:      ModeInteractive,
		writer:    os.Stdout,
		errWriter: os.Stderr,
	}
}

// resolveOutputFlag checks the --output flag and CDS_OUTPUT env var.
func resolveOutputFlag(flag string) string {
	if flag != "" {
		return strings.ToLower(flag)
	}
	if env, ok := os.LookupEnv("CDS_OUTPUT"); ok {
		return strings.ToLower(env)
	}
	return "auto"
}

func isNoColorSet() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

func isColorable() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}
