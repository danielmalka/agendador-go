// Package config loads job definitions from YAML files.
package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/danielmalka/agendador-go/internal/scheduling"
)

// YAMLLoader implements scheduling.JobLoader.
type YAMLLoader struct {
	validateSchedule scheduling.ScheduleValidator
}

var _ scheduling.JobLoader = (*YAMLLoader)(nil)

// NewYAMLLoader builds a loader that validates schedules with the given validator.
func NewYAMLLoader(validate scheduling.ScheduleValidator) *YAMLLoader {
	return &YAMLLoader{validateSchedule: validate}
}

type fileDTO struct {
	Jobs []jobDTO `yaml:"jobs"`
}

type jobDTO struct {
	Name        string            `yaml:"name"`
	Schedule    string            `yaml:"schedule"`
	Type        string            `yaml:"type"`
	Command     string            `yaml:"command"`
	Args        []string          `yaml:"args"`
	Interpreter string            `yaml:"interpreter"`
	Timeout     string            `yaml:"timeout"`
	Env         map[string]string `yaml:"env"`
	WorkDir     string            `yaml:"workdir"`
}

// Load reads path and returns the validated jobs.
func (l *YAMLLoader) Load(ctx context.Context, path string) ([]scheduling.Job, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var file fileDTO
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(file.Jobs) == 0 {
		return nil, fmt.Errorf("%s: no jobs defined", path)
	}

	seen := make(map[string]bool, len(file.Jobs))
	jj := make([]scheduling.Job, 0, len(file.Jobs))
	for i, dto := range file.Jobs {
		label := dto.Name
		if label == "" {
			label = fmt.Sprintf("#%d", i+1)
		}
		j, err := l.toJob(ctx, dto)
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", label, err)
		}
		if seen[j.Name] {
			return nil, fmt.Errorf("job %q: %w", j.Name, scheduling.ErrDuplicateJob)
		}
		seen[j.Name] = true
		jj = append(jj, j)
	}
	return jj, nil
}

func (l *YAMLLoader) toJob(ctx context.Context, dto jobDTO) (scheduling.Job, error) {
	j := scheduling.Job{
		Name:        dto.Name,
		Schedule:    scheduling.Schedule(dto.Schedule),
		Type:        scheduling.JobType(dto.Type),
		Command:     dto.Command,
		Args:        dto.Args,
		Interpreter: dto.Interpreter,
		Env:         dto.Env,
		WorkDir:     dto.WorkDir,
	}
	if dto.Timeout != "" {
		d, err := time.ParseDuration(dto.Timeout)
		if err != nil {
			return scheduling.Job{}, fmt.Errorf("invalid timeout: %w", err)
		}
		j.Timeout = d
	}
	if err := j.Validate(); err != nil {
		return scheduling.Job{}, err
	}
	if err := l.validateSchedule.Validate(ctx, j.Schedule); err != nil {
		return scheduling.Job{}, fmt.Errorf("invalid schedule %q: %w", j.Schedule, err)
	}
	return j, nil
}
