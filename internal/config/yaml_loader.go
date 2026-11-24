// Package config loads job definitions from YAML files.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/danielmalka/agendador-go/internal/domain"
)

// ScheduleValidator checks a schedule expression; injected to keep the loader
// independent of the cron library.
type ScheduleValidator func(spec string) error

// YAMLLoader implements domain.ConfigLoader.
type YAMLLoader struct {
	validateSchedule ScheduleValidator
}

var _ domain.ConfigLoader = (*YAMLLoader)(nil)

// NewYAMLLoader builds a loader that validates schedules with the given function.
func NewYAMLLoader(validate ScheduleValidator) *YAMLLoader {
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
func (l *YAMLLoader) Load(path string) ([]domain.Job, error) {
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
	jobs := make([]domain.Job, 0, len(file.Jobs))
	for i, dto := range file.Jobs {
		label := dto.Name
		if label == "" {
			label = fmt.Sprintf("#%d", i+1)
		}
		job, err := l.toJob(dto)
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", label, err)
		}
		if seen[job.Name] {
			return nil, fmt.Errorf("job %q: duplicate name", job.Name)
		}
		seen[job.Name] = true
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (l *YAMLLoader) toJob(dto jobDTO) (domain.Job, error) {
	job := domain.Job{
		Name:        dto.Name,
		Schedule:    dto.Schedule,
		Type:        domain.JobType(dto.Type),
		Command:     dto.Command,
		Args:        dto.Args,
		Interpreter: dto.Interpreter,
		Env:         dto.Env,
		WorkDir:     dto.WorkDir,
	}
	if dto.Timeout != "" {
		d, err := time.ParseDuration(dto.Timeout)
		if err != nil {
			return domain.Job{}, fmt.Errorf("invalid timeout: %w", err)
		}
		job.Timeout = d
	}
	if err := job.Validate(); err != nil {
		return domain.Job{}, err
	}
	if err := l.validateSchedule(job.Schedule); err != nil {
		return domain.Job{}, fmt.Errorf("invalid schedule %q: %w", job.Schedule, err)
	}
	return job, nil
}
