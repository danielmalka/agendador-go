// Package scheduler adapts robfig/cron to the domain.Scheduler port.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/robfig/cron/v3"

	"github.com/danielmalka/agendador-go/internal/domain"
)

// parser accepts 5-field specs, an optional leading seconds field and
// descriptors such as @every and @daily. Loader and scheduler share it.
var parser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// ValidateSchedule reports whether spec is a schedule the scheduler accepts.
func ValidateSchedule(spec string) error {
	_, err := parser.Parse(spec)
	return err
}

// Cron implements domain.Scheduler.
type Cron struct {
	cron    *cron.Cron
	logger  *slog.Logger
	baseCtx context.Context // parent of every job context; canceled when Stop gives up waiting
	cancel  context.CancelFunc
}

var _ domain.Scheduler = (*Cron)(nil)

// NewCron builds a scheduler that logs skipped runs to logger.
func NewCron(logger *slog.Logger) *Cron {
	ctx, cancel := context.WithCancel(context.Background())
	return &Cron{cron: cron.New(cron.WithParser(parser)), logger: logger, baseCtx: ctx, cancel: cancel}
}

// Add registers fn on the job's schedule. A trigger is skipped while the
// previous run of the same job is still going.
func (c *Cron) Add(job domain.Job, fn func(ctx context.Context)) error {
	wrapped := skipIfRunning(job.Name, c.logger, fn)
	if _, err := c.cron.AddFunc(job.Schedule, func() { wrapped(c.baseCtx) }); err != nil {
		return fmt.Errorf("schedule job %q: %w", job.Name, err)
	}
	return nil
}

// Start begins triggering jobs in the background.
func (c *Cron) Start() { c.cron.Start() }

// Stop stops new triggers and waits for running jobs until ctx is done. If ctx
// expires first, the jobs' context is canceled so their processes are killed.
func (c *Cron) Stop(ctx context.Context) error {
	defer c.cancel()
	done := c.cron.Stop().Done()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		c.cancel()
		<-done // jobs return promptly once killed; the runner bounds the wait
		return fmt.Errorf("waiting for running jobs: %w", ctx.Err())
	}
}

func skipIfRunning(name string, logger *slog.Logger, fn func(ctx context.Context)) func(ctx context.Context) {
	var running atomic.Bool
	return func(ctx context.Context) {
		if !running.CompareAndSwap(false, true) {
			logger.Warn("job skipped: previous run still in progress", "job", name)
			return
		}
		defer running.Store(false)
		defer func() {
			if r := recover(); r != nil {
				logger.Error("job panicked", "job", name, "panic", r)
			}
		}()
		fn(ctx)
	}
}
