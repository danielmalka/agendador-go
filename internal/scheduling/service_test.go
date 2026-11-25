package scheduling_test

import (
	"context"
	"errors"
	"testing"

	"github.com/danielmalka/agendador-go/internal/scheduling"
	"github.com/danielmalka/agendador-go/internal/scheduling/mock"
)

func TestStartRegistersStartsAndStops(t *testing.T) {
	obs := &mock.Observer{}
	sched := &mock.Scheduler{}
	runner := &mock.Runner{Res: scheduling.Result{ExitCode: 0}}
	loader := &mock.Loader{Jobs: []scheduling.Job{{Name: "a"}, {Name: "b"}}}
	uc := scheduling.NewService(loader, runner, sched, obs)

	if err := uc.Start(context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	if len(sched.Fns) != 2 || !sched.Started || sched.Stopped {
		t.Fatalf("sched = %+v", sched)
	}
	ee := obs.Events()
	if len(ee) != 3 || ee[0].Kind != scheduling.EventJobRegistered || ee[2].Kind != scheduling.EventSchedulerStarted || ee[2].JobCount != 2 {
		t.Fatalf("unexpected start events: %+v", ee)
	}

	sched.Fns["a"](context.Background())
	if len(runner.Ran) != 1 || runner.Ran[0] != "a" {
		t.Fatalf("ran = %v", runner.Ran)
	}
	ee = obs.Events()
	if len(ee) != 5 || ee[3].Kind != scheduling.EventJobStarted || ee[4].Kind != scheduling.EventJobFinished {
		t.Fatalf("unexpected run events: %+v", ee)
	}

	stopCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := uc.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if !sched.Stopped || sched.StopCtx.Err() == nil {
		t.Fatalf("sched stop not propagated: %+v", sched)
	}
}

func TestJobFailureIsObserved(t *testing.T) {
	obs := &mock.Observer{}
	sched := &mock.Scheduler{}
	boom := errors.New("exit status 3")
	runner := &mock.Runner{Res: scheduling.Result{ExitCode: 3, Stderr: "boom"}, Err: boom}
	uc := scheduling.NewService(&mock.Loader{Jobs: []scheduling.Job{{Name: "a"}}}, runner, sched, obs)
	if err := uc.Start(context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	sched.Fns["a"](context.Background())
	ee := obs.Events()
	last := ee[len(ee)-1]
	if last.Kind != scheduling.EventJobFailed || !errors.Is(last.Err, boom) || last.Result.ExitCode != 3 || last.Result.Stderr != "boom" {
		t.Fatalf("unexpected failed event: %+v", last)
	}
}

func TestNilObserverIsSafe(t *testing.T) {
	sched := &mock.Scheduler{}
	uc := scheduling.NewService(&mock.Loader{Jobs: []scheduling.Job{{Name: "a"}}}, &mock.Runner{}, sched, nil)
	if err := uc.Start(context.Background(), "x.yml"); err != nil {
		t.Fatal(err)
	}
	sched.Fns["a"](context.Background())
}

func TestLifecycleErrors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name   string
		loader *mock.Loader
		sched  *mock.Scheduler
		stop   bool
	}{
		{"load error", &mock.Loader{Err: boom}, &mock.Scheduler{}, false},
		{"add error", &mock.Loader{Jobs: []scheduling.Job{{Name: "a"}}}, &mock.Scheduler{AddErr: boom}, false},
		{"start error", &mock.Loader{Jobs: []scheduling.Job{{Name: "a"}}}, &mock.Scheduler{StartErr: boom}, false},
		{"stop error", &mock.Loader{Jobs: []scheduling.Job{{Name: "a"}}}, &mock.Scheduler{StopErr: boom}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := scheduling.NewService(tt.loader, &mock.Runner{}, tt.sched, &mock.Observer{})
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
			if tt.sched.Started && (tt.name == "load error" || tt.name == "add error") {
				t.Error("scheduler must not start after a setup error")
			}
		})
	}
}
