package domain

import (
	"context"
	"time"
)

// Result is the outcome of one job execution. Output is truncated by the runner.
type Result struct {
	ExitCode int
	Duration time.Duration
	Stdout   string
	Stderr   string
}

// ConfigLoader reads and validates the job definitions.
type ConfigLoader interface {
	Load(path string) ([]Job, error)
}

// Runner executes a job once. Result is always populated; the error reports
// failures such as a non-zero exit, a timeout or a launch problem.
type Runner interface {
	Run(ctx context.Context, job Job) (Result, error)
}

// Scheduler triggers registered functions on each job's schedule.
type Scheduler interface {
	Add(job Job, fn func(ctx context.Context)) error
	Start()
	// Stop halts new triggers and waits for running ones until ctx is done.
	Stop(ctx context.Context) error
}
