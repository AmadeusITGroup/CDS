package cerr

import (
	"fmt"
	"log/slog"
)

// Replaces an error if given through an slog any value
func ReplaceAttrErr(groups []string, a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	// other cases

	case slog.KindAny:
		switch v := a.Value.Any().(type) {
		case error:
			a.Value = fmtErr(v)
		}
	}

	return a
}

// Given an error return a slog.Value that will *Group* both the root cause and a *Group* representing the trace
// The trace is generated using our custom Err object to explore the call stack Err, Err.Cause, Err.Cause.Cause etc
// The root Cause
func fmtErr(err error) slog.Value {
	var groupValues []slog.Attr

	var topErr *Err
	if cErr, isCerr := err.(*Err); isCerr {
		topErr = cErr
	} else {
		topErr = fromBuiltinError(err)
	}
	bottomErr := topErr.root()
	groupValues = append(groupValues, slog.String("Root cause", bottomErr.String()))

	var args []slog.Attr
	for i, line := range traceLines(topErr) {
		attr := slog.String(fmt.Sprintf("D%d", i), line)
		args = append(args, attr)
	}
	traceGroup := slog.GroupValue(args...)
	groupValues = append(groupValues, slog.Any("Trace", traceGroup))

	return slog.GroupValue(groupValues...)
}

func traceLines(err *Err) []string {
	var lines []string

	for {
		line := err.String()
		lines = append(lines, line)
		if err.Cause == nil {
			break
		}
		err = err.Cause
	}

	return lines
}

func (err *Err) root() *Err {
	node := err
	for node.Cause != nil {
		node = node.Cause
	}
	return node
}

func (err Err) String() string {
	return fmt.Sprintf("%s at %s", err.Message, err.From)
}
