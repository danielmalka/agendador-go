package scheduling

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"
)

type fakeLoader struct {
	jobs []Job
	err  error
}

func (f fakeLoader) Load(context.Context, string) ([]Job, error) { return f.jobs, f.err }

type fakeRunner struct {
	res Result
	err error
	ran []string
}

func (f *fakeRunner) Run(_ context.Context, j Job) (Result, error) {
	f.ran = append(f.ran, j.Name)
	return f.res, f.err
}

type fakeScheduler struct {
	fns      map[string]func(context.Context)
	addErr   error
	startErr error
	started  bool
	stopped  bool
	stopErr  error
	stopCtx  context.Context
}

func (f *fakeScheduler) Add(_ context.Context, j Job, fn func(context.Context)) error {
	if f.addErr != nil {
		return f.addErr
	}
	if f.fns == nil {
		f.fns = map[string]func(context.Context){}
	}
	f.fns[j.Name] = fn
	return nil
}

func (f *fakeScheduler) Start(context.Context) error {
	f.started = true
	return f.startErr
}

func (f *fakeScheduler) Stop(ctx context.Context) error {
	f.stopped, f.stopCtx = true, ctx
	return f.stopErr
}

func newSvc(l fakeLoader, r *fakeRunner, s *fakeScheduler, logs *bytes.Buffer) UseCase {
	return NewService(l, r, s, slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
}

func TestStartRegistersStartsAndStops(t *testing.T) {
	var logs bytes.Buffer
	sched := &fakeScheduler{}
	runner := &fakeRunner{res: Result{ExitCode: 0}}
	uc := newSvc(fakeLoader{jobs: []Job{{Name: "a"}, {Name: "b"}}}, runner, sched, &logs)

	if err := uc.Start(context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	if len(sched.fns) != 2 || !sched.started || sched.stopped {
		t.Fatalf("sched = %+v", sched)
	}

	sched.fns["a"](context.Background())
	if len(runner.ran) != 1 || runner.ran[0] != "a" {
		t.Fatalf("ran = %v", runner.ran)
	}
	if !strings.Contains(logs.String(), "job finished") || !strings.Contains(logs.String(), "exit_code=0") {
		t.Fatalf("logs = %s", logs.String())
	}

	stopCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := uc.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if !sched.stopped || sched.stopCtx.Err() == nil {
		t.Fatalf("sched stop not propagated: %+v", sched)
	}
}

func TestJobFailureIsLogged(t *testing.T) {
	var logs bytes.Buffer
	sched := &fakeScheduler{}
	runner := &fakeRunner{res: Result{ExitCode: 3, Stderr: "boom"}, err: errors.New("exit status 3")}
	uc := newSvc(fakeLoader{jobs: []Job{{Name: "a"}}}, runner, sched, &logs)
	if err := uc.Start(context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	sched.fns["a"](context.Background())
	for _, want := range []string{"job failed", "exit_code=3", "boom"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs missing %q: %s", want, logs.String())
		}
	}
}

func TestLifecycleErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		loader fakeLoader
		sched  *fakeScheduler
		stop   bool
	}{
		{"load error", fakeLoader{err: boom}, &fakeScheduler{}, false},
		{"add error", fakeLoader{jobs: []Job{{Name: "a"}}}, &fakeScheduler{addErr: boom}, false},
		{"start error", fakeLoader{jobs: []Job{{Name: "a"}}}, &fakeScheduler{startErr: boom}, false},
		{"stop error", fakeLoader{jobs: []Job{{Name: "a"}}}, &fakeScheduler{stopErr: boom}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			uc := newSvc(tt.loader, &fakeRunner{}, tt.sched, &logs)
			err := uc.Start(context.Background(), "x.yml")
			if tt.stop {
				if err != nil {
					t.Fatalf("unexpected start err = %v", err)
				}
				err = uc.Stop(context.Background())
			}
			if !errors.Is(err, boom) {
				t.Fatalf("err = %v, want boom", err)
			}
			if tt.sched.started && (tt.name == "load error" || tt.name == "add error") {
				t.Error("scheduler must not start after a setup error")
			}
		})
	}
}

func TestJobOutputIsTruncatedInLog(t *testing.T) {
	var logs bytes.Buffer
	sched := &fakeScheduler{}
	big := strings.Repeat("x", 64*1024)
	runner := &fakeRunner{res: Result{Stdout: big, Stderr: "é" + big}}
	uc := newSvc(fakeLoader{jobs: []Job{{Name: "a"}}}, runner, sched, &logs)
	if err := uc.Start(context.Background(), "x.yml"); err != nil {
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

func TestOutputLogLevels(t *testing.T) {
	run := func(res Result, err error) string {
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
		sched := &fakeScheduler{}
		uc := NewService(fakeLoader{jobs: []Job{{Name: "a"}}}, &fakeRunner{res: res, err: err}, sched, logger)
		if e := uc.Start(context.Background(), "x.yml"); e != nil {
			t.Fatal(e)
		}
		sched.fns["a"](context.Background())
		return logs.String()
	}
	ok := run(Result{Stdout: "OUT", Stderr: "ERR"}, nil)
	if strings.Contains(ok, "OUT") || strings.Contains(ok, "ERR") || !strings.Contains(ok, "job finished") {
		t.Fatalf("info-level success must not log output: %s", ok)
	}
	failed := run(Result{Stdout: "OUT", Stderr: "ERR"}, errors.New("x"))
	if strings.Contains(failed, "OUT") || !strings.Contains(failed, "ERR") {
		t.Fatalf("failure should log stderr but not stdout at info: %s", failed)
	}

	var dbg bytes.Buffer
	sched := &fakeScheduler{}
	uc := newSvc(fakeLoader{jobs: []Job{{Name: "a"}}}, &fakeRunner{res: Result{Stdout: "OUT", Stderr: "ERR"}}, sched, &dbg)
	_ = uc.Start(context.Background(), "x.yml")
	sched.fns["a"](context.Background())
	if !strings.Contains(dbg.String(), "level=DEBUG") || !strings.Contains(dbg.String(), "stdout=OUT") || !strings.Contains(dbg.String(), "stderr=ERR") {
		t.Fatalf("debug should log stdout and stderr: %s", dbg.String())
	}
}
