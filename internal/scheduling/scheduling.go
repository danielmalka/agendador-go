// Package scheduling defines the core job scheduling domain, entities, errors, and ports.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// JobType says how a job's Command is launched.
type JobType string

const (
	// TypePython runs Command as a script through an interpreter.
	TypePython JobType = "python"
	// TypeCommand runs Command directly as an executable.
	TypeCommand JobType = "command"
)

// Schedule is a cron expression or descriptor accepted by a ScheduleValidator.
type Schedule string

// DefaultPythonInterpreter is used when a python job sets no Interpreter.
const DefaultPythonInterpreter = "python3"

var (
	// ErrEmptyName indicates a job without a name.
	ErrEmptyName = errors.New("name is required")
	// ErrEmptySchedule indicates a job without a schedule.
	ErrEmptySchedule = errors.New("schedule is required")
	// ErrEmptyCommand indicates a job without a command.
	ErrEmptyCommand = errors.New("command is required")
	// ErrInvalidJobType indicates an unsupported JobType.
	ErrInvalidJobType = errors.New("unknown type")
	// ErrNegativeTimeout indicates a negative job timeout.
	ErrNegativeTimeout = errors.New("timeout must not be negative")
	// ErrDuplicateJob indicates a repeated job name in the same configuration.
	ErrDuplicateJob = errors.New("duplicate name")
)

// Job is a scheduled unit of work.
type Job struct {
	Name        string
	Schedule    Schedule
	Type        JobType
	Command     string
	Args        []string
	Interpreter string
	Timeout     time.Duration // zero means no timeout
	Env         map[string]string
	WorkDir     string
}

// Validate checks the structural invariants of a job (not the schedule syntax).
func (j Job) Validate() error {
	switch {
	case j.Name == "":
		return ErrEmptyName
	case j.Schedule == "":
		return ErrEmptySchedule
	case j.Command == "":
		return ErrEmptyCommand
	case j.Type != TypePython && j.Type != TypeCommand:
		return fmt.Errorf("%w %q (want %q or %q)", ErrInvalidJobType, j.Type, TypePython, TypeCommand)
	case j.Timeout < 0:
		return ErrNegativeTimeout
	}
	return nil
}

// Argv returns the executable and arguments to launch; it never involves a shell.
func (j Job) Argv() (string, []string) {
	if j.Type == TypePython {
		interpreter := j.Interpreter
		if interpreter == "" {
			interpreter = DefaultPythonInterpreter
		}
		return interpreter, append([]string{j.Command}, j.Args...)
	}
	return j.Command, append([]string(nil), j.Args...)
}

// Result is the outcome of one job execution. Output is truncated by the runner.
type Result struct {
	ExitCode int
	Duration time.Duration
	Stdout   string
	Stderr   string
}

// UseCase defines the job scheduling application operations.
type UseCase interface {
	Start(ctx context.Context, configPath string) error
	Stop(ctx context.Context) error
}

// JobLoader reads and validates job definitions.
type JobLoader interface {
	Load(ctx context.Context, path string) ([]Job, error)
}

// ScheduleValidator checks whether a schedule expression is valid.
type ScheduleValidator interface {
	Validate(ctx context.Context, s Schedule) error
}

// ScheduleValidatorFunc adapts a function to ScheduleValidator.
type ScheduleValidatorFunc func(ctx context.Context, s Schedule) error

// Validate implements ScheduleValidator.
func (f ScheduleValidatorFunc) Validate(ctx context.Context, s Schedule) error {
	return f(ctx, s)
}

// Runner executes a job once. Result is always populated; the error reports
// failures such as a non-zero exit, a timeout or a launch problem.
type Runner interface {
	Run(ctx context.Context, j Job) (Result, error)
}

// Scheduler triggers registered functions on each job's schedule.
type Scheduler interface {
	Add(ctx context.Context, j Job, fn func(ctx context.Context)) error
	Start(ctx context.Context) error
	// Stop halts new triggers and waits for running ones until ctx is done.
	Stop(ctx context.Context) error
}
