// Package config loads and validates CLI startup settings for agendador.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

const (
	// DefaultJobsPath is the default path to the jobs YAML file.
	DefaultJobsPath = "configs/jobs.yml"
	// DefaultShutdownTimeout bounds how long shutdown waits for running jobs.
	DefaultShutdownTimeout = 30 * time.Second
)

var (
	// ErrInvalidShutdownTimeout indicates a non-positive -shutdown-timeout flag.
	ErrInvalidShutdownTimeout = errors.New("usage: -shutdown-timeout must be greater than zero")
	// ErrInvalidLogLevel indicates an unsupported -log-level flag.
	ErrInvalidLogLevel = errors.New("usage: -log-level must be debug, info, warn or error")
)

// Config holds the validated startup settings of the application.
type Config struct {
	JobsPath        string
	ShutdownTimeout time.Duration
	LogLevel        slog.Level
}

// Load parses CLI flags from args, writing usage messages to stderr.
func Load(args []string, stderr io.Writer) (Config, error) {
	fs := flag.NewFlagSet("agendador", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jobsPath := fs.String("config", DefaultJobsPath, "path to the jobs YAML file")
	shutdownTimeout := fs.Duration("shutdown-timeout", DefaultShutdownTimeout, "how long to wait for running jobs on shutdown (must be > 0)")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if *shutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("%w, got %s", ErrInvalidShutdownTimeout, *shutdownTimeout)
	}
	level, err := parseLogLevel(*logLevel)
	if err != nil {
		return Config{}, err
	}
	return Config{
		JobsPath:        *jobsPath,
		ShutdownTimeout: *shutdownTimeout,
		LogLevel:        level,
	}, nil
}

func parseLogLevel(name string) (slog.Level, error) {
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("%w, got %q", ErrInvalidLogLevel, name)
}
