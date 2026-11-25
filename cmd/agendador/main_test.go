package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
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
