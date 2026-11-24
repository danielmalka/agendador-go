package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/danielmalka/agendador-go/internal/domain"
)

type fakeLoader struct {
	jobs []domain.Job
	err  error
}

func (f fakeLoader) Load(string) ([]domain.Job, error) { return f.jobs, f.err }

type fakeRunner struct {
	res domain.Result
	err error
	ran []string
}

func (f *fakeRunner) Run(_ context.Context, j domain.Job) (domain.Result, error) {
	f.ran = append(f.ran, j.Name)
	return f.res, f.err
}

type fakeScheduler struct {
	fns     map[string]func(context.Context)
	addErr  error
	started bool
	stopped bool
	stopErr error
	stopCtx context.Context
}

func (f *fakeScheduler) Add(j domain.Job, fn func(context.Context)) error {
	if f.addErr != nil {
		return f.addErr
	}
	if f.fns == nil {
		f.fns = map[string]func(context.Context){}
	}
	f.fns[j.Name] = fn
	return nil
}
func (f *fakeScheduler) Start() { f.started = true }
func (f *fakeScheduler) Stop(ctx context.Context) error {
	f.stopped, f.stopCtx = true, ctx
	return f.stopErr
}

func newSvc(l fakeLoader, r *fakeRunner, s *fakeScheduler, logs *bytes.Buffer) *Service {
	return NewService(l, r, s, slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), time.Second)
}

func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestRunRegistersStartsAndStops(t *testing.T) {
	var logs bytes.Buffer
	sched := &fakeScheduler{}
	runner := &fakeRunner{res: domain.Result{ExitCode: 0}}
	svc := newSvc(fakeLoader{jobs: []domain.Job{{Name: "a"}, {Name: "b"}}}, runner, sched, &logs)

	if err := svc.Run(cancelled(), context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	if len(sched.fns) != 2 || !sched.started || !sched.stopped {
		t.Fatalf("sched = %+v", sched)
	}
	if _, ok := sched.stopCtx.Deadline(); !ok {
		t.Error("stop context should carry a deadline")
	}

	sched.fns["a"](context.Background())
	if len(runner.ran) != 1 || runner.ran[0] != "a" {
		t.Fatalf("ran = %v", runner.ran)
	}
	if !strings.Contains(logs.String(), "job finished") || !strings.Contains(logs.String(), "exit_code=0") {
		t.Fatalf("logs = %s", logs.String())
	}
}

func TestJobFailureIsLogged(t *testing.T) {
	var logs bytes.Buffer
	sched := &fakeScheduler{}
	runner := &fakeRunner{res: domain.Result{ExitCode: 3, Stderr: "boom"}, err: errors.New("exit status 3")}
	svc := newSvc(fakeLoader{jobs: []domain.Job{{Name: "a"}}}, runner, sched, &logs)
	if err := svc.Run(cancelled(), context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	sched.fns["a"](context.Background())
	for _, want := range []string{"job failed", "exit_code=3", "boom"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs missing %q: %s", want, logs.String())
		}
	}
}

func TestRunErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		loader fakeLoader
		sched  *fakeScheduler
	}{
		{"load error", fakeLoader{err: boom}, &fakeScheduler{}},
		{"add error", fakeLoader{jobs: []domain.Job{{Name: "a"}}}, &fakeScheduler{addErr: boom}},
		{"stop error", fakeLoader{jobs: []domain.Job{{Name: "a"}}}, &fakeScheduler{stopErr: boom}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			err := newSvc(tt.loader, &fakeRunner{}, tt.sched, &logs).Run(cancelled(), context.Background(), "x.yml")
			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want boom", err)
			}
			if tt.sched.started && tt.name != "stop error" {
				t.Error("scheduler must not start after a setup error")
			}
		})
	}
}

func TestJobOutputIsTruncatedInLog(t *testing.T) {
	var logs bytes.Buffer
	sched := &fakeScheduler{}
	big := strings.Repeat("x", 64*1024)
	runner := &fakeRunner{res: domain.Result{Stdout: big, Stderr: "é" + big}}
	svc := newSvc(fakeLoader{jobs: []domain.Job{{Name: "a"}}}, runner, sched, &logs)
	if err := svc.Run(cancelled(), context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	sched.fns["a"](context.Background())
	if !strings.Contains(logs.String(), "...(truncated)") || logs.Len() > 3*maxLoggedOutput {
		t.Fatalf("log not truncated, len=%d", logs.Len())
	}
	if got := truncateForLog("short"); got != "short" {
		t.Fatalf("short output changed: %q", got)
	}
	// cut must not split a multi-byte rune
	got := truncateForLog(strings.Repeat("é", maxLoggedOutput))
	if !utf8.ValidString(got) || !strings.HasSuffix(got, truncatedMark) {
		t.Fatalf("invalid truncation")
	}
}

func TestForceContextAbortsStopWait(t *testing.T) {
	var logs bytes.Buffer
	sched := &fakeScheduler{}
	svc := newSvc(fakeLoader{jobs: []domain.Job{{Name: "a"}}}, &fakeRunner{}, sched, &logs)
	forced, force := context.WithCancel(context.Background())
	force()
	if err := svc.Run(cancelled(), forced, "x.yml"); err != nil {
		t.Fatal(err)
	}
	if sched.stopCtx.Err() == nil {
		t.Fatal("stop context should already be canceled when forceCtx is canceled")
	}
}

func TestOutputLogLevels(t *testing.T) {
	run := func(res domain.Result, err error) string {
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
		sched := &fakeScheduler{}
		svc := NewService(fakeLoader{jobs: []domain.Job{{Name: "a"}}}, &fakeRunner{res: res, err: err}, sched, logger, time.Second)
		if e := svc.Run(cancelled(), context.Background(), "x.yml"); e != nil {
			t.Fatal(e)
		}
		sched.fns["a"](context.Background())
		return logs.String()
	}
	ok := run(domain.Result{Stdout: "OUT", Stderr: "ERR"}, nil)
	if strings.Contains(ok, "OUT") || strings.Contains(ok, "ERR") || !strings.Contains(ok, "job finished") {
		t.Fatalf("info-level success must not log output: %s", ok)
	}
	failed := run(domain.Result{Stdout: "OUT", Stderr: "ERR"}, errors.New("x"))
	if strings.Contains(failed, "OUT") || !strings.Contains(failed, "ERR") {
		t.Fatalf("failure should log stderr but not stdout at info: %s", failed)
	}

	var dbg bytes.Buffer
	sched := &fakeScheduler{}
	svc := newSvc(fakeLoader{jobs: []domain.Job{{Name: "a"}}}, &fakeRunner{res: domain.Result{Stdout: "OUT", Stderr: "ERR"}}, sched, &dbg)
	_ = svc.Run(cancelled(), context.Background(), "x.yml")
	sched.fns["a"](context.Background())
	if !strings.Contains(dbg.String(), "level=DEBUG") || !strings.Contains(dbg.String(), "stdout=OUT") || !strings.Contains(dbg.String(), "stderr=ERR") {
		t.Fatalf("debug should log stdout and stderr: %s", dbg.String())
	}
}
