// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"fmt"
	"strings"
)

// FailureEvidence contains optional, bounded reporter observations. It never
// determines execution outcome. A nil pointer means evidence was not recorded.
type FailureEvidence struct {
	Availability     string         `json:"availability"`
	Reason           string         `json:"reason,omitempty"`
	ReporterComplete bool           `json:"reporter_complete"`
	Truncated        []string       `json:"truncated,omitempty"`
	Tests            []EvidenceTest `json:"tests,omitempty"`
	Steps            []EvidenceStep `json:"steps,omitempty"`
	FirstFailure     *FailureDetail `json:"first_failure,omitempty"`
}

type EvidenceTest struct {
	ID                string            `json:"id"`
	Title             string            `json:"title"`
	Location          *EvidenceLocation `json:"location,omitempty"`
	Status            string            `json:"status"`
	ExpectedStatus    string            `json:"expected_status"`
	DurationMS        *float64          `json:"duration_ms,omitempty"`
	ReportingComplete bool              `json:"reporting_complete"`
	TextTruncated     bool              `json:"text_truncated,omitempty"`
}

type EvidenceStep struct {
	ID            string            `json:"id"`
	TestID        string            `json:"test_id"`
	ParentID      string            `json:"parent_id,omitempty"`
	ParentOmitted bool              `json:"parent_omitted,omitempty"`
	Title         string            `json:"title"`
	Category      string            `json:"category"`
	Location      *EvidenceLocation `json:"location,omitempty"`
	State         string            `json:"state"`
	DurationMS    *float64          `json:"duration_ms,omitempty"`
	Message       string            `json:"message,omitempty"`
	TextTruncated bool              `json:"text_truncated,omitempty"`
}

type EvidenceLocation struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type FailureDetail struct {
	PathIncomplete   bool              `json:"path_incomplete,omitempty"`
	Test             EvidenceTest      `json:"test"`
	Path             []EvidenceStep    `json:"path"`
	OmittedAncestors int               `json:"omitted_ancestors"`
	Message          string            `json:"message"`
	Location         *EvidenceLocation `json:"location,omitempty"`
	TextTruncated    bool              `json:"text_truncated,omitempty"`
}

// CloneFailureEvidence isolates mutable evidence at snapshot boundaries.
func CloneFailureEvidence(in *FailureEvidence) *FailureEvidence {
	if in == nil {
		return nil
	}
	out := *in
	out.Truncated = append([]string(nil), in.Truncated...)
	out.Tests = append([]EvidenceTest(nil), in.Tests...)
	for i := range out.Tests {
		cloneEvidenceTest(&out.Tests[i])
	}
	out.Steps = append([]EvidenceStep(nil), in.Steps...)
	for i := range out.Steps {
		cloneEvidenceStep(&out.Steps[i])
	}
	if in.FirstFailure != nil {
		f := *in.FirstFailure
		cloneEvidenceTest(&f.Test)
		f.Location = cloneEvidenceLocation(f.Location)
		f.Path = append([]EvidenceStep(nil), f.Path...)
		for i := range f.Path {
			cloneEvidenceStep(&f.Path[i])
		}
		out.FirstFailure = &f
	}
	return &out
}

func cloneEvidenceLocation(in *EvidenceLocation) *EvidenceLocation {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
func cloneEvidenceTest(test *EvidenceTest) {
	test.Location = cloneEvidenceLocation(test.Location)
	if test.DurationMS != nil {
		duration := *test.DurationMS
		test.DurationMS = &duration
	}
}
func cloneEvidenceStep(step *EvidenceStep) {
	step.Location = cloneEvidenceLocation(step.Location)
	if step.DurationMS != nil {
		duration := *step.DurationMS
		step.DurationMS = &duration
	}
}

// ReportingStatus distinguishes missing or unavailable evidence from a partial
// reporter stream; truncation remains an independent property of the evidence.
func (e *FailureEvidence) ReportingStatus() string {
	if e == nil {
		return "not_recorded"
	}
	if e.Availability != "supported" {
		return e.Availability
	}
	if e.ReporterComplete {
		return "complete"
	}
	return "partial"
}

// FailureSummary is shared by the retained-run Function and synchronous rehearsal.
func (e *FailureEvidence) FailureSummary() string {
	if e == nil || e.FirstFailure == nil {
		return ""
	}
	f := e.FirstFailure
	title := f.Test.Title
	if f.Test.Status != "" && f.Test.ExpectedStatus != "" && f.Test.Status != f.Test.ExpectedStatus {
		title += fmt.Sprintf(" [%s; expected %s]", f.Test.Status, f.Test.ExpectedStatus)
	}
	parts := []string{title}
	for i, step := range f.Path {
		if step.ParentOmitted || (i == 0 && f.OmittedAncestors > 0) {
			parts = append(parts, "…")
		}
		parts = append(parts, step.Title)
	}
	summary := strings.Join(parts, " > ")
	location := f.Location
	if location == nil && len(f.Path) > 0 {
		location = f.Path[len(f.Path)-1].Location
	}
	if location == nil {
		location = f.Test.Location
	}
	if location != nil {
		summary += fmt.Sprintf(" (%s:%d:%d)", location.File, location.Line, location.Column)
	}
	if f.Message != "" {
		summary += ": " + f.Message
	}
	if f.PathIncomplete {
		summary += " [path incomplete]"
	}
	if f.TextTruncated {
		summary += " [text truncated]"
	}
	return summary
}

// LastObservedStepSummary reports the newest retained running step, not current
// browser truth. Concurrent running siblings are represented by retention order.
func (e *FailureEvidence) LastObservedStepSummary() string {
	if e == nil {
		return ""
	}
	for i := len(e.Steps) - 1; i >= 0; i-- {
		step := e.Steps[i]
		if step.State != "running" {
			continue
		}
		// Walk only retained entries, stopping at omissions, missing parents or
		// cycles. The wire retention bound also bounds this presentation work.
		path := []string{step.Title}
		seen := map[string]bool{step.ID: true}
		current := step
		for current.ParentID != "" || current.ParentOmitted {
			if current.ParentOmitted || seen[current.ParentID] {
				path = append(path, "…")
				break
			}
			var parent *EvidenceStep
			for j := range e.Steps {
				if e.Steps[j].ID == current.ParentID && e.Steps[j].TestID == step.TestID {
					parent = &e.Steps[j]
					break
				}
			}
			if parent == nil {
				path = append(path, "…")
				break
			}
			seen[parent.ID] = true
			path = append(path, parent.Title)
			current = *parent
		}
		for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
			path[left], path[right] = path[right], path[left]
		}
		for _, test := range e.Tests {
			if test.ID == step.TestID {
				path = append([]string{test.Title}, path...)
				break
			}
		}
		summary := strings.Join(path, " > ")
		if step.Location != nil {
			summary += fmt.Sprintf(" (%s:%d:%d)", step.Location.File, step.Location.Line, step.Location.Column)
		}
		if e.Availability == "invalid" {
			summary += " [partial: evidence invalid]"
		}
		for _, scope := range e.Truncated {
			if scope == "user_steps" || scope == "other_steps" {
				summary += " [partial: step detail truncated]"
				break
			}
		}
		return summary
	}
	return ""
}
