package clog

import (
	"os"

	"golang.org/x/sys/windows"
)

// Modifies stdout to enable the usage of ansi escape sequences
// https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences#output-sequences
// -> We need ENABLE_VIRTUAL_TERMINAL_PROCESSING
// From documentation https://learn.microsoft.com/en-us/windows/console/setconsolemode
// "ENABLE_PROCESSED_OUTPUT [...] It should be enabled [...] when ENABLE_VIRTUAL_TERMINAL_PROCESSING is set."
func init() {
	var outMode uint32
	out := windows.Handle(os.Stdout.Fd())
	if err := windows.GetConsoleMode(out, &outMode); err != nil {
		return
	}
	outMode |= windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	_ = windows.SetConsoleMode(out, outMode)
}
