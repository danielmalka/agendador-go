package config

import (
	"bytes"
	"errors"
	"flag"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := Load(nil, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JobsPath != DefaultJobsPath || cfg.ShutdownTimeout != DefaultShutdownTimeout || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadCustomFlags(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := Load([]string{"-config", "custom.yml", "-shutdown-timeout", "45s", "-log-level", "WARN"}, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JobsPath != "custom.yml" || cfg.ShutdownTimeout != 45*time.Second || cfg.LogLevel != slog.LevelWarn {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadLogLevels(t *testing.T) {
	levels := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for in, want := range levels {
		cfg, err := Load([]string{"-log-level", in}, &bytes.Buffer{})
		if err != nil || cfg.LogLevel != want {
			t.Errorf("%s: got %v, %v", in, cfg.LogLevel, err)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{"unknown flag", []string{"-nope"}, nil},
		{"zero shutdown timeout", []string{"-shutdown-timeout", "0s"}, ErrInvalidShutdownTimeout},
		{"negative shutdown timeout", []string{"-shutdown-timeout", "-1s"}, ErrInvalidShutdownTimeout},
		{"invalid log level", []string{"-log-level", "loud"}, ErrInvalidLogLevel},
		{"help", []string{"-h"}, flag.ErrHelp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			_, err := Load(tt.args, &errOut)
			if err == nil {
				t.Fatal("want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.name == "help" && !strings.Contains(errOut.String(), "-config") {
				t.Fatalf("usage should go to stderr: %q", errOut.String())
			}
		})
	}
}
