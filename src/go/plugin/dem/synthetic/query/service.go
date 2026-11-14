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

type Observations interface {
	Snapshot(time.Time) []registry.Job
	Latest(jobID, runID string) (synthetic.Run, bool)
}
type History interface {
	QueryRuns(context.Context, history.RunFilter) (history.RunPage, error)
	GetRun(context.Context, string, string) (synthetic.Run, error)
}
type Artifacts interface {
	Catalogue(context.Context, artifacts.Filter) (artifacts.Page, error)
	Manifest(context.Context, string, string) (artifacts.Manifest, error)
	Fetch(context.Context, string, string, string) (synthetic.Artifact, []byte, error)
}

type Service struct {
	active      Observations
	history     History
	artifacts   Artifacts
	policyError func() string
}

func New(active Observations, history History, artifacts Artifacts, policyError func() string) *Service {
	return &Service{
		active:      active,
		history:     history,
		artifacts:   artifacts,
		policyError: policyError,
	}
}

func (s *Service) Jobs(now time.Time) []registry.Job {
	return s.active.Snapshot(now)
}

func (s *Service) Runs(ctx context.Context, filter history.RunFilter) (history.RunPage, error) {
	if err := ctx.Err(); err != nil {
		return history.RunPage{}, err
	}
	page, err := s.history.QueryRuns(ctx, filter)
	return page, queryError(err)
}

// Run prefers the exact active observation so failed journal publication does
// not hide current diagnosis. Retired jobs remain readable from history.
func (s *Service) Run(ctx context.Context, jobID, runID string) (synthetic.Run, error) {
	if err := ctx.Err(); err != nil {
		return synthetic.Run{}, err
	}
	if run, ok := s.active.Latest(jobID, runID); ok {
		return run, nil
	}
	run, err := s.history.GetRun(ctx, jobID, runID)
	return run, queryError(err)
}

func queryError(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, artifacts.ErrNotFound):
		return ErrNotFound
	default:
		return artifactError(err)
	}
}

func artifactError(err error) error {
	switch {
	case errors.Is(err, artifacts.ErrNotFound), errors.Is(err, artifacts.ErrOwnerMismatch):
		return ErrNotFound
	case errors.Is(err, artifacts.ErrTooLarge):
		return ErrTooLarge
	case errors.Is(err, artifacts.ErrInvalidID):
		return ErrInvalidID
	default:
		return err
	}
}
