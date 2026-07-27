package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/amadeusitgroup/cds/internal/bootstrap"
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/amadeusitgroup/cds/internal/command"
	"github.com/amadeusitgroup/cds/internal/config"
	"github.com/amadeusitgroup/cds/internal/db"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/profile"
	"github.com/mattn/go-isatty"
)

var (
	root cmd = command.New()
)

func init() {
	setupLogger()
}

func init() {
	var err error
	defer func() {
		if err != nil {
			clog.Error("cds init failed", err)
			os.Exit(1)
		}
	}()

	if err = config.InitCLIConfig(); err != nil {
		err = cerr.AppendError("Failed to initialize CDS config", err)
		return
	}

	// Init profile from config-resolved reader when an optional profile exists.
	r, profileExists, profileErr := config.OptionalProfileReader()
	if profileErr != nil {
		clog.Warn("Failed to read profile source, skipping", profileErr)
	} else if profileExists {
		profile.New(profile.WithReader(r))
	}
}

type cmd interface {
	Execute() error
}

func main() {
	dbSrc, err := config.DBSource()
	if err != nil {
		clog.Error("Failed to resolve state database source", err)
		os.Exit(1)
	}

	if err := db.Load(dbSrc); err != nil {
		clog.Error("Failed to load state from database", err)
		os.Exit(1)
	}
	exitCode := 0
	defer func() {
		if saveConfigErr := db.Save(); saveConfigErr != nil {
			clog.Error("Failed to save state to database", saveConfigErr)
			exitCode = 1
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	if err := startRegisteredLocalAgent(os.Args[1:]); err != nil {
		clog.Error("Failed to start local agent", err)
		exitCode = 1
		return
	}

	if err := root.Execute(); err != nil {
		clog.Error(fmt.Sprintf("Failed to execute command: %v", err))
		exitCode = 1
		return
	}
}

func startRegisteredLocalAgent(args []string) error {
	if isLocalHostDeleteCommand(args) {
		return nil
	}
	if !db.HasHost(cg.KLocalhost) {
		return nil
	}
	if _, addrErr := config.AgentAddress(cg.KLocalhost); addrErr != nil {
		return nil
	}
	ownership, err := bootstrap.StartAgent(cg.KLocalhost)
	if err != nil {
		if _, ok := err.(bootstrap.StartOnRunError); ok {
			clog.Debug("Agent is already running")
			return nil
		}
		return err
	}
	if ownership.Manager != cg.EmptyStr && db.HasHost(cg.KLocalhost) {
		return db.SetHostAgentOwnership(cg.KLocalhost, ownership)
	}
	return nil
}

func isLocalHostDeleteCommand(args []string) bool {
	if len(args) < 4 || args[0] != "space" || args[1] != "host" {
		return false
	}
	subcommand := args[2]
	if subcommand != "delete" && subcommand != "remove" && subcommand != "rm" {
		return false
	}
	target := strings.TrimSpace(args[3])
	return target == cg.KLocalhost || strings.HasPrefix(target, ":")
}

// TODO:BK: refactor - logging implementation details exposed into the wild
func setupLogger() {
	// TODO: get level from cmd
	handlerOptions := slog.HandlerOptions{
		ReplaceAttr: cerr.ReplaceAttrErr,
		Level:       slog.LevelDebug,
	}
	var handlers []slog.Handler
	logfilePath := cenv.ConfigFile("logs") // workaround to avoid circular dep between logging and cenv.
	logFile, err := os.OpenFile(logfilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		handlers = append(handlers, slog.NewJSONHandler(logFile, &handlerOptions))
	}
	customCLIHandler := clog.NewCliHandler(
		os.Stdout,
		clog.WithLevel(slog.LevelDebug),
		clog.WithNoColor(isNoColorSet() || !isColorable()),
		clog.WithTimeFormat(cg.KLogTimeFormat),
		clog.WithReplaceAttr(cerr.ReplaceAttrErr),
	)
	handlers = append(handlers, customCLIHandler)
	logger := slog.New(clog.NewLevelHandler(slog.LevelDebug, clog.NewFanoutHandler(handlers...)))
	clog.SetLogger(logger)
}

func isNoColorSet() bool {
	_, ok := os.LookupEnv("NO_COLOR")
	return ok
}

func isColorable() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}
