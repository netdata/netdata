// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// failure_evidence.v1 budgets, enforced by the producer
// (src/dem-runtime/runner/failure-evidence.cjs) and described in
// src/dem-runtime/README.md, "Feature budgets". They bound arbitrary user-script
// detail independently of core outcomes; separate pools reserve test and
// user-step detail against API noise. Retained data stays below 544 KiB.
const (
	evidenceTestLimit      = 64
	evidenceUserStepLimit  = 128
	evidenceOtherStepLimit = 64
	failurePathLimit       = 16 // first eight and last eight ancestors

	// Encoded JSON bytes, quotes included. Browser and capture details share
	// the test ID and title budgets.
	testIDBytes      = 128
	stepIDBytes      = 64
	titleBytes       = 256
	sourceBytes      = 256 // script-relative source file label
	categoryBytes    = 64
	stepMessageBytes = 1024
	stepBytes        = 2048 // one serialized step; field budgets alone allow more
	// A first-failure message is clipped to textLimit runes; at the widest JSON
	// escape (\u00XX) that encodes to this many bytes.
	failureMessageBytes = 6*textLimit + 2
)

// maxSafeInteger is JavaScript's Number.MAX_SAFE_INTEGER: the producer's counts
// and source positions are JavaScript numbers.
const maxSafeInteger = 1<<53 - 1

type evidenceState struct {
	featureState
	value      synthetic.FailureEvidence
	tests      map[string]int
	steps      map[string]int
	userSteps  int
	otherSteps int
}

func newEvidenceState() *evidenceState {
	return &evidenceState{tests: make(map[string]int), steps: make(map[string]int)}
}

func (e *evidenceState) truncate(scope string) {
	if !slices.Contains(e.value.Truncated, scope) {
		e.value.Truncated = append(e.value.Truncated, scope)
	}
}

// accept decodes one record of a selected, incomplete stream.
func (e *evidenceState) accept(raw json.RawMessage, redact *textRedactor) {
	var data struct {
		Record  string                   `json:"record"`
		Test    *synthetic.EvidenceTest  `json:"test"`
		Step    *synthetic.EvidenceStep  `json:"step"`
		Failure *synthetic.FailureDetail `json:"failure"`
		Stop    *synthetic.FailureDetail `json:"stop"`
		Scope   string                   `json:"scope"`
	}
	if json.Unmarshal(raw, &data) != nil {
		e.invalidate("invalid_record")
		return
	}
	valid := false
	switch data.Record {
	case "test":
		if data.Test != nil && data.Step == nil && data.Failure == nil && data.Stop == nil && data.Scope == "" && validEvidenceTest(*data.Test) {
			redactEvidenceTest(data.Test, redact)
			if data.Test.TextTruncated {
				e.truncate("text")
			}
			valid = e.test(*data.Test)
		}
	case "step":
		if data.Step != nil && data.Test == nil && data.Failure == nil && data.Stop == nil && data.Scope == "" && validEvidenceStep(*data.Step) {
			redactEvidenceStep(data.Step, redact)
			if data.Step.TextTruncated {
				e.truncate("text")
			}
			valid = e.step(*data.Step)
		}
	case "failure":
		if data.Failure != nil && data.Stop == nil && data.Test == nil && data.Step == nil && data.Scope == "" && validFailure(*data.Failure) && e.value.FirstFailure == nil {
			e.redactDetail(data.Failure, redact)
			e.value.FirstFailure = data.Failure
			valid = true
		}
	case "stop":
		if data.Stop != nil && data.Test == nil && data.Step == nil && data.Failure == nil && data.Scope == "" && validStop(*data.Stop) && e.value.StopContext == nil {
			e.redactDetail(data.Stop, redact)
			e.value.StopContext = data.Stop
			valid = true
		}
	case "truncated":
		if data.Test == nil && data.Step == nil && data.Failure == nil && data.Stop == nil && slices.Contains([]string{"tests", "user_steps", "other_steps", "text"}, data.Scope) {
			e.truncate(data.Scope)
			valid = true
		}
	case "complete":
		if data.Test == nil && data.Step == nil && data.Failure == nil && data.Stop == nil && data.Scope == "" {
			e.complete()
			valid = true
		}
	}
	if !valid {
		e.invalidate("invalid_record")
	}
}

