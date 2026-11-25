package scheduling

import (
	"context"
	"fmt"
	"log/slog"
	"unicode/utf8"
)

type service struct {
	loader    JobLoader
	runner    Runner
	scheduler Scheduler
	logger    *slog.Logger
}

// NewService builds the scheduling use case.
func NewService(l JobLoader, r Runner, s Scheduler, logger *slog.Logger) UseCase {
	return &service{
		loader:    l,
		runner:    r,
		scheduler: s,
		logger:    logger,
	}
}

// Start loads the jobs from configPath, registers them, and starts the scheduler.
func (s *service) Start(ctx context.Context, configPath string) error {
	jj, err := s.loader.Load(ctx, configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	for _, j := range jj {
		if err := s.scheduler.Add(ctx, j, s.jobFunc(j)); err != nil {
			return err
		}
		s.logger.Info("job registered", "job", j.Name, "schedule", j.Schedule)
	}

	if err := s.scheduler.Start(ctx); err != nil {
		return err
	}
	s.logger.Info("scheduler started", "jobs", len(jj))
	return nil
}

// Stop halts new triggers and waits for running jobs until ctx is done.
func (s *service) Stop(ctx context.Context) error {
	return s.scheduler.Stop(ctx)
}

func (s *service) jobFunc(j Job) func(context.Context) {
	return func(ctx context.Context) {
		s.logger.Info("job started", "job", j.Name)
		res, err := s.runner.Run(ctx, j)
		base := []any{"job", j.Name, "exit_code", res.ExitCode, "duration", res.Duration}
		output := []any{"job", j.Name, "stdout", truncateForLog(res.Stdout)}
		if err != nil {
			s.logger.Debug("job output", output...)
			s.logger.Error("job failed", append(base, "error", err, "stderr", truncateForLog(res.Stderr))...)
			return
		}
		s.logger.Debug("job output", append(output, "stderr", truncateForLog(res.Stderr))...)
		s.logger.Info("job finished", base...)
	}
}

const (
	maxLoggedOutput = 4 * 1024
	truncatedMark   = "...(truncated)"
)

// truncateForLog caps s at maxLoggedOutput bytes (on a rune boundary) and marks the cut.
func truncateForLog(s string) string {
	if len(s) <= maxLoggedOutput {
		return s
	}
	n := maxLoggedOutput
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + truncatedMark
}
