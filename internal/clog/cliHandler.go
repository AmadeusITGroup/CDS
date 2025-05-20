package clog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	kLevelWidth              = 9
	kMaxWidthLevelIdentifier = 7
)

type CustomHandlerOption struct {
	Level slog.Leveler

	TimeFormat string

	AddSource bool

	ReplaceAttr func([]string, slog.Attr) slog.Attr

	NoColor bool
}

type customHandler struct {
	opts *CustomHandlerOption
	out  io.Writer
	goas []groupOrAttrs
	sync.Mutex
}

type groupOrAttrs struct {
	group string      // group name if non-empty
	attrs []slog.Attr // attrs if non-empty
}

type cliHandler struct {
	*customHandler
}

var _ slog.Handler = (*cliHandler)(nil)

func NewCliHandler(out io.Writer, options ...func(*CustomHandlerOption)) *cliHandler {
	opts := &CustomHandlerOption{
		Level:      slog.LevelInfo,
		TimeFormat: time.DateTime,
	}
	for _, f := range options {
		f(opts)
	}
	h := &cliHandler{customHandler: &customHandler{out: out, opts: opts}}
	return h
}

func WithLevel(l slog.Level) func(*CustomHandlerOption) {
	return func(o *CustomHandlerOption) {
		o.Level = l
	}
}

func WithTimeFormat(format string) func(*CustomHandlerOption) {
	return func(o *CustomHandlerOption) {
		o.TimeFormat = format
	}
}

// WithAddSource sets the AddSource option for the custom handler.
func WithAddSource(addSource bool) func(*CustomHandlerOption) {
	return func(o *CustomHandlerOption) {
		o.AddSource = addSource
	}
}

// WithReplaceAttr sets the ReplaceAttr function for the custom handler.
func WithReplaceAttr(replaceAttr func([]string, slog.Attr) slog.Attr) func(*CustomHandlerOption) {
	return func(o *CustomHandlerOption) {
		o.ReplaceAttr = replaceAttr
	}
}

// WithNoColor sets the NoColor option for the custom handler.
func WithNoColor(noColor bool) func(*CustomHandlerOption) {
	return func(o *CustomHandlerOption) {
		o.NoColor = noColor
	}
}

// Enabled implements slog.Handler.
func (c *cliHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= c.opts.Level.Level()
}

// Handle is the main function that will build the buffer which will be written to the out writer at the end
// There is a lot of noise in this function as there are a lot of option to deal with
// replaceAttr is the noisiest one it is used to modify all Attr that are present in the record so if you see it you need to apply replaceAttr *if* given
// schedule is time -> source -> level -> goas (groups and attrs)
func (c *cliHandler) Handle(ctx context.Context, r slog.Record) error {
	buf := newBuffer()
	defer buf.Free()
	replaceAttr := c.opts.ReplaceAttr
	// write time
	if !r.Time.IsZero() {
		val := r.Time.Round(0)
		if replaceAttr == nil {
			c.appendTime(buf, val)
		} else if a := replaceAttr(nil, slog.Time(slog.TimeKey, val)); a.Key != cg.EmptyStr {
			if a.Value.Kind() == slog.KindTime {
				c.appendTime(buf, a.Value.Time())
			} else {
				c.appendAttr(buf, a, 0)
			}
		}
		_ = buf.WriteByte(cg.SpaceRune)
	}
	// write source
	if c.opts.AddSource && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != cg.EmptyStr {
			src := slog.Source{
				Function: f.Function,
				File:     f.File,
				Line:     f.Line,
			}

			if replaceAttr == nil {
				c.appendSource(buf, src)
				_ = buf.WriteByte(cg.SpaceRune)
			} else if a := replaceAttr(nil, slog.Any(slog.SourceKey, src)); a.Key != cg.EmptyStr {
				c.appendAttr(buf, a, 0)
				_ = buf.WriteByte(cg.SpaceRune)
			}
		}

	}

	// write level
	c.appendLevel(buf, r.Level)
	_ = buf.WriteByte(cg.SpaceRune)

	alignCharsTotal := bufferSizeWithoutColors(buf)

	c.addFgColorForLevel(buf, r.Level)
	if replaceAttr == nil {
		_, _ = buf.WriteString(r.Message)
		_ = buf.WriteByte(cg.SpaceRune)
	} else if a := replaceAttr(nil, slog.String(slog.MessageKey, r.Message)); a.Key != cg.EmptyStr {
		c.appendAttr(buf, a, 0)
	}
	// Handle state from WithGroup and WithAttrs.
	goas := c.goas
	if r.NumAttrs() == 0 {
		// If the record has no Attrs, remove groups at the end of the list; they are empty.
		for len(goas) > 0 && goas[len(goas)-1].group != cg.EmptyStr {
			goas = goas[:len(goas)-1]
		}
	}
	indentLevel := 0
	for _, goa := range goas {
		indentSpaces := alignCharsTotal + indentLevel*4
		if goa.group != cg.EmptyStr {
			_, _ = buf.WriteStringf("%*s%s:\n", indentSpaces, cg.EmptyStr, goa.group)
			indentLevel++
		} else {
			for _, a := range goa.attrs {
				c.appendAttr(buf, a, indentSpaces)
			}
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		c.appendAttr(buf, a, alignCharsTotal+indentLevel*4)
		return true
	})
	if !c.opts.NoColor {
		_, _ = buf.WriteString(ansiCodeReset)
	}
	c.Lock()
	defer c.Unlock()
	_, err := c.out.Write(*buf)
	return err
}

