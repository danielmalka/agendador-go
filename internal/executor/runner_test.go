package executor

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielmalka/agendador-go/internal/scheduling"
)

func cmdJob(command string, args ...string) scheduling.Job {
	return scheduling.Job{Name: "t", Schedule: "@daily", Type: scheduling.TypeCommand, Command: command, Args: args}
}

func TestRunSuccess(t *testing.T) {
	res, err := NewRunner(DefaultMaxOutput).Run(context.Background(), cmdJob("echo", "hello", "world"))
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "hello world" || res.Duration <= 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunNonZeroExit(t *testing.T) {
	res, err := NewRunner(DefaultMaxOutput).Run(context.Background(), cmdJob("false"))
	if err == nil || res.ExitCode != 1 {
		t.Fatalf("err=%v res=%+v, want error and exit code 1", err, res)
	}
}

func TestRunStderr(t *testing.T) {
	res, _ := NewRunner(DefaultMaxOutput).Run(context.Background(), cmdJob("ls", "/definitely/missing"))
	if res.ExitCode == 0 || res.Stderr == "" {
		t.Fatalf("want stderr and non-zero exit, got %+v", res)
	}
}

func TestRunCommandNotFound(t *testing.T) {
	res, err := NewRunner(DefaultMaxOutput).Run(context.Background(), cmdJob("no-such-binary-agendador"))
	if err == nil || res.ExitCode != -1 {
		t.Fatalf("err=%v res=%+v, want error and exit code -1", err, res)
	}
}

func TestRunTimeout(t *testing.T) {
	job := cmdJob("sleep", "5")
	job.Timeout = 100 * time.Millisecond
	start := time.Now()
	_, err := NewRunner(DefaultMaxOutput).Run(context.Background(), job)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout did not kill the process promptly")
	}
}

func TestRunCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewRunner(DefaultMaxOutput).Run(ctx, cmdJob("sleep", "5")); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
}

func TestRunEnv(t *testing.T) {
	t.Setenv("AGD_INHERITED", "yes")
	job := cmdJob("env")
	job.Env = map[string]string{"AGD_EXTRA": "42"}
	res, err := NewRunner(DefaultMaxOutput).Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"AGD_INHERITED=yes", "AGD_EXTRA=42"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("env output missing %q", want)
		}
	}
}

func TestRunWorkDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "agd")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	job := cmdJob("pwd")
	job.WorkDir = dir
	res, err := NewRunner(DefaultMaxOutput).Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.Stat(strings.TrimSpace(res.Stdout))
	want, _ := os.Stat(dir)
	if got == nil || !os.SameFile(got, want) {
		t.Fatalf("pwd = %q, want %q", res.Stdout, dir)
	}
}

func TestRunOutputIsCapped(t *testing.T) {
	res, err := NewRunner(10).Run(context.Background(), cmdJob("echo", "0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "0123456789" {
		t.Fatalf("stdout = %q, want first 10 bytes", res.Stdout)
	}
}

func TestRunPythonUsesInterpreter(t *testing.T) {
	job := scheduling.Job{Type: scheduling.TypePython, Interpreter: "echo", Command: "script.py", Args: []string{"a"}}
	res, err := NewRunner(DefaultMaxOutput).Run(context.Background(), job)
	if err != nil || strings.TrimSpace(res.Stdout) != "script.py a" {
		t.Fatalf("err=%v stdout=%q", err, res.Stdout)
	}
}

func TestCappedBufferReportsFullWrite(t *testing.T) {
	b := &cappedBuffer{max: 2}
	for i := 0; i < 2; i++ { // second write hits a full buffer
		if n, err := b.Write([]byte("abcd")); n != 4 || err != nil {
			t.Fatalf("Write = %d, %v", n, err)
		}
	}
	if b.String() != "ab" {
		t.Fatalf("buffer = %q", b.String())
	}
}