func (e *evidenceState) redactDetail(detail *synthetic.FailureDetail, redact *textRedactor) {
	redactEvidenceTest(&detail.Test, redact)
	detail.TextTruncated = detail.TextTruncated || detail.Test.TextTruncated
	for i := range detail.Path {
		redactEvidenceStep(&detail.Path[i], redact)
		detail.TextTruncated = detail.TextTruncated || detail.Path[i].TextTruncated
	}
	var clipped bool
	detail.Message, clipped = evidenceText(detail.Message, failureMessageBytes, redact)
	locationClipped := redactLocation(detail.Location, redact)
	detail.TextTruncated = detail.TextTruncated || clipped || locationClipped
	if detail.TextTruncated {
		e.truncate("text")
	}
}

func (e *evidenceState) test(value synthetic.EvidenceTest) bool {
	if i, ok := e.tests[value.ID]; ok {
		old := e.value.Tests[i]
		if old.ReportingComplete || !value.ReportingComplete || old.Title != value.Title || !reflect.DeepEqual(old.Location, value.Location) {
			// Conflicting observations must not leave the disputed item looking valid.
			e.value.Tests = slices.Delete(e.value.Tests, i, i+1)
			if e.value.FirstFailure != nil && e.value.FirstFailure.Test.ID == value.ID {
				e.value.FirstFailure = nil
			}
			if e.value.StopContext != nil && e.value.StopContext.Test.ID == value.ID {
				e.value.StopContext = nil
			}
			return false
		}
		e.value.Tests[i] = value
		return true
	}
	if len(e.tests) == evidenceTestLimit {
		e.truncate("tests")
		return true
	}
	// Playwright may report skipped tests without a begin callback.
	if value.ReportingComplete && value.Status != "skipped" {
		return false
	}
	e.tests[value.ID] = len(e.value.Tests)
	e.value.Tests = append(e.value.Tests, value)
	return true
}

func (e *evidenceState) step(value synthetic.EvidenceStep) bool {
	if i, ok := e.steps[value.ID]; ok {
		old := e.value.Steps[i]
		if old.State != "running" || value.State == "running" || old.TestID != value.TestID || old.Title != value.Title || old.Category != value.Category || old.ParentID != value.ParentID || old.ParentOmitted != value.ParentOmitted || !reflect.DeepEqual(old.Location, value.Location) {
			e.value.Steps = slices.Delete(e.value.Steps, i, i+1)
			for j := range e.value.Steps {
				if e.value.Steps[j].ParentID == value.ID {
					e.value.Steps[j].ParentOmitted = true
				}
			}
			for _, detail := range []**synthetic.FailureDetail{&e.value.FirstFailure, &e.value.StopContext} {
				if *detail == nil {
					continue
				}
				for _, step := range (*detail).Path {
					if step.ID == value.ID {
						*detail = nil
						break
					}
				}
			}
			return false
		}
		e.value.Steps[i] = value
		return true
	}
	user := value.Category == "test.step"
	if user && e.userSteps == evidenceUserStepLimit || !user && e.otherSteps == evidenceOtherStepLimit {
		if user {
			e.truncate("user_steps")
		} else {
			e.truncate("other_steps")
		}
		return true
	}
	if value.State != "running" {
		return false
	}
	if value.ParentID != "" && !value.ParentOmitted {
		i, ok := e.steps[value.ParentID]
		if !ok || e.value.Steps[i].TestID != value.TestID {
			return false
		}
	}
	e.steps[value.ID] = len(e.value.Steps)
	e.value.Steps = append(e.value.Steps, value)
	if user {
		e.userSteps++
	} else {
		e.otherSteps++
	}
	return true
}

func (e *evidenceState) snapshot() *synthetic.FailureEvidence {
	if e == nil {
		return nil
	}
	out := synthetic.CloneFailureEvidence(&e.value)
	out.FeatureStatus = e.status()
	return out
}

func boundedJSON(value any, limit int) bool {
	raw, err := json.Marshal(value)
	return err == nil && len(raw) <= limit
}

