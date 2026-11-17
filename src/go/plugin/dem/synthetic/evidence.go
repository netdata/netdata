// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"fmt"
	"slices"
	"strings"
)

// FailureEvidence contains optional, bounded reporter observations. It never
// determines execution outcome. A nil pointer means evidence was not recorded.
type FailureEvidence struct {
	FeatureStatus
	Truncated    []string       `json:"truncated,omitempty"`
	Tests        []EvidenceTest `json:"tests,omitempty"`
	Steps        []EvidenceStep `json:"steps,omitempty"`
	FirstFailure *FailureDetail `json:"first_failure,omitempty"`
	StopContext  *FailureDetail `json:"stop_context,omitempty"`
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
	out.Truncated = slices.Clone(in.Truncated)
	out.Tests = slices.Clone(in.Tests)
	for i := range out.Tests {
		cloneEvidenceTest(&out.Tests[i])
	}
	out.Steps = slices.Clone(in.Steps)
	for i := range out.Steps {
		cloneEvidenceStep(&out.Steps[i])
	}
	out.FirstFailure = cloneFailureDetail(in.FirstFailure)
	out.StopContext = cloneFailureDetail(in.StopContext)
	return &out
}

func cloneFailureDetail(in *FailureDetail) *FailureDetail {
	if in == nil {
		return nil
	}
	out := *in
	cloneEvidenceTest(&out.Test)
	out.Location = clonePtr(in.Location)
	out.Path = slices.Clone(in.Path)
	for i := range out.Path {
		cloneEvidenceStep(&out.Path[i])
	}
	return &out
}

func cloneEvidenceTest(test *EvidenceTest) {
	test.Location = clonePtr(test.Location)
	test.DurationMS = clonePtr(test.DurationMS)
}

func cloneEvidenceStep(step *EvidenceStep) {
	step.Location = clonePtr(step.Location)
	step.DurationMS = clonePtr(step.DurationMS)
}

// ReportingStatus is NotRecorded for absent evidence, else its FeatureStatus.
func (e *FailureEvidence) ReportingStatus() string {
	if e == nil {
		return NotRecorded
	}
	return e.FeatureStatus.ReportingStatus()
}

// FailureSummary is shared by the retained-run Function and synchronous rehearsal.
func (e *FailureEvidence) FailureSummary() string {
	if e == nil {
		return ""
	}
	return e.detailSummary(e.FirstFailure)
}

// StopSummary describes pending activity observed before cleanup, not the cause
// or exact location of a timeout. It is independent of the first failure.
func (e *FailureEvidence) StopSummary() string {
	if e == nil {
		return ""
	}
	return e.detailSummary(e.StopContext)
}

func (e *FailureEvidence) detailSummary(f *FailureDetail) string {
	if f == nil {
		return ""
	}
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
	if e.Availability == FeatureInvalid {
		summary += " [partial: evidence invalid]"
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
		slices.Reverse(path)
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
		if e.Availability == FeatureInvalid {
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
