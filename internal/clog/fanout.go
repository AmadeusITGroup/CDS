package clog

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	cg "github.com/amadeusitgroup/cds/internal/global"
)

// makes sure FanoutHandler implements slog.Handler
var _ slog.Handler = (*fanoutHandler)(nil)

type fanoutHandler struct {
	handlers []slog.Handler
}

// NewFanoutHandler distributes records to multiple slog.Handler sequentially.
func NewFanoutHandler(handlers ...slog.Handler) slog.Handler {
	return &fanoutHandler{
		handlers: handlers,
	}
}

func (fh *fanoutHandler) Enabled(ctx context.Context, l slog.Level) bool {
	for i := range fh.handlers {
		if fh.handlers[i].Enabled(ctx, l) {
			return true
		}
	}

	return false
}

func (fh *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for i := range fh.handlers {
		if fh.handlers[i].Enabled(ctx, r.Level) {
			err := fh.handlers[i].Handle(ctx, r.Clone())

			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

func (fh *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := Map(fh.handlers, func(h slog.Handler) slog.Handler {
		return h.WithAttrs(slices.Clone(attrs))
	})
	return NewFanoutHandler(handlers...)
}

func (fh *fanoutHandler) WithGroup(name string) slog.Handler {
	if name == cg.EmptyStr {
		return fh
	}

	handlers := Map(fh.handlers, func(h slog.Handler) slog.Handler {
		return h.WithGroup(name)
	})
	return NewFanoutHandler(handlers...)
}

// TODO: move to future equivalent of com package
func Map[S ~[]T, T, U any](s S, f func(T) U) []U {
	var resultList []U
	for _, x := range s {
		resultList = append(resultList, f(x))
	}
	return resultList
}
