// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"context"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/dem/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"time"
)

// Process Functions copy current observations before doing independent disk work.
type syntheticSource struct {
	hub       *synthetic.Hub
	history   *store.Store
	artifacts *artifacts.Store
}

func (s *syntheticSource) Snapshot(now time.Time) []synthetic.Job { return s.hub.Snapshot(now) }
func (s *syntheticSource) QuerySyntheticRuns(ctx context.Context, f synthetic.RunFilter) (synthetic.RunPage, error) {
	if s.history == nil {
		return synthetic.RunPage{}, errors.New("history unavailable")
	}
	return s.history.QuerySyntheticRuns(ctx, f)
}
func (s *syntheticSource) GetSyntheticRun(ctx context.Context, jobID, runID string) (synthetic.Run, error) {
	if s.history == nil {
		return synthetic.Run{}, errors.New("history unavailable")
	}
	return s.history.GetSyntheticRun(ctx, jobID, runID)
}
func (s *syntheticSource) Manifest(ctx context.Context, runID string) ([]synthetic.Artifact, error) {
	if s.artifacts == nil {
		return nil, errors.New("artifact store unavailable")
	}
	return s.artifacts.Manifest(ctx, runID)
}
func (s *syntheticSource) Fetch(ctx context.Context, runID, artifactID string) (synthetic.Artifact, []byte, error) {
	if s.artifacts == nil {
		return synthetic.Artifact{}, nil, errors.New("artifact store unavailable")
	}
	return s.artifacts.Fetch(ctx, runID, artifactID)
}
func (s *syntheticSource) ArtifactStats() artifacts.Stats {
	if s.artifacts == nil {
		return artifacts.Stats{}
	}
	return s.artifacts.Stats()
}
