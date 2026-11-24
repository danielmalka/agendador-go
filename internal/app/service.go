// Package app wires the use case: load jobs, schedule them, run until stopped.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/danielmalka/agendador-go/internal/domain"
)

// Service is the scheduler use case.
type Service struct {
	loader      domain.ConfigLoader
	runner      domain.Runner
	scheduler   domain.Scheduler
	logger      *slog.Logger
	stopTimeout time.Duration
}

// NewService builds the use case; stopTimeout bounds the wait for running jobs on shutdown.
func NewService(l domain.ConfigLoader, r domain.Runner, s domain.Scheduler, logger *slog.Logger, stopTimeout time.Duration) *Service {
	return &Service{loader: l, runner: r, scheduler: s, logger: logger, stopTimeout: stopTimeout}
}

// Run schedules every job in the config and blocks until ctx is done, then
// stops gracefully, waiting up to the shutdown timeout. Canceling forceCtx
// aborts that wait at once, which kills the running jobs.
func (s *Service) Run(ctx, forceCtx context.Context, configPath string) error {
	jobs, err := s.loader.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	for _, job := range jobs {
		if err := s.scheduler.Add(job, s.jobFunc(job)); err != nil {
			return err
		}
		s.logger.Info("job registered", "job", job.Name, "schedule", job.Schedule)
	}

	s.scheduler.Start()
	s.logger.Info("scheduler started", "jobs", len(jobs))
	<-ctx.Done()

	s.logger.Info("shutting down")
	stopCtx, cancel := context.WithTimeout(forceCtx, s.stopTimeout)
	defer cancel()
	return s.scheduler.Stop(stopCtx)
}

func (s *Service) jobFunc(job domain.Job) func(context.Context) {
	return func(ctx context.Context) {
		s.logger.Info("job started", "job", job.Name)
		res, err := s.runner.Run(ctx, job)
		base := []any{"job", job.Name, "exit_code", res.ExitCode, "duration", res.Duration}
		output := []any{"job", job.Name, "stdout", truncateForLog(res.Stdout)}
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
