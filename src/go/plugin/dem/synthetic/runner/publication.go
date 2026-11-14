// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
)

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
		run.Captures.Unpublished = append(run.Captures.Unpublished, synthetic.CaptureOmission{ID: candidate.ID, Reason: "not_eligible"})
	}
	for _, a := range run.Captures.Associations {
		if !known[a.ID] {
			run.Captures.Unpublished = append(run.Captures.Unpublished, synthetic.CaptureOmission{ID: a.ID, Reason: "not_in_final_report"})
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
			run.Captures.Unpublished = append(run.Captures.Unpublished, synthetic.CaptureOmission{ID: c.ID, Reason: "too_large"})
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
		run.Captures.Unpublished = append(run.Captures.Unpublished, synthetic.CaptureOmission{ID: c.ID, Reason: reason})
	}
}
