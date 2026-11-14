// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic

import "slices"

// CaptureKind names a capture file type; each kind has exactly one MIME type.
type CaptureKind string

const (
	ScreenshotCapture CaptureKind = "screenshot" // journey failure screenshot
	TraceCapture      CaptureKind = "trace"      // journey Playwright trace
	ReportCapture     CaptureKind = "report"     // Lighthouse HTML report
)

// MIME returns the media type of a kind, or "" for an unknown kind.
func (k CaptureKind) MIME() string {
	switch k {
	case ScreenshotCapture:
		return "image/png"
	case TraceCapture:
		return "application/zip"
	case ReportCapture:
		return "text/html"
	}
	return ""
}

// CaptureState says whether a run published captures.
type CaptureState string

const (
	CaptureDisabled    CaptureState = "disabled"   // capture was not requested
	CaptureNotNeeded   CaptureState = "not_needed" // the runner had nothing to capture
	CaptureUnavailable CaptureState = "unavailable"
	CaptureAvailable   CaptureState = "available"
	// CaptureExpired is reported when reading evidence whose recorded captures
	// are no longer retained; history never records it.
	CaptureExpired CaptureState = "expired_or_unavailable"
)

// Capture is a candidate file the runner reports under the run's output
// directory. The artifact store validates its location and computes its size
// and digest before publishing it.
type Capture struct {
	ID   string      `json:"id"`
	Kind CaptureKind `json:"kind"`
	Path string      `json:"path"`
	MIME string      `json:"mime"`
	// Test is joined from capture details after core validation; the runner
	// never reports it.
	Test ArtifactTest `json:"-"`
}

type Artifact struct {
	ID     string       `json:"id"`
	Kind   CaptureKind  `json:"kind"`
	MIME   string       `json:"mime"`
	Bytes  int64        `json:"bytes"`
	SHA256 string       `json:"sha256"`
	Test   ArtifactTest `json:"test,omitzero"`
}

type ArtifactTest struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	TextTruncated bool   `json:"text_truncated,omitempty"`
}

// CaptureReport explains capture observations and publication decisions; it
// never authorizes a file. Reporting (journeys) and Trace (when requested)
// hold optional-output status; an absent value means not recorded. Truncated
// marks Tests or Associations omitted by the record budget or an unrecordable
// test ID; it does not limit Unpublished.
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
