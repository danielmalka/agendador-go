//go:build unix

package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/danielmalka/agendador-go/internal/scheduling"
)

// The sh below is only the job's command under test; production code never uses a shell.
func TestCancelKillsGrandchildren(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pid")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	job := scheduling.Job{Type: scheduling.TypeCommand, Command: "sh",
		Args: []string{"-c", `sleep 300 & echo $! > "$1"; wait`, "sh", fifo}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := NewRunner(DefaultMaxOutput).Run(ctx, job); done <- err }()

	raw, err := os.ReadFile(fifo) // blocks until the shell has started the grandchild
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled", err)
	}
	// The orphan is reaped asynchronously by init, so poll with a deadline.
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still alive after cancel", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
