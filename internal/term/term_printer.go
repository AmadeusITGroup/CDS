package termprint

import (
	"fmt"
	"time"

	"github.com/pterm/pterm"
)

type TermPrintStatus int

const (
	KSuccess TermPrintStatus = iota
	KWarning
	KFail
	KDefault
)

func New() *TermPrint {
	return &TermPrint{Status: KDefault}
}

type TermPrint struct {
	Status  TermPrintStatus
	spinner *pterm.SpinnerPrinter
}

func (tp *TermPrint) Printer(l any) func() {
	log, ok := l.(string)
	if !ok {
		return func() {}
	}
	if tp.spinner == nil {
		lSpinner := pterm.SpinnerPrinter{
			Sequence:            []string{"▀ ", " ▀", " ▄", "▄ "},
			Style:               &pterm.ThemeDefault.SpinnerStyle,
			Delay:               time.Millisecond * 200,
			ShowTimer:           true,
			TimerRoundingFactor: time.Second,
			TimerStyle:          &pterm.Style{pterm.FgLightRed},
			MessageStyle:        &pterm.ThemeDefault.SpinnerTextStyle,
			SuccessPrinter:      &pterm.Success,
			FailPrinter:         &pterm.Error,
			WarningPrinter:      &pterm.Warning,
		}
		tp.spinner, _ = lSpinner.Start(log)
	}
	successMessage := "Action completed !"
	warningMessage := "Something is wrong. Please check the logs."
	failMessage := "Action failed. Please check the logs."
	ongoingMessage := "Processing ..."

	if len(log) != 0 {
		successMessage = log
		warningMessage = log
		failMessage = log
		ongoingMessage = fmt.Sprintf("%s...", log)
	}

	return func() {
		switch tp.Status {
		case KSuccess:
			tp.spinner.Success(successMessage)
		case KWarning:
			tp.spinner.Warning(warningMessage)
		case KFail:
			tp.spinner.Fail(failMessage)
		case KDefault:
			tp.spinner.UpdateText(ongoingMessage)
		}
	}
}
