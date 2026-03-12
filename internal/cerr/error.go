package cerr

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/amadeusitgroup/cds/internal/clog"
)

type ErrorDetail int

const (
	TopLevelOnly ErrorDetail = iota
	MessageOnly
	FullChain
)

var DefaultErrorDetail ErrorDetail = FullChain

type Err struct {
	From    string
	Message string
	Cause   *Err
}

func (e *Err) Error() string {
	return e.format(DefaultErrorDetail)
}

func Message(e error) string {
	switch e := e.(type) {
	case *Err:
		return e.format(MessageOnly)
	default:
		return e.Error()
	}
}

func (e *Err) format(detail ErrorDetail) string {
	switch detail {
	case TopLevelOnly:
		return fmt.Sprintf("%s, at [%s].", e.Message, e.From)
	case MessageOnly:
		return e.Message
	case FullChain:
		return e.bubbleUp()
	default:
		return e.bubbleUp()
	}
}

func (e *Err) bubbleUp() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s, at [%s].", e.Message, e.From)

	}
	return fmt.Sprintf("%s, at [%s]. From: %s", e.Message, e.From, e.Cause.bubbleUp())
}

func NewError(message string) error {
	return &Err{
		From:    getCodePosition(3),
		Message: message,
	}
}

func AppendError(message string, err error) error {
	if err == nil {
		clog.Error("Incorrect usage of AppendError, received a nil error !\n Occured at", getCodePosition(3))
	}

	return forwardError(message, err, 4)
}

func AppendErrorFmt(fmtMessage string, err error, elems ...any) error {
	if err == nil {
		clog.Error("Incorrect usage of AppendError, received a nil error !\n Occured at", getCodePosition(3))
	}

	return forwardError(fmt.Sprintf(fmtMessage, elems...), err, 4)
}

func AppendMultipleErrors(reportTitle string, errors []error) error {
	var errorsReport strings.Builder
	fmt.Fprintf(&errorsReport, "%s\n", reportTitle)
	for index, err := range errors {
		if err == nil {
			clog.Error("Incorrect usage of AppendMultipleErrors, received a nil error !\n Occured at", getCodePosition(3))
		}
		fmt.Fprintf(&errorsReport, "Error %v: %v \n", index, err.Error())
	}
	return NewError(errorsReport.String())
}

func getCodePosition(skipFrames int) string {
	pcs := make([]uintptr, 10)
	n := runtime.Callers(0, pcs)
	pcs = pcs[:n]
	frames := runtime.CallersFrames(pcs)

	var frame runtime.Frame
	var next bool

	for i := range n {
		frame, next = frames.Next()
		if i == int(skipFrames) {
			break
		}
		if !next {
			panic("No more frame to wind back in call stack !")
		}
	}

	return fmt.Sprintf("%s:%d(%s)", frame.File, frame.Line, frame.Function)
}

func forwardError(message string, err error, frameSkip int) error {
	comErr, isErr := err.(*Err)

	if isErr {
		return &Err{
			From:    getCodePosition(frameSkip),
			Message: message,
			Cause:   comErr,
		}
	} else {
		return &Err{
			From:    getCodePosition(frameSkip),
			Message: message,
			Cause:   fromBuiltinError(err),
		}
	}
}

func fromBuiltinError(err error) *Err {
	return &Err{
		From:    getCodePosition(2),
		Message: err.Error(),
		Cause:   nil,
	}
}
