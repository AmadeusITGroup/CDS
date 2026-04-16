package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/db"
)

var _ baseCmd = (*projectClear)(nil)

type projectClear struct {
	defaultCmd
}

func (pc *projectClear) command() *cobra.Command {
	if pc.cmd == nil {
		pc.cmd = &cobra.Command{
			Use:     "clear [PROJECT-NAME]",
			Aliases: []string{"cl"},
			Short:   "Deallocate deployed resources",
			Long: `Clear will remove any resources present on the target and relative to the project.
The cleared project will continue to target the same host.`,
			Args:              validateProjectNameFromArgsOrContext,
			RunE:              pc.execute,
			SilenceUsage:      true,
			SilenceErrors:     true,
			ValidArgsFunction: completionProject,
		}
		pc.initFlags()
		pc.initSubCommands()
	}
	return pc.cmd
}

func (pc *projectClear) subCommands() []baseCmd {
	return pc.subCmds
}

func (pc *projectClear) initFlags() {
}

func (pc *projectClear) initSubCommands() {
	pc.subCmds = []baseCmd{}
}

// projectClearMain is the core of the 'clear' command, reused by the 'drain' and 'delete' commands.
func projectClearMain(projectName string) error {
	// TODO: Agent Service interaction needed — full clear sequence:
	// 1. Sync container statuses with reality (safeSyncContainerInConfig)
	// 2. Stop running containers via engine
	// 3. Re-align container statuses
	// 4. Remove containers via engine
	// 5. Re-align container statuses
	// 6. Verify no containers remain

	containers := db.ProjectContainersName(projectName)
	if len(containers) == 0 {
		clog.Warn("No containers found in configuration, nothing to clear.")
		return nil
	}

	return cerr.NewError("TODO: Agent service not yet implemented — cannot execute clear (stop + remove containers) operation")
}

func (pc *projectClear) execute(cmd *cobra.Command, args []string) error {
	projectName := getProjectNameFromArgsOrContext(args)
	clog.Info(fmt.Sprintf("Using project '%s'.", projectName))

	if err := projectClearMain(projectName); err != nil {
		return err
	}

	clog.Info(fmt.Sprintf("Project %s cleared successfully", projectName))
	return nil
}
