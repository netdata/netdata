// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic

import "slices"

// CaptureReport describes observations and publication decisions, never file authority.
// Reporting applies to journey metadata; an absent value means it was not recorded.
// Trace describes requested runtime support and never grants capture authority.
// Truncated marks omitted Tests/Associations detail due to the shared record budget
// or an unrecordable test ID. It does not describe publication or limit Unpublished.
type CaptureReport struct {
	Trace        string               `json:"trace,omitempty"`
	Reporting    string               `json:"reporting,omitempty"`
	Truncated    bool                 `json:"truncated,omitempty"`
	Tests        []CaptureTest        `json:"tests,omitempty"`
	Associations []CaptureAssociation `json:"associations,omitempty"`
	Unpublished  []CaptureOmission    `json:"unpublished,omitempty"`
}

type CaptureTest struct {
	ArtifactTest
	Status         string `json:"status"`
	ExpectedStatus string `json:"expected_status"`
	Accepted       int64  `json:"accepted"`
	Unaccepted     int64  `json:"unaccepted"`
}

type CaptureAssociation struct {
	ID     string `json:"id"`
	TestID string `json:"test_id"`
}

type CaptureOmission struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

func CloneCaptureReport(in *CaptureReport) *CaptureReport {
	if in == nil {
		return nil
	}
	out := *in
	out.Tests = slices.Clone(in.Tests)
	out.Associations = slices.Clone(in.Associations)
	out.Unpublished = slices.Clone(in.Unpublished)
	return &out
}
