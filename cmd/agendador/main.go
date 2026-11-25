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
	"syscall"

	"github.com/danielmalka/agendador-go/config"
	yamlconfig "github.com/danielmalka/agendador-go/internal/config"
	"github.com/danielmalka/agendador-go/internal/executor"
	"github.com/danielmalka/agendador-go/internal/scheduler"
	"github.com/danielmalka/agendador-go/internal/scheduling"
)

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

// run is the composition root: it wires adapters into the use case.
func run(ctx, forceCtx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := config.Load(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil // usage was already printed
		}
		return err
	}

	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	uc := scheduling.NewService(
		yamlconfig.NewYAMLLoader(scheduling.ScheduleValidatorFunc(func(_ context.Context, s scheduling.Schedule) error {
			return scheduler.ValidateSchedule(string(s))
		})),
		executor.NewRunner(executor.DefaultMaxOutput),
		scheduler.NewCron(logger),
		logger,
	)
	if err := uc.Start(ctx, cfg.JobsPath); err != nil {
		return err
	}
	<-ctx.Done()

	logger.Info("shutting down")
	stopCtx, cancel := context.WithTimeout(forceCtx, cfg.ShutdownTimeout)
	defer cancel()
	return uc.Stop(stopCtx)
}
