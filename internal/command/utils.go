package command

import (
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/spf13/cobra"
)

// TODO: FixMe
func completionFlavour(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// flavours, err := container.ListFlavours()
	flavours, err := []string{}, cerr.NewError("")
	if err != nil {
		clog.Error("Failed to fetch flavour list for completion !")
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return flavours, cobra.ShellCompDirectiveNoFileComp
}
