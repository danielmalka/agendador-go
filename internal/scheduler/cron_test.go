package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielmalka/agendador-go/internal/domain"
)

func TestValidateSchedule(t *testing.T) {
	for _, spec := range []string{"* * * * *", "0 * * * * *", "@every 1s", "@daily"} {
		if err := ValidateSchedule(spec); err != nil {
			t.Errorf("%q rejected: %v", spec, err)
		}
	}
	for _, spec := range []string{"", "nope", "61 * * * *", "* * *"} {
		if ValidateSchedule(spec) == nil {
			t.Errorf("%q accepted", spec)
		}
	}
}

func TestAddInvalidSchedule(t *testing.T) {
	c := NewCron(slog.Default())
	err := c.Add(domain.Job{Name: "j", Schedule: "nope"}, func(context.Context) {})
	if err == nil || !strings.Contains(err.Error(), `"j"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestSkipIfRunningSkipsAndLogs(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	started, release := make(chan struct{}), make(chan struct{})
	var runs atomic.Int32
	wrapped := skipIfRunning("slow", logger, func(context.Context) {
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
	if !strings.Contains(logs.String(), "skipped") || !strings.Contains(logs.String(), "job=slow") {
		t.Fatalf("skip not logged: %q", logs.String())
	}
	wrapped(context.Background()) // runs again once free
	if runs.Load() != 2 {
		t.Fatalf("runs = %d, want 2 after release", runs.Load())
	}
}

func TestCronTriggersAndStops(t *testing.T) {
	c := NewCron(slog.Default())
	ran := make(chan struct{}, 1)
	if err := c.Add(domain.Job{Name: "tick", Schedule: "@every 1s"}, func(context.Context) {
		select {
		case ran <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	c.Start()
	select {
	case <-ran:
	case <-time.After(3 * time.Second):
		t.Fatal("job never triggered")
	}
	if err := c.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStopCancelsJobContextWhenWaitExpires(t *testing.T) {
	c := NewCron(slog.Default())
	started, canceled := make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	_ = c.Add(domain.Job{Name: "hang", Schedule: "@every 1s"}, func(ctx context.Context) {
		if once.CompareAndSwap(false, true) {
			close(started)
			<-ctx.Done()
			close(canceled)
		}
	})
	c.Start()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("job never started")
	}
	stopCtx, cancel := context.WithCancel(context.Background())
	cancel() // wait budget already exhausted
	if err := c.Stop(stopCtx); err == nil {
		t.Fatal("want error when running job outlives ctx")
	}
	select {
	case <-canceled:
	default:
		t.Fatal("job context was not canceled by Stop")
	}
}

func TestPanicInJobIsRecoveredAndLogged(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	var runs atomic.Int32
	wrapped := skipIfRunning("boom", logger, func(context.Context) {
		if runs.Add(1) == 1 {
			panic("kaboom")
		}
	})
	wrapped(context.Background()) // must not panic
	if !strings.Contains(logs.String(), "job panicked") || !strings.Contains(logs.String(), "job=boom") || !strings.Contains(logs.String(), "kaboom") {
		t.Fatalf("panic not logged: %s", logs.String())
	}
	wrapped(context.Background()) // job is not left marked as running
	if runs.Load() != 2 {
		t.Fatalf("runs = %d, want 2", runs.Load())
	}
}
