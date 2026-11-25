// Package executor runs jobs as OS processes, never through a shell.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/danielmalka/agendador-go/internal/scheduling"
)

const (
	// DefaultMaxOutput caps each captured stream.
	DefaultMaxOutput = 64 * 1024
	// waitDelay bounds how long Wait lingers on pipes after a kill.
	waitDelay = 2 * time.Second
)

// Runner implements scheduling.Runner with os/exec.
type Runner struct {
	maxOutput int
}

var _ scheduling.Runner = (*Runner)(nil)

// NewRunner builds a Runner keeping at most maxOutput bytes of each stream.
func NewRunner(maxOutput int) *Runner {
	return &Runner{maxOutput: maxOutput}
}

// Run executes the job and waits for it to finish.
func (r *Runner) Run(ctx context.Context, j scheduling.Job) (scheduling.Result, error) {
	if j.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, j.Timeout)
		defer cancel()
	}

	bin, args := j.Argv()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = j.WorkDir
	cmd.Env = buildEnv(j.Env)
	cmd.WaitDelay = waitDelay
	configureProcessGroup(cmd)
	stdout, stderr := &cappedBuffer{max: r.maxOutput}, &cappedBuffer{max: r.maxOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	start := time.Now()
	err := cmd.Run()
	res := scheduling.Result{
		ExitCode: cmd.ProcessState.ExitCode(), // -1 when the process never started
		Duration: time.Since(start),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	return res, classify(ctx, err)
}

func classify(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("job interrupted: %w", ctxErr)
	}
	return err
}

func buildEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// cappedBuffer keeps the first max bytes and silently discards the rest, so the
// child never blocks on a full pipe.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
