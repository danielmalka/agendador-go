// Package scheduler adapts robfig/cron to the scheduling.Scheduler port.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/robfig/cron/v3"

	"github.com/danielmalka/agendador-go/internal/scheduling"
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

// Cron implements scheduling.Scheduler.
type Cron struct {
	cron    *cron.Cron
	logger  *slog.Logger
	baseCtx context.Context // parent of every job context; canceled when Stop gives up waiting
	cancel  context.CancelFunc
}

var _ scheduling.Scheduler = (*Cron)(nil)

// NewCron builds a scheduler that logs skipped runs to logger.
func NewCron(logger *slog.Logger) *Cron {
	return &Cron{cron: cron.New(cron.WithParser(parser)), logger: logger}
}

// Add registers fn on the job's schedule. A trigger is skipped while the
// previous run of the same job is still going.
func (c *Cron) Add(_ context.Context, j scheduling.Job, fn func(ctx context.Context)) error {
	wrapped := skipIfRunning(j.Name, c.logger, fn)
	if _, err := c.cron.AddFunc(string(j.Schedule), func() { wrapped(c.baseCtx) }); err != nil {
		return fmt.Errorf("schedule job %q: %w", j.Name, err)
	}
	return nil
}

// Start begins triggering jobs in the background.
func (c *Cron) Start(ctx context.Context) error {
	c.baseCtx, c.cancel = context.WithCancel(context.WithoutCancel(ctx))
	c.cron.Start()
	return nil
}

// Stop stops new triggers and waits for running jobs until ctx is done. If ctx
// expires first, the jobs' context is canceled so their processes are killed.
func (c *Cron) Stop(ctx context.Context) error {
	if c.cancel != nil {
		defer c.cancel()
	}
	done := c.cron.Stop().Done()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		if c.cancel != nil {
			c.cancel()
		}
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
