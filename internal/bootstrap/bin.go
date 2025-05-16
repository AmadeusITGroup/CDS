package bootstrap

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

func ensureBinary(bin binary) error {
	name := bin.name()
	path, err := exec.LookPath(name)

	if err == nil {
		clog.Debug(fmt.Sprintf("%s binary is available at %s", name, path))
		return nil //
	}

	notFound := errors.Is(err, exec.ErrNotFound)
	if notFound {
		return bin.install()
	}
	return cerr.NewError(fmt.Sprintf("Neither found nor managed to install %s binary", name))
}

type binary interface {
	name() string
	install() error
}

type cfsslbin struct {
	n string
}

func (c cfsslbin) name() string {
	return c.n
}

func (c cfsslbin) install() error {
	clog.Warn("Not implemented yet")
	return nil
}

type cfssljsonbin struct {
	n string
}

func (c cfssljsonbin) name() string {
	return c.n
}

func (c cfssljsonbin) install() error {
	clog.Warn("Not implemented yet")
	return nil
}

type cdsagentbin struct {
	n string
}

func (c cdsagentbin) name() string {
	return c.n
}

func (c cdsagentbin) install() error {
	clog.Warn("Not implemented yet")
	return nil
}
