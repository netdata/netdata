// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
)

// eligibleCaptures splits reported candidates into those to publish and those
// to explain: interrupted captures publish only after a journey budget stop.
func eligibleCaptures(run *synthetic.Run, s sinkSnapshot) (captures, ineligible []synthetic.Capture) {
	if s.err != nil || s.result == nil {
		return nil, nil
	}
	captures = append(captures, s.result.Artifacts...)
	if run.TerminationReason == synthetic.ExecutionBudget {
		return append(captures, s.result.InterruptedArtifacts...), nil
	}
	return captures, s.result.InterruptedArtifacts
}

// joinCaptures adds labels only to already admitted terminal candidates. Metadata
// IDs absent from that authority remain explanations, never offered files.
func joinCaptures(run *synthetic.Run, candidates, ineligible []synthetic.Capture) {
	if run.CaptureState == synthetic.CaptureDisabled {
		return
	}
	if run.Captures == nil {
		run.Captures = &synthetic.CaptureReport{}
	}
	tests := make(map[string]synthetic.ArtifactTest, len(run.Captures.Tests))
	for _, test := range run.Captures.Tests {
		tests[test.ID] = test.ArtifactTest
	}
	associations := make(map[string]synthetic.ArtifactTest, len(run.Captures.Associations))
	for _, a := range run.Captures.Associations {
		associations[a.ID] = tests[a.TestID]
	}
	known := make(map[string]bool, len(candidates)+len(ineligible))
	for i := range candidates {
		known[candidates[i].ID] = true
		candidates[i].Test = associations[candidates[i].ID]
	}
	for _, candidate := range ineligible {
		known[candidate.ID] = true
		run.Captures.Unpublished = append(
			run.Captures.Unpublished,
			synthetic.CaptureOmission{
				ID:     candidate.ID,
				Reason: "not_eligible",
			},
		)
	}
	for _, a := range run.Captures.Associations {
		if !known[a.ID] {
			run.Captures.Unpublished = append(
				run.Captures.Unpublished,
				synthetic.CaptureOmission{
					ID:     a.ID,
					Reason: "not_in_final_report",
				},
			)
		}
	}
}

// publish finalizes the run work directory with the eligible captures and
// records the result: published artifacts, omission reasons and the capture state.
func (e *Engine) publish(ctx context.Context, a *attempt, captures []synthetic.Capture) {
	run := &a.run
	artifacts, err := e.artifacts.Finalize(ctx, run.ID, captures)
	if err != nil {
		capturePublicationFailed(run, captures, err)
		run.Error = joinText(run.Error, a.redact.text("artifact publication: "+err.Error()))
		if run.CaptureState != synthetic.CaptureDisabled {
			run.CaptureState = synthetic.CaptureUnavailable
		}
		return
	}
	run.Artifacts = artifacts
	capturePublicationSucceeded(run, captures)
	if run.CaptureState != synthetic.CaptureDisabled && len(captures) > 0 {
		run.CaptureState = synthetic.CaptureUnavailable
		if len(artifacts) > 0 {
			run.CaptureState = synthetic.CaptureAvailable
		}
	}
}

func capturePublicationSucceeded(run *synthetic.Run, candidates []synthetic.Capture) {
	if run.Captures == nil || len(candidates) == len(run.Artifacts) {
		return
	}
	// Successful Finalize omits only candidates above its initial-size limit.
	published := make(map[string]bool, len(run.Artifacts))
	for _, a := range run.Artifacts {
		published[a.ID] = true
	}
	for _, c := range candidates {
		if !published[c.ID] {
			run.Captures.Unpublished = append(
				run.Captures.Unpublished,
				synthetic.CaptureOmission{
					ID:     c.ID,
					Reason: "too_large",
				},
			)
		}
	}
}

func capturePublicationFailed(run *synthetic.Run, candidates []synthetic.Capture, err error) {
	if run.Captures == nil {
		return
	}
	var failure *artifacts.CaptureError
	tooLarge := errors.As(err, &failure) && errors.Is(failure, artifacts.ErrTooLarge)
	for _, c := range candidates {
		reason := "publication_failed"
		if tooLarge && failure.ID == c.ID {
			reason = "too_large"
		}
		run.Captures.Unpublished = append(run.Captures.Unpublished, synthetic.CaptureOmission{
			ID:     c.ID,
			Reason: reason,
		})
	}
}
