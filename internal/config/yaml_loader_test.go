package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielmalka/agendador-go/internal/scheduling"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "jobs.yml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var okSchedule = scheduling.ScheduleValidatorFunc(func(_ context.Context, spec scheduling.Schedule) error {
	if strings.HasPrefix(string(spec), "bad") {
		return errors.New("bad spec")
	}
	return nil
})

func TestLoadValid(t *testing.T) {
	p := writeYAML(t, `
jobs:
  - name: a
    schedule: "@daily"
    type: python
    command: x.py
    args: ["--n", "1"]
    interpreter: python3.12
    timeout: 5m
    env: {K: V}
    workdir: /tmp
  - name: b
    schedule: "* * * * *"
    type: command
    command: echo
`)
	jj, err := NewYAMLLoader(okSchedule).Load(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	want := scheduling.Job{Name: "a", Schedule: "@daily", Type: scheduling.TypePython, Command: "x.py",
		Args: []string{"--n", "1"}, Interpreter: "python3.12", Timeout: 5 * time.Minute,
		Env: map[string]string{"K": "V"}, WorkDir: "/tmp"}
	if len(jj) != 2 || !reflect.DeepEqual(jj[0], want) || jj[1].Name != "b" {
		t.Fatalf("unexpected jobs: %+v", jj)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name, yaml, wantErr string
	}{
		{"bad schedule", "jobs:\n - {name: j1, schedule: bad, type: command, command: echo}", `job "j1": invalid schedule`},
		{"duplicate name", "jobs:\n - {name: j1, schedule: '@daily', type: command, command: echo}\n - {name: j1, schedule: '@daily', type: command, command: echo}", `job "j1": duplicate`},
		{"unknown type", "jobs:\n - {name: j1, schedule: '@daily', type: ruby, command: echo}", `job "j1": unknown type`},
		{"empty command", "jobs:\n - {name: j1, schedule: '@daily', type: command}", `job "j1": command is required`},
		{"bad timeout", "jobs:\n - {name: j1, schedule: '@daily', type: command, command: echo, timeout: soon}", `job "j1": invalid timeout`},
		{"missing name", "jobs:\n - {schedule: '@daily', type: command, command: echo}", `job "#1": name is required`},
		{"unknown field", "jobs:\n - {name: j1, shedule: x}", "parse"},
		{"malformed yaml", "jobs: [", "parse"},
		{"empty file", "", "no jobs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewYAMLLoader(okSchedule).Load(context.Background(), writeYAML(t, tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := NewYAMLLoader(okSchedule).Load(context.Background(), filepath.Join(t.TempDir(), "nope.yml")); err == nil {
		t.Fatal("want error for missing file")
	}
}
