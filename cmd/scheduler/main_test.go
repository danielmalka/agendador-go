package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/danielmalka/agendador-go/internal/scheduling"
)

func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestRunWithValidConfig(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "jobs.yml")
	yml := "jobs:\n  - {name: tick, schedule: '@every 1h', type: command, command: echo, args: [hi]}\n"
	if err := os.WriteFile(cfg, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := run(cancelled(), context.Background(), []string{"-config", cfg}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "job registered") || !strings.Contains(out.String(), "shutting down") {
		t.Fatalf("stdout = %s", out.String())
	}
}

func TestRunInvalidConfig(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "jobs.yml")
	if err := os.WriteFile(cfg, []byte("jobs:\n  - {name: a, schedule: nope, type: command, command: echo}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run(cancelled(), context.Background(), []string{"-config", cfg}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `job "a"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestRunMissingConfig(t *testing.T) {
	if err := run(cancelled(), context.Background(), []string{"-config", filepath.Join(t.TempDir(), "x.yml")}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("want error")
	}
}

func TestRunBadFlag(t *testing.T) {
	if err := run(cancelled(), context.Background(), []string{"-nope"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("want error")
	}
}

func TestRunInvalidShutdownTimeout(t *testing.T) {
	for _, v := range []string{"0s", "-1s"} {
		err := run(cancelled(), context.Background(), []string{"-shutdown-timeout", v}, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "shutdown-timeout") {
			t.Errorf("%s: err = %v, want usage error", v, err)
		}
	}
}

func TestEscalateFirstSignalGracefulSecondForced(t *testing.T) {
	sigs := make(chan os.Signal)
	graceful, cancelGraceful := context.WithCancel(context.Background())
	forced, cancelForced := context.WithCancel(context.Background())
	go escalate(sigs, cancelGraceful, cancelForced)

	sigs <- syscall.SIGINT
	<-graceful.Done()
	if forced.Err() != nil {
		t.Fatal("first signal must not force")
	}
	sigs <- syscall.SIGTERM
	<-forced.Done() // the test timeout guards against a hang
}

func TestRunInvalidLogLevel(t *testing.T) {
	err := run(cancelled(), context.Background(), []string{"-log-level", "loud"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "log-level") {
		t.Fatalf("err = %v, want usage error", err)
	}
}

func TestRunHelpIsSuccess(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run(context.Background(), context.Background(), []string{"-h"}, &out, &errOut); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !strings.Contains(errOut.String(), "-config") || out.Len() != 0 {
		t.Fatalf("usage should go to stderr only: out=%q err=%q", out.String(), errOut.String())
	}
}

func TestLogObserverFormatsLevelsAndTruncates(t *testing.T) {
	observe := func(level slog.Level, ee ...scheduling.Event) string {
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: level}))
		obs := newLogObserver(logger)
		for _, e := range ee {
			obs.Observe(context.Background(), e)
		}
		return logs.String()
	}

	ok := observe(slog.LevelInfo, scheduling.Event{
		Kind:   scheduling.EventJobFinished,
		Job:    scheduling.Job{Name: "a"},
		Result: scheduling.Result{Stdout: "OUT", Stderr: "ERR", Duration: time.Millisecond},
	})
	if strings.Contains(ok, "OUT") || strings.Contains(ok, "ERR") || !strings.Contains(ok, "job finished") {
		t.Fatalf("info-level success must not log output: %s", ok)
	}

	failed := observe(slog.LevelInfo, scheduling.Event{
		Kind:   scheduling.EventJobFailed,
		Job:    scheduling.Job{Name: "a"},
		Result: scheduling.Result{ExitCode: 3, Stdout: "OUT", Stderr: "ERR"},
		Err:    errors.New("exit status 3"),
	})
	if strings.Contains(failed, "OUT") || !strings.Contains(failed, "ERR") || !strings.Contains(failed, "job failed") {
		t.Fatalf("failure should log stderr but not stdout at info: %s", failed)
	}

	dbg := observe(slog.LevelDebug,
		scheduling.Event{Kind: scheduling.EventJobStarted, Job: scheduling.Job{Name: "a"}},
		scheduling.Event{Kind: scheduling.EventJobFinished, Job: scheduling.Job{Name: "a"}, Result: scheduling.Result{Stdout: "OUT", Stderr: "ERR"}},
		scheduling.Event{Kind: scheduling.EventJobSkipped, Job: scheduling.Job{Name: "a"}, Err: scheduling.ErrJobSkipped},
		scheduling.Event{Kind: scheduling.EventJobPanicked, Job: scheduling.Job{Name: "a"}, Err: errors.New("kaboom")},
	)
	for _, want := range []string{"level=DEBUG", "stdout=OUT", "stderr=ERR", "job started", "job skipped", "job panicked", "kaboom"} {
		if !strings.Contains(dbg, want) {
			t.Fatalf("debug logs missing %q: %s", want, dbg)
		}
	}

	if got := truncateForLog("short"); got != "short" {
		t.Fatalf("short output changed: %q", got)
	}
	got := truncateForLog(strings.Repeat("é", maxLoggedOutput))
	if !utf8.ValidString(got) || !strings.HasSuffix(got, truncatedMark) {
		t.Fatalf("invalid truncation: %q", got)
	}
}
