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

// Evidence resolves recorded metadata against retained files. A supplied ID
// must belong to the exact run before the artifact store is allowed to read it.
func (s *Service) Evidence(ctx context.Context, jobID, runID, artifactID string) (Evidence, error) {
	run, err := s.Run(ctx, jobID, runID)
	if err != nil {
		return Evidence{}, err
	}
	result := Evidence{
		JobID:        run.JobID,
		RunID:        run.ID,
		CaptureState: run.CaptureState,
	}
	if artifactID != "" {
		recorded := false
		for _, artifact := range run.Artifacts {
			if artifact.ID == artifactID {
				recorded = true
				break
			}
		}
		if !recorded {
			return result, ErrArtifactNotRecorded
		}
		if s.artifacts == nil {
			return result, errors.New("artifact store unavailable")
		}
		metadata, body, err := s.artifacts.Fetch(ctx, run.ID, artifactID)
		if errors.Is(err, artifacts.ErrNotFound) {
			result.CaptureState = "expired_or_unavailable"
			return result, ErrExpired
		}
		if err != nil {
			return result, queryError(err)
		}
		result.CaptureState = "available"
		result.Artifacts = []Artifact{{Artifact: metadata, Availability: "available"}}
		result.Body = body
		return result, nil
	}
	if len(run.Artifacts) == 0 {
		return result, nil
	}
	if s.artifacts == nil {
		return result, errors.New("artifact store unavailable")
	}
	manifest, err := s.artifacts.Manifest(ctx, run.ID)
	if err != nil && !errors.Is(err, artifacts.ErrNotFound) {
		return result, queryError(err)
	}
	available := make(map[string]bool, len(manifest))
	for _, item := range manifest {
		available[item.ID] = true
	}
	for _, item := range run.Artifacts {
		state := "expired_or_unavailable"
		if available[item.ID] {
			state = "available"
		}
		result.Artifacts = append(result.Artifacts, Artifact{
			Artifact:     item,
			Availability: state,
		})
	}
	if errors.Is(err, artifacts.ErrNotFound) {
		result.CaptureState = "expired_or_unavailable"
	}
	return result, nil
}
