// Command agendador runs the jobs defined in a YAML file on their cron schedules.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/danielmalka/agendador-go/internal/app"
	"github.com/danielmalka/agendador-go/internal/config"
	"github.com/danielmalka/agendador-go/internal/executor"
	"github.com/danielmalka/agendador-go/internal/scheduler"
)

const defaultShutdownTimeout = 30 * time.Second

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	forceCtx, forceCancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go escalate(sigs, cancel, forceCancel)

	err := run(ctx, forceCtx, os.Args[1:], os.Stdout, os.Stderr)
	cancel()
	forceCancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// escalate turns the first signal into a graceful shutdown and the second into
// a forced one. Jobs run in their own process groups and do not receive the
// terminal's Ctrl+C, so the second signal must kill them via forceCancel
// instead of letting the process die and orphan them.
func escalate(sigs <-chan os.Signal, graceful, force context.CancelFunc) {
	<-sigs
	graceful()
	<-sigs
	force()
}

func parseLogLevel(name string) (slog.Level, error) {
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("usage: -log-level must be debug, info, warn or error, got %q", name)
}

// run is the composition root: it wires adapters into the use case.
func run(ctx, forceCtx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("agendador", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "configs/jobs.yml", "path to the jobs YAML file")
	shutdownTimeout := fs.Duration("shutdown-timeout", defaultShutdownTimeout, "how long to wait for running jobs on shutdown (must be > 0)")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // usage was already printed
		}
		return err
	}
	if *shutdownTimeout <= 0 {
		return fmt.Errorf("usage: -shutdown-timeout must be greater than zero, got %s", *shutdownTimeout)
	}

	level, err := parseLogLevel(*logLevel)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: level}))
	svc := app.NewService(
		config.NewYAMLLoader(scheduler.ValidateSchedule),
		executor.NewRunner(executor.DefaultMaxOutput),
		scheduler.NewCron(logger),
		logger,
		*shutdownTimeout,
	)
	return svc.Run(ctx, forceCtx, *configPath)
}
