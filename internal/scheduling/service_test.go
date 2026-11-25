package scheduling

import (
	"context"
	"errors"
	"testing"
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

type recordingObserver struct {
	events []Event
}

func (o *recordingObserver) Observe(_ context.Context, e Event) {
	o.events = append(o.events, e)
}

func TestStartRegistersStartsAndStops(t *testing.T) {
	obs := &recordingObserver{}
	sched := &fakeScheduler{}
	runner := &fakeRunner{res: Result{ExitCode: 0}}
	uc := NewService(fakeLoader{jobs: []Job{{Name: "a"}, {Name: "b"}}}, runner, sched, obs)

	if err := uc.Start(context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	if len(sched.fns) != 2 || !sched.started || sched.stopped {
		t.Fatalf("sched = %+v", sched)
	}
	if len(obs.events) != 3 || obs.events[0].Kind != EventJobRegistered || obs.events[2].Kind != EventSchedulerStarted || obs.events[2].JobCount != 2 {
		t.Fatalf("unexpected start events: %+v", obs.events)
	}

	sched.fns["a"](context.Background())
	if len(runner.ran) != 1 || runner.ran[0] != "a" {
		t.Fatalf("ran = %v", runner.ran)
	}
	if len(obs.events) != 5 || obs.events[3].Kind != EventJobStarted || obs.events[4].Kind != EventJobFinished {
		t.Fatalf("unexpected run events: %+v", obs.events)
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

func TestJobFailureIsObserved(t *testing.T) {
	obs := &recordingObserver{}
	sched := &fakeScheduler{}
	boom := errors.New("exit status 3")
	runner := &fakeRunner{res: Result{ExitCode: 3, Stderr: "boom"}, err: boom}
	uc := NewService(fakeLoader{jobs: []Job{{Name: "a"}}}, runner, sched, obs)
	if err := uc.Start(context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	sched.fns["a"](context.Background())
	last := obs.events[len(obs.events)-1]
	if last.Kind != EventJobFailed || !errors.Is(last.Err, boom) || last.Result.ExitCode != 3 || last.Result.Stderr != "boom" {
		t.Fatalf("unexpected failed event: %+v", last)
	}
}

func TestNilObserverIsSafe(t *testing.T) {
	sched := &fakeScheduler{}
	uc := NewService(fakeLoader{jobs: []Job{{Name: "a"}}}, &fakeRunner{}, sched, nil)
	if err := uc.Start(context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	sched.fns["a"](context.Background())
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
			uc := NewService(tt.loader, &fakeRunner{}, tt.sched, &recordingObserver{})
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
