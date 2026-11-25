package scheduling

import (
	"errors"
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
		wantErr error
	}{
		{"valid", func(*Job) {}, nil},
		{"no name", func(j *Job) { j.Name = "" }, ErrEmptyName},
		{"no schedule", func(j *Job) { j.Schedule = "" }, ErrEmptySchedule},
		{"no command", func(j *Job) { j.Command = "" }, ErrEmptyCommand},
		{"bad type", func(j *Job) { j.Type = "ruby" }, ErrInvalidJobType},
		{"empty type", func(j *Job) { j.Type = "" }, ErrInvalidJobType},
		{"negative timeout", func(j *Job) { j.Timeout = -time.Second }, ErrNegativeTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := validJob()
			tt.mutate(&j)
			err := j.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInvalidTypeMessage(t *testing.T) {
	j := validJob()
	j.Type = "ruby"
	if err := j.Validate(); err == nil || !strings.Contains(err.Error(), `unknown type "ruby"`) {
		t.Fatalf("err = %v", err)
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