func (c *cliHandler) WithGroup(name string) slog.Handler {
	if name == cg.EmptyStr {
		return c
	}
	return c.withGroupOrAttrs(groupOrAttrs{group: name})
}

func (c *cliHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return c
	}
	return c.withGroupOrAttrs(groupOrAttrs{attrs: attrs})
}

func (c *cliHandler) withGroupOrAttrs(goa groupOrAttrs) *cliHandler {
	c2 := *c
	c2.goas = slices.Clone(c.goas)
	c2.goas = append(c2.goas, goa)
	return &c2
}

func (c *cliHandler) appendAttr(buf *buffer, a slog.Attr, indentSpaceTotal int) {
	// Values need to be resolved before usage
	a.Value = a.Value.Resolve()

	if replaceAttr := c.opts.ReplaceAttr; replaceAttr != nil && a.Value.Kind() != slog.KindGroup {
		a = replaceAttr(getGroupsFromKey(a.Key), a)
		a.Value = a.Value.Resolve()
	}

	if a.Equal(slog.Attr{}) {
		return
	}

	indentsTotal := indentSpaceTotal
	_, _ = buf.WriteStringf("%*s", indentsTotal, cg.EmptyStr)
	switch a.Value.Kind() {
	case slog.KindString:
		if a.Key == slog.MessageKey {
			_, _ = buf.WriteStringf("%s\n", a.Value.String())
		} else {
			_, _ = buf.WriteStringf("%s: %q\n", a.Key, a.Value)
		}
	case slog.KindTime:
		_, _ = buf.WriteStringf("%s", a.Value.Time().Format(c.opts.TimeFormat))
	case slog.KindGroup:
		attrs := a.Value.Group()
		if len(attrs) == 0 {
			return
		}
		if a.Key != cg.EmptyStr {
			_, _ = buf.WriteStringf("%s:\n", a.Key)
			indentsTotal += 4
		}
		for _, ga := range attrs {
			c.appendAttr(buf, ga, indentsTotal)
		}
	default:
		_, _ = buf.WriteStringf("%s: %s\n", a.Key, a.Value)
	}
}

func (c *cliHandler) appendSource(buf *buffer, src slog.Source) {
	if !c.opts.NoColor {
		_, _ = buf.WriteString(ansiCodeFaint)
	}
	dir, file := filepath.Split(src.File)
	_ = buf.WriteByte('(')
	_, _ = buf.WriteString(filepath.Join(filepath.Base(dir), file))
	_ = buf.WriteByte(':')
	_, _ = buf.WriteString(strconv.Itoa(src.Line))
	_ = buf.WriteByte(')')
	if !c.opts.NoColor {
		_, _ = buf.WriteString(ansiCodeReset)
	}
}

func (c *cliHandler) appendTime(buf *buffer, t time.Time) {
	if !c.opts.NoColor {
		_, _ = buf.WriteString(ansiCodeFaint)
	}
	*buf = t.AppendFormat(*buf, c.opts.TimeFormat)
	if !c.opts.NoColor {
		_, _ = buf.WriteString(ansiCodeReset)
	}
}

func (c *cliHandler) appendLevel(buf *buffer, l slog.Level) {
	var color string
	switch l {
	case slog.LevelError:
		color = ansiCodeBgBrightRed
	case slog.LevelWarn:
		color = ansiCodeBgBrightYellow
	case slog.LevelInfo:
		color = ansiCodeBgBrightCyan
	case slog.LevelDebug:
	}
	levelIdentifier := l.String()
	if len(levelIdentifier) > kMaxWidthLevelIdentifier {
		levelIdentifier = levelIdentifier[:kMaxWidthLevelIdentifier]
	}
	s := centerText(levelIdentifier, kLevelWidth)
	if !c.opts.NoColor {
		s = fmt.Sprintf("%s%s%s", color, s, ansiCodeReset)
	}
	_, _ = buf.WriteString(s)
}

// centerText centers the given text within the specified width.
func centerText(text string, width int) string {
	if len(text) >= width {
		return text[:width]
	}
	spacesTotal := width - len(text)
	leftSpaces := spacesTotal / 2
	rightSpaces := spacesTotal - leftSpaces
	return strings.Repeat(" ", leftSpaces) + text + strings.Repeat(" ", rightSpaces)
}

func (c *cliHandler) addFgColorForLevel(buf *buffer, l slog.Level) {
	var color string
	switch l {
	case slog.LevelError:
		color = ansiCodeFgBrightRed
	case slog.LevelWarn:
		color = ansiCodeFgBrightYellow
	case slog.LevelInfo:
		color = ansiCodeFgBrightCyan
	case slog.LevelDebug:
	}
	if !c.opts.NoColor {
		_, _ = buf.WriteString(color)
	}
}

func getGroupsFromKey(key string) []string {
	keys := strings.Split(key, ".")
	if len(keys) < 2 {
		return nil
	}
	return keys[:len(keys)-1]
}

func bufferSizeWithoutColors(buf *buffer) int {
	return len(stripAnsiCodes(*buf))
}

func stripAnsiCodes(b []byte) []byte {
	var out []byte
	ansiCodeEscape := byte(27) // \x1b -> 0x1b=27
	ansiCodeEnd := byte('m')
	i := 0
	for i < len(b) {
		if b[i] == ansiCodeEscape {
			for i < len(b) && b[i] != ansiCodeEnd {
				i++
			}
			if i < len(b) {
				i++
			}
		} else {
			out = append(out, b[i])
			i++
		}
	}
	return out
}
