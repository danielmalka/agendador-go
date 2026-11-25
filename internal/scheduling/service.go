package scheduling

import (
	"context"
	"fmt"
)

type service struct {
	loader    JobLoader
	runner    Runner
	scheduler Scheduler
	observer  Observer
}

// NewService builds the scheduling use case.
func NewService(l JobLoader, r Runner, s Scheduler, obs Observer) UseCase {
	if obs == nil {
		obs = ObserverFunc(func(context.Context, Event) {})
	}
	return &service{
		loader:    l,
		runner:    r,
		scheduler: s,
		observer:  obs,
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
		s.observer.Observe(ctx, Event{Kind: EventJobRegistered, Job: j})
	}

	if err := s.scheduler.Start(ctx); err != nil {
		return err
	}
	s.observer.Observe(ctx, Event{Kind: EventSchedulerStarted, JobCount: len(jj)})
	return nil
}

// Stop halts new triggers and waits for running jobs until ctx is done.
func (s *service) Stop(ctx context.Context) error {
	return s.scheduler.Stop(ctx)
}

func (s *service) jobFunc(j Job) func(context.Context) {
	return func(ctx context.Context) {
		s.observer.Observe(ctx, Event{Kind: EventJobStarted, Job: j})
		res, err := s.runner.Run(ctx, j)
		if err != nil {
			s.observer.Observe(ctx, Event{Kind: EventJobFailed, Job: j, Result: res, Err: err})
			return
		}
		s.observer.Observe(ctx, Event{Kind: EventJobFinished, Job: j, Result: res})
	}
}
