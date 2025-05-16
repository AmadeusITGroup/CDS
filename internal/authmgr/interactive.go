package authmgr

import (
	"bufio"
	"fmt"
	"os"

	"golang.org/x/term"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

var (
	isInteractive bool
)

func init() {
	stdinStat, _ := os.Stdin.Stat()
	isInteractive = (stdinStat.Mode() & os.ModeCharDevice) == os.ModeCharDevice
}

func DefaultPrompt() string {
	return "Enter your office LDAP user password (Hint: windows account password):"
}

func password(message string) (string, error) {
	if isInteractive {
		return askForPasswordInteractive(message)
	}
	return askForPasswordFromStdin(message)
}

func askForPasswordFromStdin(message string) (string, error) {
	fmt.Print(message)
	return readLineFromStdin()
}

func askForPasswordInteractive(message string) (string, error) {
	fmt.Print(message)
	byteSecret, err := term.ReadPassword(int(os.Stdin.Fd()))
	// print newline to signify password was entered
	fmt.Println(cg.EmptyStr)
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("Unable to read password", err)
	}

	return string(byteSecret), nil
}

func readLineFromStdin() (string, error) {
	stdinscanner := bufio.NewScanner(os.Stdin)
	stdinscanner.Split(bufio.ScanLines)
	var line string
	if stdinscanner.Scan() {
		line = stdinscanner.Text()
	} else {
		return cg.EmptyStr, cerr.NewError("Failed to acquire a new line from stdin")
	}

	return line, nil
}
