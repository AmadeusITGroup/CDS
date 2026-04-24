package command

import (
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/output"
	"github.com/spf13/cobra"
)

type spcHostList struct {
	defaultCmd
}

var _ baseCmd = (*spcHostList)(nil)

func (s *spcHostList) command() *cobra.Command {
	if s.cmd == nil {
		s.cmd = &cobra.Command{
			Use:           "list",
			Aliases:       []string{"ls"},
			Short:         "List registered agent hosts",
			Args:          cobra.NoArgs,
			RunE:          s.runE,
			SilenceUsage:  true,
			SilenceErrors: true,
		}
	}
	return s.cmd
}

func (s *spcHostList) subCommands() []baseCmd {
	return s.subCmds
}

func (s *spcHostList) runE(cmd *cobra.Command, args []string) error {
	agents, err := config.RegisteredAgents()
	if err != nil {
		return err
	}

	agentEntries := make([]agentListEntry, 0, len(agents))
	for _, a := range agents {
		agentEntries = append(agentEntries, agentListEntry{
			Hostname:  a.TargetSrv,
			SSHTunnel: a.SshTunnel,
			TLS:       a.Certs.CA != "",
		})
	}

	o := output.FromContext(cmd.Context())
	tableToRender := prepareHostListOutput(agentEntries)
	return output.Render(o, tableToRender)
}

//
// RENDERING
//

type agentListEntry struct {
	Hostname  string
	SSHTunnel bool
	TLS       bool
}

func prepareHostListOutput(agents []agentListEntry) output.TableResult {
	agentTotal := len(agents)

	headers := []string{"HOSTNAME", "SSH TUNNEL", "TLS"}
	rows := make([][]string, 0, agentTotal)
	entries := make([]agentListEntry, 0, agentTotal)

	for _, a := range agents {
		sshTunnel := "no"
		if a.SSHTunnel {
			sshTunnel = "yes"
		}
		hasTLS := "no"
		if a.TLS {
			hasTLS = "yes"
		}
		rows = append(rows, []string{a.Hostname, sshTunnel, hasTLS})
		entries = append(entries, agentListEntry{
			Hostname:  a.Hostname,
			SSHTunnel: a.SSHTunnel,
			TLS:       a.TLS,
		})
	}

	return output.TableResult{
		Headers: headers,
		Rows:    rows,
		Data:    entries,
	}
}
