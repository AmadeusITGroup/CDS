package shexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

func RunLocalCmds(cmds []ExecuteEvent) error {
	shellToUse := "bash"
	for _, cmd := range cmds {
		stdout := bytes.Buffer{}
		stderr := bytes.Buffer{}

		exeCmd := exec.Command(shellToUse, "-c", cmd.Cmd())
		exeCmd.Stdout = &stdout
		exeCmd.Stderr = &stderr

		if err := exeCmd.Run(); err != nil {
			return cerr.AppendError(
				fmt.Sprintf("Failed to execute commands locally, command (%v) failed\nstdout: %s\nstderr: %s",
					cmd.Cmd(),
					stdout.String(),
					stderr.String(),
				),
				err)
		}
	}
	return nil
}

// run single command and return its output
// if the command fails, error is captured and (stderr, exe.Run's error) are returned
func RunLocalCmdWithOutput(cmd []ExecuteEvent) (string, error) {
	var shell string
	var args []string

	switch runtime.GOOS {
	case "windows":
		shell = "cmd.exe"
		args = []string{"/c"}
	default:
		shell = "bash"
		args = []string{"-c"}
	}

	args = append(args, cmd[0].Cmd())

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exeCmd := exec.Command(shell, args...)
	exeCmd.Stdout = &stdout
	exeCmd.Stderr = &stderr

	if err := exeCmd.Run(); err != nil {
		return stderr.String(), cerr.AppendError(
			fmt.Sprintf("Failed to execute commands locally, command (%v) failed\nstdout: %s\nstderr: %s",
				cmd[0].Cmd(),
				stdout.String(),
				stderr.String(),
			),
			err)
	}

	return stdout.String(), nil
}

// attach the terminal to a ssh session, the assumption here is that
// cds is started with a TTY !
func AttachShellUsingKey(h target) error {
	client, connectErr := connectSSHKey(h)

	if connectErr != nil {
		return connectErr
	}

	session, cleanup, errOpenSession := openAttachedSession(client)
	defer cleanup()

	if errOpenSession != nil {
		return errOpenSession
	}

	if shellErr := session.Shell(); shellErr != nil {
		return cerr.AppendError("Failed to attach to ssh session", shellErr)
	}

	ctxWindowHandler, cancelWindowHandler := context.WithCancel(context.Background())

	errChan := windowSizeChangeHandler(ctxWindowHandler, session)

	waitErr := session.Wait()
	cancelWindowHandler()

	for errWinChan := range errChan {
		if errWinChan != nil {
			clog.Debug("An error occurred in the window resize handler", errWinChan)
		}
	}

	if waitErr != nil {
		if endSessionErr, ok := waitErr.(*ssh.ExitError); ok {
			return cerr.AppendError("SSH session exited ", endSessionErr)
		}

		return cerr.AppendError("SSH session closed", waitErr)
	}

	return nil
}

