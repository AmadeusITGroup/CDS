package main

import (
	"fmt"
	"log/slog"
	"os"

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

	// Bootstrap local agent
	if err = bootstrap.StartAgent(cg.KLocalhost); err != nil {
		if _, ok := err.(bootstrap.StartOnRunError); ok {
			err = nil
			clog.Debug("Agent is already running")
		} else {
			err = cerr.AppendError("Failed to start local agent", err)
		}
		return
	}

	// Init profile from config-resolved reader
	r, profileErr := config.ProfileReader()
	if profileErr != nil {
		clog.Warn("Failed to read profile source, skipping", profileErr)
	} else {
		profile.New(profile.WithReader(r))
	}
}

type cmd interface {
	Execute() error
}

func main() {

	if err := db.Load(config.DBSource()); err != nil {
		clog.Error("Failed to load state from database", err)
		os.Exit(1)
	}
	var saveConfigErr error
	defer func() {
		saveConfigErr = db.Save()
		if saveConfigErr != nil {
			clog.Error("Failed to save state to database", saveConfigErr)
			os.Exit(1)
		}
	}()

	if err := root.Execute(); err != nil {
		clog.Error(fmt.Sprintf("Failed to execute command: %v", err))
		os.Exit(1)
	}
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
