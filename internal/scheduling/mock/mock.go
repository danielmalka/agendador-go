// Package mock provides reusable test doubles for scheduling domain ports.
package mock

import (
	"context"
	"sync"

	"github.com/danielmalka/agendador-go/internal/scheduling"
)

var (
	_ scheduling.JobLoader         = (*Loader)(nil)
	_ scheduling.ScheduleValidator = (*ScheduleValidator)(nil)
	_ scheduling.Runner            = (*Runner)(nil)
	_ scheduling.Scheduler         = (*Scheduler)(nil)
	_ scheduling.Observer          = (*Observer)(nil)
)

// Loader is a test double for scheduling.JobLoader.
type Loader struct {
	Jobs []scheduling.Job
	Err  error
}

// Load returns the configured Jobs and Err.
func (l *Loader) Load(context.Context, string) ([]scheduling.Job, error) {
	return l.Jobs, l.Err
}

// ScheduleValidator is a test double for scheduling.ScheduleValidator.
type ScheduleValidator struct {
	Fn  func(ctx context.Context, s scheduling.Schedule) error
	Err error
}

// Validate delegates to Fn when set, otherwise returns Err.
func (v *ScheduleValidator) Validate(ctx context.Context, s scheduling.Schedule) error {
	if v.Fn != nil {
		return v.Fn(ctx, s)
	}
	return v.Err
}

// Runner is a test double for scheduling.Runner.
type Runner struct {
	Res scheduling.Result
	Err error
	Ran []string
}

// Run records the job name and returns Res and Err.
func (r *Runner) Run(_ context.Context, j scheduling.Job) (scheduling.Result, error) {
	r.Ran = append(r.Ran, j.Name)
	return r.Res, r.Err
}

// Scheduler is a test double for scheduling.Scheduler.
type Scheduler struct {
	Fns      map[string]func(context.Context)
	AddErr   error
	StartErr error
	Started  bool
	Stopped  bool
	StopErr  error
	StopCtx  context.Context
}

// Add stores fn by job name unless AddErr is set.
func (s *Scheduler) Add(_ context.Context, j scheduling.Job, fn func(context.Context)) error {
	if s.AddErr != nil {
		return s.AddErr
	}
	if s.Fns == nil {
		s.Fns = map[string]func(context.Context){}
	}
	s.Fns[j.Name] = fn
	return nil
}

// Start marks the scheduler as started and returns StartErr.
func (s *Scheduler) Start(context.Context) error {
	s.Started = true
	return s.StartErr
}

// Stop marks the scheduler as stopped, records ctx, and returns StopErr.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.Stopped, s.StopCtx = true, ctx
	return s.StopErr
}

// Observer is a thread-safe test double for scheduling.Observer.
type Observer struct {
	mu     sync.Mutex
	events []scheduling.Event
}

// Observe appends e to the recorded events.
func (o *Observer) Observe(_ context.Context, e scheduling.Event) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, e)
}

// Events returns a snapshot of the observed events.
func (o *Observer) Events() []scheduling.Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	ee := make([]scheduling.Event, len(o.events))
	copy(ee, o.events)
	return ee
}