// open a ssh session from the client and bind the terminal to the session using session pty
// return a function to cleanup as is delegates resources closure !
func openAttachedSession(client *ssh.Client) (*ssh.Session, func(), error) {
	var cleanup func()
	session, err := client.NewSession()
	cleanup = func() {
		if session != nil {
			_ = session.Close()
		}
	}

	if err != nil {
		return nil, cleanup, cerr.AppendError("Failed to create session", err)
	}

	fd := int(os.Stdin.Fd())
	state, rawErr := term.MakeRaw(fd)
	cleanup = func() {
		if state != nil {
			if err := term.Restore(fd, state); err != nil {
				clog.Error("Failed to restore the teminal", err)
			}
		}

		if session != nil {
			_ = session.Close()
		}
	}

	if rawErr != nil {
		return nil, cleanup, cerr.AppendError("Failed to set current terminal in raw mode", rawErr)
	}

	width, height, termErr := getTermSize()
	if termErr != nil {
		return nil, cleanup, cerr.AppendError("Failed to get terminal dimensions", termErr)
	}
	userTerm := os.Getenv("TERM")
	if userTerm == "" {
		userTerm = "xterm-256color"
	}

	termModes := ssh.TerminalModes{
		ssh.ECHO: 1,
		// TODO:FixMe: add UTF8, not available right now
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	// TODO: fix com.Trace
	// com.Trace(fmt.Sprintf("terminal size found: width %d, height %d", width, height))
	if ptyErr := session.RequestPty(userTerm, height, width, termModes); ptyErr != nil {
		return nil, cleanup, cerr.AppendError("Failed to request PTY", ptyErr)
	}

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	return session, cleanup, nil
}

func getTermSize() (int, int, error) {
	// https://github.com/golang/go/issues/20388 -> this is what being in hell must feel like
	var getSizeFd int
	switch runtime.GOOS {
	case "windows":
		getSizeFd = int(os.Stdout.Fd())
	default:
		getSizeFd = int(os.Stdin.Fd())
	}

	width, height, termErr := term.GetSize(getSizeFd)
	if termErr != nil {
		return width, height, cerr.AppendError("Failed to get terminal dimensions", termErr)
	}

	return width, height, nil
}

// attach the terminal to a ssh session, the assumption here is that
// cds is started with a TTY !
func AttachProcessUsingKey(h target, cmd ExecuteEvent) error {
	client, connectErr := connectSSHKey(h)

	if connectErr != nil {
		return connectErr
	}

	session, cleanup, errOpenSession := openAttachedSession(client)
	defer cleanup()

	if errOpenSession != nil {
		return errOpenSession
	}

	ctxWindowHandler, cancelWindowHandler := context.WithCancel(context.Background())

	errChan := windowSizeChangeHandler(ctxWindowHandler, session)

	if shellErr := session.Run(cmd.Cmd()); shellErr != nil {
		cancelWindowHandler()
		return cerr.AppendError("Failed to attach to ssh session", shellErr)
	}

	cancelWindowHandler()

	for errWinChan := range errChan {
		if errWinChan != nil {
			clog.Debug("An error occurred in the window resize handler", errWinChan)
		}
	}

	return nil
}

// TODO: Fix/refactor com.Trace
func windowSizeChangeHandler(ctx context.Context, session *ssh.Session) <-chan error {
	errChan := make(chan error)
	go func(ctx context.Context, errChannel chan<- error) {
		defer close(errChannel)
		ticker := time.NewTicker(time.Second)
		// defer com.Trace("window size handler goroutine exits")
		width, height, termErr := getTermSize()
		if termErr != nil {
			errChannel <- cerr.AppendError("Failed to get terminal dimensions", termErr)
			return
		}

		for {
			select {
			case <-ticker.C:
				w, h, termErr := getTermSize()
				if termErr != nil {
					errChannel <- cerr.AppendError("Failed to get terminal dimensions", termErr)
					return
				}
				if w != width || h != height {
					width, height = w, h
					clog.Debug(fmt.Sprintf("Window size change detected, new width %d, new height %d", width, height))
					if errWinCh := session.WindowChange(height, width); errWinCh != nil {
						errChannel <- cerr.AppendError("Failed to notify change of window size to pty", errWinCh)
						return
					}
				}

			case <-ctx.Done():
				// com.Trace("window size handler received done")
				errChannel <- nil
				return
			}
		}
	}(ctx, errChan)

	return errChan
}

func DryRun(actions []string) error {
	// TODO:Feature:
	return nil
}

type ExecuteEvent interface {
	Cmd() string
	Description() string
	Recoverable() bool
	CarryOn() bool
	Recover() string
	// Notify(string, event.NotifType) event.Notification
}

type DefaultShEvent struct {
	ExeCmd         string
	DescriptionCmd string
	Host           target
}

func (se *DefaultShEvent) Cmd() string {
	return se.ExeCmd
}

func (se *DefaultShEvent) Description() string {
	return se.DescriptionCmd
}

func (se *DefaultShEvent) Recover() string {
	return ""
}

func (se *DefaultShEvent) Recoverable() bool {
	return false
}

func (se *DefaultShEvent) CarryOn() bool {
	return true
}

func (se *DefaultShEvent) Fallback() error {
	return nil
}

// func (se *DefaultShEvent) Notify(m string, t event.NotifType) event.Notification {
// 	sn := &event.DefaultNotification{
// 		NotifName:    "",
// 		NotifLevel:   event.KNotifLvlShell,
// 		NotifType:    t,
// 		NotifMessage: m,
// 	}
// 	return sn
// }

var _ ExecuteEvent = (*CopyFileEvent)(nil)

type CopyFileEvent struct {
	FileName string
	DefaultShEvent
}

// func (cpe *CopyFileEvent) Notify(m string, t event.NotifType) event.Notification {
// 	cn := &event.DefaultNotification{
// 		NotifName:    "remote-copy",
// 		NotifLevel:   event.KNotifLvlShell,
// 		NotifType:    t,
// 		NotifMessage: fmt.Sprintf(`%v: '%v'`, m, cpe.FileName),
// 	}
// 	return cn
// }
