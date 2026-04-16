package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

var _ baseCmd = (*projectDrain)(nil)

type projectDrain struct {
	defaultCmd
}

func (pd *projectDrain) command() *cobra.Command {
	if pd.cmd == nil {
		pd.cmd = &cobra.Command{
			Use:     "drain [PROJECT-NAME]",
			Aliases: []string{"dr"},
			Short:   "Clear a project and remove its target",
			Long: "Drain is used to change deployment target, it will first clear the project from the host and then remove " +
				"the current target from the project configuration",
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              pd.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		pd.initFlags()
		pd.initSubCommands()
	}
	return pd.cmd
}

func (pd *projectDrain) subCommands() []baseCmd {
	return pd.subCmds
}

func (pd *projectDrain) initFlags() {
}

func (pd *projectDrain) initSubCommands() {
	pd.subCmds = []baseCmd{}
}

func (pd *projectDrain) execute(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)
	clog.Info(fmt.Sprintf("Using project '%s'.", projectName))

	// First, clear the project (stop + remove containers)
	if err := projectClearMain(projectName); err != nil {
		return err
	}

	// Then unregister from host and update project config
	unregisterFromHostWithAssertError(projectName)
	if err := db.RemoveHostAndContainersFromProject(projectName); err != nil {
		clog.Warn("Failed to update project config file on drain action", err)
		return nil
	}

	clog.Info(fmt.Sprintf("Project %s drained", projectName))
	return nil
}

// unregisterFromHostWithAssertError unregisters the project from the host, logging warnings on failure.
func unregisterFromHostWithAssertError(projectName string) {
	hostName := db.ProjectHostName(projectName)
	if len(hostName) == 0 {
		return
	}
	if err := db.RemoveProjectFromHost(hostName, projectName); err != nil {
		clog.Warn("Failed to update host config file on unregistering host action", err)
	}
}

// identifyProjectHost configures the host for a project. Simplified from original
// which had SSH key generation, remote key copy etc. via the agent.
func identifyProjectHost(projectName string, desiredHostName string) error {
	currentHostName := db.ProjectHostName(projectName)

	hostName := desiredHostName
	if len(currentHostName) == 0 {
		if len(desiredHostName) == 0 {
			hostName = "localhost"
			clog.Warn(fmt.Sprintf("No target specified. Using '%v'", hostName))
		}
	} else {
		if len(desiredHostName) != 0 && desiredHostName != currentHostName {
			clog.Warn(fmt.Sprintf(`Project '%s' is already mapped to a host '%s', cannot change its host to %s`,
				projectName, currentHostName, desiredHostName))
			clog.Info(fmt.Sprintf(`Tip: use "cds project drain %s" to free project resources and un-map the host, then rerun your command`, projectName))
			return cerr.NewError(fmt.Sprintf(`project "%v" is mapped to "%v" server while requesting a deployment on %v`,
				projectName, currentHostName, desiredHostName))
		} else if len(desiredHostName) == 0 {
			hostName = currentHostName
		}
	}

	if !db.HasHost(hostName) {
		// TODO: Agent Service interaction needed — build host (generate SSH keypair, copy public key to remote)
		// For now, just register the host in the config
		db.AddHost(hostName, "")
		clog.Debug(fmt.Sprintf("Registered new host '%s' (SSH key setup requires agent service)", hostName))
	}

	if err := db.SetProjectHost(projectName, hostName); err != nil {
		return cerr.AppendError(fmt.Sprintf("Failed to set project (%s)", projectName), err)
	}
	return nil
}
