package cron

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielmalka/agendador-go/internal/scheduling"
	"github.com/danielmalka/agendador-go/internal/scheduling/mock"
)

func TestValidator(t *testing.T) {
	v := NewValidator()
	for _, spec := range []scheduling.Schedule{"* * * * *", "0 * * * * *", "@every 1s", "@daily"} {
		if err := v.Validate(context.Background(), spec); err != nil {
			t.Errorf("%q rejected: %v", spec, err)
		}
	}
	for _, spec := range []scheduling.Schedule{"", "nope", "61 * * * *", "* * *"} {
		if v.Validate(context.Background(), spec) == nil {
			t.Errorf("%q accepted", spec)
		}
	}
}

func TestAddInvalidSchedule(t *testing.T) {
	s := NewScheduler(nil)
	err := s.Add(context.Background(), scheduling.Job{Name: "j", Schedule: "nope"}, func(context.Context) {})
	if err == nil || !strings.Contains(err.Error(), `"j"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestSkipIfRunningSkipsAndObserves(t *testing.T) {
	obs := &mock.Observer{}
	started, release := make(chan struct{}), make(chan struct{})
	var runs atomic.Int32
	wrapped := skipIfRunning(scheduling.Job{Name: "slow"}, obs, func(context.Context) {
		if runs.Add(1) == 1 {
			close(started)
			<-release
		}
	})

	done := make(chan struct{})
	go func() { wrapped(context.Background()); close(done) }()
	<-started
	wrapped(context.Background()) // overlaps: must be skipped
	close(release)
	<-done

	if runs.Load() != 1 {
		t.Fatalf("runs = %d, want 1", runs.Load())
	}
	ee := obs.Events()
	if len(ee) != 1 || ee[0].Kind != scheduling.EventJobSkipped || ee[0].Job.Name != "slow" || !errors.Is(ee[0].Err, scheduling.ErrJobSkipped) {
		t.Fatalf("skip event not recorded: %+v", ee)
	}
	wrapped(context.Background()) // runs again once free
	if runs.Load() != 2 {
		t.Fatalf("runs = %d, want 2 after release", runs.Load())
	}
}

func TestCronTriggersAndStops(t *testing.T) {
	s := NewScheduler(nil)
	ran := make(chan struct{}, 1)
	if err := s.Add(context.Background(), scheduling.Job{Name: "tick", Schedule: "@every 1s"}, func(context.Context) {
		select {
		case ran <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(3 * time.Second):
		t.Fatal("job never triggered")
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStopCancelsJobContextWhenWaitExpires(t *testing.T) {
	s := NewScheduler(nil)
	started, canceled := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	_ = s.Add(context.Background(), scheduling.Job{Name: "hang", Schedule: "@every 1s"}, func(ctx context.Context) {
		if once.CompareAndSwap(false, true) {
			close(started)
			<-ctx.Done()
			close(canceled)
		}
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("job never started")
	}
	stopCtx, cancel := context.WithCancel(context.Background())
	cancel() // wait budget already exhausted
	if err := s.Stop(stopCtx); err == nil {
		t.Fatal("want error when running job outlives ctx")
	}
	select {
	case <-canceled:
	default:
		t.Fatal("job context was not canceled by Stop")
	}
}

func TestPanicInJobIsRecoveredAndObserved(t *testing.T) {
	obs := &mock.Observer{}
	var runs atomic.Int32
	wrapped := skipIfRunning(scheduling.Job{Name: "boom"}, obs, func(context.Context) {
		if runs.Add(1) == 1 {
			panic("kaboom")
		}
	})
	wrapped(context.Background()) // must not panic
	ee := obs.Events()
	if len(ee) != 1 || ee[0].Kind != scheduling.EventJobPanicked || ee[0].Job.Name != "boom" || !errors.Is(ee[0].Err, scheduling.ErrJobPanicked) || !strings.Contains(ee[0].Err.Error(), "kaboom") {
		t.Fatalf("panic event not recorded: %+v", ee)
	}
	wrapped(context.Background()) // job is not left marked as running
	if runs.Load() != 2 {
		t.Fatalf("runs = %d, want 2", runs.Load())
	}
}
