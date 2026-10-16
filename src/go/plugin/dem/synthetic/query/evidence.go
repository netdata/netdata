// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
)

type Artifact struct {
	synthetic.Artifact
	Availability string
}

type Evidence struct {
	JobID        string
	RunID        string
	CaptureState string
	Artifacts    []Artifact
	Body         []byte
}

// Evidence reads membership from the retained manifest. History can explain a
// genuinely absent capture set, but cannot authorize bytes or override ownership.
func (s *Service) Evidence(ctx context.Context, jobID, runID, artifactID string) (Evidence, error) {
	if err := ctx.Err(); err != nil {
		return Evidence{}, err
	}
	if s.artifacts == nil {
		return Evidence{}, errors.New("artifact store unavailable")
	}
	manifest, err := s.artifacts.Manifest(ctx, jobID, runID)
	if errors.Is(err, artifacts.ErrNotFound) {
		return s.absentEvidence(ctx, jobID, runID, artifactID)
	}
	if err != nil {
		return Evidence{}, artifactError(err)
	}
	result := Evidence{
		JobID:        manifest.JobID,
		RunID:        manifest.RunID,
		CaptureState: "available",
	}
	if artifactID == "" {
		for _, item := range manifest.Artifacts {
			result.Artifacts = append(result.Artifacts, Artifact{
				Artifact:     item,
				Availability: "available",
			})
		}
		return result, nil
	}
	recorded := false
	for _, item := range manifest.Artifacts {
		if item.ID == artifactID {
			recorded = true
			break
		}
	}
	if !recorded {
		return result, ErrArtifactNotRecorded
	}
	metadata, body, err := s.artifacts.Fetch(ctx, jobID, runID, artifactID)
	if errors.Is(err, artifacts.ErrNotFound) {
		result.CaptureState = "expired_or_unavailable"
		return result, ErrExpired
	}
	if err != nil {
		return result, artifactError(err)
	}
	result.Artifacts = []Artifact{{Artifact: metadata, Availability: "available"}}
	result.Body = body
	return result, nil
}

func (s *Service) absentEvidence(ctx context.Context, jobID, runID, artifactID string) (Evidence, error) {
	// Active observations remain useful when history publication failed.
	run, err := s.Run(ctx, jobID, runID)
	if err != nil {
		if s.history == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return Evidence{}, ErrNotFound
		}
		return Evidence{}, err
	}
	result := Evidence{
		JobID:        run.JobID,
		RunID:        run.ID,
		CaptureState: run.CaptureState,
	}
	recorded := false
	for _, item := range run.Artifacts {
		recorded = recorded || item.ID == artifactID
		result.Artifacts = append(result.Artifacts, Artifact{
			Artifact:     item,
			Availability: "expired_or_unavailable",
		})
	}
	if len(run.Artifacts) > 0 {
		result.CaptureState = "expired_or_unavailable"
	}
	if artifactID != "" {
		if !recorded {
			return result, ErrArtifactNotRecorded
		}
		return result, ErrExpired
	}
	return result, nil
}
