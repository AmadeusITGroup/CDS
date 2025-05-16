package command

import (
	"github.com/spf13/cobra"
)

func New() *cds {
	root := &cds{}
	_ = root.command()
	buildCommand(root)
	return root
}

type baseCmd interface {
	command() *cobra.Command
	subCommands() []baseCmd
}

type defaultCmd struct {
	cmd     *cobra.Command
	subCmds []baseCmd
}

func buildCommand(bc baseCmd) {
	for _, sub := range bc.subCommands() {
		bc.command().AddCommand(sub.command())
		buildCommand(sub)
	}
}