func validEvidenceID(value string, limit int) bool {
	if value == "" || !boundedJSON(value, limit) {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validLocation(l *synthetic.EvidenceLocation) bool {
	if l == nil {
		return true
	}
	if l.File == "" || strings.HasPrefix(l.File, "/") || strings.ContainsAny(l.File, "\\:\x00") || !boundedJSON(l.File, sourceBytes) || l.Line <= 0 || l.Column <= 0 || int64(l.Line) > maxSafeInteger || int64(l.Column) > maxSafeInteger {
		return false
	}
	return !slices.Contains(strings.Split(l.File, "/"), "..")
}

func validDuration(d *float64) bool {
	return d == nil || finite(*d) && *d >= 0 && *d <= maxSafeInteger
}

func validEvidenceTest(t synthetic.EvidenceTest) bool {
	// The producer's 1024-byte limit per serialized test never binds here: the
	// field budgets keep a test well below it.
	return validEvidenceID(t.ID, testIDBytes) && boundedJSON(t.Title, titleBytes) && validLocation(t.Location) &&
		slices.Contains([]string{"running", "passed", "failed", "timedOut", "skipped", "interrupted"}, t.Status) &&
		slices.Contains([]string{"passed", "failed", "skipped"}, t.ExpectedStatus) &&
		(t.Status != "running") == t.ReportingComplete && validDuration(t.DurationMS)
}

func validEvidenceStep(s synthetic.EvidenceStep) bool {
	return validEvidenceID(s.ID, stepIDBytes) && validEvidenceID(s.TestID, testIDBytes) &&
		(s.ParentID == "" || validEvidenceID(s.ParentID, stepIDBytes) && s.ParentID != s.ID) &&
		boundedJSON(s.Title, titleBytes) && s.Category != "" && boundedJSON(s.Category, categoryBytes) && validLocation(s.Location) &&
		slices.Contains([]string{"running", "passed", "failed"}, s.State) && validDuration(s.DurationMS) &&
		boundedJSON(s.Message, stepMessageBytes) && boundedJSON(s, stepBytes)
}

func validFailure(f synthetic.FailureDetail) bool {
	return slices.Contains([]string{"failed", "timedOut", "passed"}, f.Test.Status) &&
		f.Test.Status != f.Test.ExpectedStatus && (f.Test.Status != "passed" || f.Test.ExpectedStatus == "failed") && validDetail(f)
}

func validStop(f synthetic.FailureDetail) bool {
	return slices.Contains([]string{"timedOut", "interrupted"}, f.Test.Status) && f.Message == "" &&
		(f.PathIncomplete || len(f.Path) > 0 && f.Path[len(f.Path)-1].State == "running") && validDetail(f)
}

func validDetail(f synthetic.FailureDetail) bool {
	if !validEvidenceTest(f.Test) || !f.Test.ReportingComplete ||
		len(f.Path) > failurePathLimit || f.OmittedAncestors < 0 || int64(f.OmittedAncestors) > maxSafeInteger || !validLocation(f.Location) ||
		utf8.RuneCountInString(f.Message) > textLimit || !boundedJSON(f, extensionBodyLimit) {
		return false
	}
	seen := make(map[string]bool, len(f.Path))
	for i, s := range f.Path {
		if !validEvidenceStep(s) || s.TestID != f.Test.ID || seen[s.ID] || s.Message != "" || s.DurationMS != nil {
			return false
		}
		if i > 0 && s.ParentID != f.Path[i-1].ID && !s.ParentOmitted {
			return false
		}
		seen[s.ID] = true
	}
	return true
}

// Redaction can expand text; trim after redaction to preserve the feature budget.
func evidenceText(value string, limit int, redact *textRedactor) (string, bool) {
	return clippedEvidenceText(redact.redacted(value), limit)
}
func clippedEvidenceText(value string, limit int) (string, bool) {
	clipped := utf8.RuneCountInString(value) > textLimit
	value = clip(value)
	if boundedJSON(value, limit) {
		return value, clipped
	}
	runes := []rune(value)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if boundedJSON(string(runes[:mid]), limit) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo]), true
}

func redactLocation(l *synthetic.EvidenceLocation, redact *textRedactor) bool {
	if l == nil {
		return false
	}
	var clipped bool
	l.File, clipped = evidenceText(l.File, sourceBytes, redact)
	if clipped {
		l.File = "[external]"
	}
	return clipped
}

func redactEvidenceTest(t *synthetic.EvidenceTest, redact *textRedactor) {
	var clipped bool
	t.Title, clipped = evidenceText(t.Title, titleBytes, redact)
	locationClipped := redactLocation(t.Location, redact)
	t.TextTruncated = t.TextTruncated || clipped || locationClipped
}

func redactEvidenceStep(s *synthetic.EvidenceStep, redact *textRedactor) {
	var clipped bool
	s.Title, clipped = evidenceText(s.Title, titleBytes, redact)
	s.TextTruncated = s.TextTruncated || clipped
	s.Message, clipped = evidenceText(s.Message, stepMessageBytes, redact)
	locationClipped := redactLocation(s.Location, redact)
	s.TextTruncated = s.TextTruncated || clipped || locationClipped
}
