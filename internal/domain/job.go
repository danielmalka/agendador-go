// Package domain holds the core entities and ports; it depends on nothing else.
package domain

import (
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

// DefaultPythonInterpreter is used when a python job sets no Interpreter.
const DefaultPythonInterpreter = "python3"

// Job is a scheduled unit of work.
type Job struct {
	Name        string
	Schedule    string
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
		return errors.New("name is required")
	case j.Schedule == "":
		return errors.New("schedule is required")
	case j.Command == "":
		return errors.New("command is required")
	case j.Type != TypePython && j.Type != TypeCommand:
		return fmt.Errorf("unknown type %q (want %q or %q)", j.Type, TypePython, TypeCommand)
	case j.Timeout < 0:
		return errors.New("timeout must not be negative")
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
