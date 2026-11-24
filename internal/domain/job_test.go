package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func validJob() Job {
	return Job{Name: "n", Schedule: "@daily", Type: TypeCommand, Command: "echo"}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Job)
		wantErr string
	}{
		{"valid", func(*Job) {}, ""},
		{"no name", func(j *Job) { j.Name = "" }, "name"},
		{"no schedule", func(j *Job) { j.Schedule = "" }, "schedule"},
		{"no command", func(j *Job) { j.Command = "" }, "command"},
		{"bad type", func(j *Job) { j.Type = "ruby" }, "unknown type"},
		{"empty type", func(j *Job) { j.Type = "" }, "unknown type"},
		{"negative timeout", func(j *Job) { j.Timeout = -time.Second }, "timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := validJob()
			tt.mutate(&j)
			err := j.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestArgv(t *testing.T) {
	tests := []struct {
		name     string
		job      Job
		wantBin  string
		wantArgs []string
	}{
		{"python default interpreter", Job{Type: TypePython, Command: "a.py", Args: []string{"x"}}, "python3", []string{"a.py", "x"}},
		{"python custom interpreter", Job{Type: TypePython, Interpreter: "python3.12", Command: "a.py"}, "python3.12", []string{"a.py"}},
		{"command", Job{Type: TypeCommand, Command: "ls", Args: []string{"-l", "/"}}, "ls", []string{"-l", "/"}},
		{"command no args", Job{Type: TypeCommand, Command: "ls"}, "ls", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin, args := tt.job.Argv()
			if bin != tt.wantBin || !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("got %q %v, want %q %v", bin, args, tt.wantBin, tt.wantArgs)
			}
		})
	}
}
