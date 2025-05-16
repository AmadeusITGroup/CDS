package clog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

type Loggable interface {
	Key() string
	Value() any
}

type logItem struct {
	key   string
	value any
}

func (li logItem) Key() string {
	return li.key
}

func (li logItem) Value() any {
	return li.value
}

func NewLoggable(key string, value any) Loggable {
	return &logItem{key, value}
}

var (
	logger *slog.Logger
)

func init() {
	initLogger(slog.LevelInfo)
}

func initLogger(l slog.Level) {
	th := slog.NewTextHandler(os.Stdout, nil)
	logger = slog.New(NewLevelHandler(l, th))
}

func SetLogger(l *slog.Logger) {
	logger = l
}

func Default() *slog.Logger {
	return logger
}

func Verbose() {
	initLogger(slog.LevelDebug)
}

func Debug(msg string, args ...interface{}) {
	if logger == nil {
		initLogger(slog.LevelInfo)
	}
	logger.Debug(msg, prepareArgs(args...)...)
}

func Info(msg string, args ...interface{}) {
	if logger == nil {
		initLogger(slog.LevelInfo)
	}
	logger.Info(msg, prepareArgs(args...)...)
}

func Warn(msg string, args ...interface{}) {
	if logger == nil {
		initLogger(slog.LevelInfo)
	}
	logger.Warn(msg, prepareArgs(args...)...)
}

func Error(msg string, args ...interface{}) {
	if logger == nil {
		initLogger(slog.LevelInfo)
	}
	logger.Error(msg, prepareArgs(args...)...)
}

func prepareArgs(args ...interface{}) []any {
	var attrs []any
	for i, arg := range args {
		key := generateKey(i)
		value := arg
		if l, ok := arg.(Loggable); ok {
			key = l.Key()
			value = l.Value()
		}
		switch v := value.(type) {
		case error:
			attrs = append(attrs, slog.Any("Error", v))
		case string:
			attrs = append(attrs, slog.String(key, v))
		case int:
			attrs = append(attrs, slog.Int(key, v))
		case int64:
			attrs = append(attrs, slog.Int64(key, v))
		case uint64:
			attrs = append(attrs, slog.Uint64(key, v))
		case float64:
			attrs = append(attrs, slog.Float64(key, v))
		case bool:
			attrs = append(attrs, slog.Bool(key, v))
		case time.Time:
			attrs = append(attrs, slog.Time(key, v))
		case time.Duration:
			attrs = append(attrs, slog.Duration(key, v))
		case slog.Attr:
			attrs = append(attrs, v)
		default:
			attrs = append(attrs, slog.Any(key, v))
		}
	}
	return attrs
}

func generateKey(i int) string {
	return fmt.Sprintf("arg%d", i)
}

// A levelHandler wraps a Handler with an Enabled method
// that returns false for levels below a minimum.
type levelHandler struct {
	level   slog.Leveler
	handler slog.Handler
}

// NewLevelHandler returns a LevelHandler with the given level.
// All methods except Enabled delegate to h.
func NewLevelHandler(level slog.Leveler, h slog.Handler) *levelHandler {
	// Optimization: avoid chains of LevelHandlers.
	if lh, ok := h.(*levelHandler); ok {
		h = lh.Handler()
	}
	return &levelHandler{level, h}
}

func (h *levelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *levelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.handler.Handle(ctx, r)
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewLevelHandler(h.level, h.handler.WithAttrs(attrs))
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return NewLevelHandler(h.level, h.handler.WithGroup(name))
}

func (h *levelHandler) Handler() slog.Handler {
	return h.handler
}
