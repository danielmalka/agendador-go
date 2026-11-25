// Package cron adapts robfig/cron to the scheduling.Scheduler and scheduling.ScheduleValidator ports.
package cron

import (
	"context"
	"fmt"
	"sync/atomic"

	robfigcron "github.com/robfig/cron/v3"

	"github.com/danielmalka/agendador-go/internal/scheduling"
)

// parser accepts 5-field specs, an optional leading seconds field and
// descriptors such as @every and @daily. Validator and Scheduler share it.
var parser = robfigcron.NewParser(
	robfigcron.SecondOptional | robfigcron.Minute | robfigcron.Hour | robfigcron.Dom | robfigcron.Month | robfigcron.Dow | robfigcron.Descriptor,
)

// Validator implements scheduling.ScheduleValidator using robfig/cron's parser.
type Validator struct{}

var _ scheduling.ScheduleValidator = (*Validator)(nil)

// NewValidator builds a ScheduleValidator backed by robfig/cron.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate reports whether s is a schedule accepted by the cron parser.
func (*Validator) Validate(_ context.Context, s scheduling.Schedule) error {
	_, err := parser.Parse(string(s))
	return err
}

// Scheduler implements scheduling.Scheduler.
type Scheduler struct {
	cron     *robfigcron.Cron
	observer scheduling.Observer
	baseCtx  context.Context // parent of every job context; canceled when Stop gives up waiting
	cancel   context.CancelFunc
}

var _ scheduling.Scheduler = (*Scheduler)(nil)

// NewScheduler builds a scheduler that reports skips and panics to obs.
func NewScheduler(obs scheduling.Observer) *Scheduler {
	if obs == nil {
		obs = scheduling.ObserverFunc(func(context.Context, scheduling.Event) {})
	}
	return &Scheduler{cron: robfigcron.New(robfigcron.WithParser(parser)), observer: obs}
}

// Add registers fn on the job's schedule. A trigger is skipped while the
// previous run of the same job is still going.
func (s *Scheduler) Add(_ context.Context, j scheduling.Job, fn func(ctx context.Context)) error {
	wrapped := skipIfRunning(j, s.observer, fn)
	if _, err := s.cron.AddFunc(string(j.Schedule), func() { wrapped(s.baseCtx) }); err != nil {
		return fmt.Errorf("schedule job %q: %w", j.Name, err)
	}
	return nil
}

// Start begins triggering jobs in the background.
func (s *Scheduler) Start(ctx context.Context) error {
	s.baseCtx, s.cancel = context.WithCancel(context.WithoutCancel(ctx))
	s.cron.Start()
	return nil
}

// Stop stops new triggers and waits for running jobs until ctx is done. If ctx
// expires first, the jobs' context is canceled so their processes are killed.
func (s *Scheduler) Stop(ctx context.Context) error {
	if s.cancel != nil {
		defer s.cancel()
	}
	done := s.cron.Stop().Done()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		if s.cancel != nil {
			s.cancel()
		}
		<-done // jobs return promptly once killed; the runner bounds the wait
		return fmt.Errorf("waiting for running jobs: %w", ctx.Err())
	}
}

func skipIfRunning(j scheduling.Job, obs scheduling.Observer, fn func(ctx context.Context)) func(ctx context.Context) {
	var running atomic.Bool
	return func(ctx context.Context) {
		if !running.CompareAndSwap(false, true) {
			obs.Observe(ctx, scheduling.Event{
				Kind: scheduling.EventJobSkipped,
				Job:  j,
				Err:  scheduling.ErrJobSkipped,
			})
			return
		}
		defer running.Store(false)
		defer func() {
			if r := recover(); r != nil {
				obs.Observe(ctx, scheduling.Event{
					Kind: scheduling.EventJobPanicked,
					Job:  j,
					Err:  fmt.Errorf("%w: %v", scheduling.ErrJobPanicked, r),
				})
			}
		}()
		fn(ctx)
	}
}
