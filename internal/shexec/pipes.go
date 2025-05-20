package shexec

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

type Execcmd struct {
	Name string
	Args []string
}

type Pipe struct {
	Left, Right Execcmd
}

func ExecuteCmd(ec Execcmd, workDir string) (string, error) {
	c := append([]string{ec.Name}, ec.Args...)
	name := c[0]
	args := c[1:]
	cmd := exec.Command(name, args...)
	cmd.Dir = workDir

	clog.Debug(fmt.Sprintf("shell command: (%s)", cmd.String()))

	stdout, err := cmd.CombinedOutput()
	return string(stdout), err
}

func ExecutePipe(p Pipe, workDir string) (string, error) {
	c1 := append([]string{p.Left.Name}, p.Left.Args...)
	c2 := append([]string{p.Right.Name}, p.Right.Args...)

	cmd1 := exec.Command(c1[0], c1[1:]...)
	cmd1.Dir = workDir

	clog.Debug(fmt.Sprintf("shell command: (%s)", cmd1.String()))

	stdout, errStdoutPipe := cmd1.StdoutPipe()
	if errStdoutPipe != nil {
		return cg.EmptyStr, cerr.AppendError(fmt.Sprintf("Error setting stdout pipe for command (%s)", cmd1.String()), errStdoutPipe)
	}
	if err := cmd1.Start(); err != nil {
		return cg.EmptyStr, cerr.AppendError(fmt.Sprintf("Error to start command (%s)", cmd1.String()), err)
	}

	buf := new(strings.Builder)
	var errBuf error
	_, errBuf = io.Copy(buf, stdout)
	if errBuf != nil {
		clog.Error(fmt.Sprintf("Error copying from stdout to buf: %v", errBuf))
	}

	if err := cmd1.Wait(); err != nil {
		clog.Debug(fmt.Sprintf("Error waiting for command (%s)", c1))
		return cg.EmptyStr, cerr.AppendError(fmt.Sprintf("Error waiting for command (%s)", cmd1.String()), err)
	}

	cmd2 := exec.Command(c2[0], c2[1:]...)
	cmd2.Dir = workDir
	clog.Debug(fmt.Sprintf("shell command: (%s)", cmd2.String()))
	stdin, err := cmd2.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		defer func() {
			_ = stdin.Close()
		}()
		_, err = io.WriteString(stdin, buf.String())
		if err != nil {
			clog.Error(fmt.Sprintf("Error writing to stdin: %v", err))
		}
	}()

	out, err := cmd2.Output()
	if err != nil {
		return cg.EmptyStr, cerr.AppendError("Pipe failed:", err)
	}
	return string(out), nil
}
