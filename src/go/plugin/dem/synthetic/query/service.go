// SPDX-License-Identifier: GPL-3.0-or-later

// Package query combines active synthetic observations with retained evidence.
// Reads never acquire browser admission or extend an active job's lifetime.
package query

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
)

var (
	ErrNotFound            = errors.New("requested evidence is unavailable")
	ErrArtifactNotRecorded = errors.New("artifact is not recorded for this run")
	ErrExpired             = errors.New("recorded artifact is expired or unavailable")
	ErrTooLarge            = errors.New("artifact exceeds the fetch limit")
	ErrInvalidID           = errors.New("invalid run or artifact identifier")
)

type Job registry.Job
type RunFilter history.RunFilter
type RunPage history.RunPage
type ArtifactStats artifacts.Stats

type Observations interface {
	Snapshot(time.Time) []registry.Job
}
type History interface {
	QueryRuns(context.Context, history.RunFilter) (history.RunPage, error)
	GetRun(context.Context, string, string) (synthetic.Run, error)
}
type Artifacts interface {
	Stats() artifacts.Stats
	Manifest(context.Context, string) ([]synthetic.Artifact, error)
	Fetch(context.Context, string, string) (synthetic.Artifact, []byte, error)
}

type Service struct {
	active    Observations
	history   History
	artifacts Artifacts
}

func New(active Observations, history History, artifacts Artifacts) *Service {
	return &Service{
		active:    active,
		history:   history,
		artifacts: artifacts,
	}
}

func (s *Service) Jobs(now time.Time) []Job {
	if s.active == nil {
		return nil
	}
	snapshots := s.active.Snapshot(now)
	out := make([]Job, len(snapshots))
	for i, job := range snapshots {
		out[i] = Job(job)
	}
	return out
}

func (s *Service) ArtifactStats() ArtifactStats {
	if s.artifacts == nil {
		return ArtifactStats{}
	}
	return ArtifactStats(s.artifacts.Stats())
}

func (s *Service) Runs(ctx context.Context, filter RunFilter) (RunPage, error) {
	if err := ctx.Err(); err != nil {
		return RunPage{}, err
	}
	if s.history == nil {
		return RunPage{}, errors.New("history unavailable")
	}
	page, err := s.history.QueryRuns(ctx, history.RunFilter(filter))
	return RunPage(page), queryError(err)
}

// Run prefers the exact active observation so failed journal publication does
// not hide current diagnosis. Retired jobs remain readable from history.
func (s *Service) Run(ctx context.Context, jobID, runID string) (synthetic.Run, error) {
	if err := ctx.Err(); err != nil {
		return synthetic.Run{}, err
	}
	for _, job := range s.Jobs(time.Now()) {
		if job.JobID == jobID && job.Latest != nil && job.Latest.ID == runID {
			return *job.Latest, nil
		}
	}
	if s.history == nil {
		return synthetic.Run{}, errors.New("history unavailable")
	}
	run, err := s.history.GetRun(ctx, jobID, runID)
	return run, queryError(err)
}

func queryError(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, artifacts.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, artifacts.ErrTooLarge):
		return ErrTooLarge
	case errors.Is(err, artifacts.ErrInvalidID):
		return ErrInvalidID
	default:
		return err
	}
}
