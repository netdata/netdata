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

const failureEvidenceFeature = "failure_evidence.v1"

// Diagnostic quotas bound arbitrary user-script detail independently of core
// outcomes. Separate pools reserve test and user-step detail against API noise.
// With the per-item limits below, retained data is <512 KiB and the producer's
// begin/end records plus first failure emit <1 MiB. See the runtime wire policy.
const (
	evidenceTestLimit      = 64
	evidenceUserStepLimit  = 128
	evidenceOtherStepLimit = 64
	evidencePayloadLimit   = 32 << 10
	maxSafeInteger         = 1<<53 - 1
)

type evidenceState struct {
	value       *synthetic.FailureEvidence
	initialized bool
	coreStarted bool
	tests       map[string]int
	steps       map[string]int
	userSteps   int
	otherSteps  int
}

func newEvidenceState(kind synthetic.Kind) *evidenceState {
	if kind != synthetic.Journey {
		return nil
	}
	return &evidenceState{
		value: &synthetic.FailureEvidence{Availability: "unavailable", Reason: "negotiation_not_reported"},
		tests: make(map[string]int), steps: make(map[string]int),
	}
}

func (e *evidenceState) core() {
	if e != nil {
		e.coreStarted = true
	}
}

func (e *evidenceState) invalid(reason string) {
	if e == nil || e.value.Availability == "invalid" {
		return
	}
	e.value.Availability = "invalid"
	e.value.Reason = reason
	e.value.ReporterComplete = false
}

func (e *evidenceState) init(raw json.RawMessage) {
	if e == nil {
		return
	}
	if e.initialized || e.coreStarted {
		e.invalid("late_or_duplicate_init")
		return
	}
	e.initialized = true
	var body struct {
		Selected []string `json:"selected"`
	}
	if len(raw) > evidencePayloadLimit || decodeCore(raw, &body) != nil || body.Selected == nil || len(body.Selected) > 1 {
		e.invalid("invalid_init")
		return
	}
	if e.value.Availability == "invalid" {
		return
	}
	if len(body.Selected) == 0 {
		e.value.Availability, e.value.Reason = "unsupported", "runtime_declined"
		return
	}
	if body.Selected[0] != failureEvidenceFeature {
		e.invalid("unrequested_selection")
		return
	}
	e.value.Availability, e.value.Reason = "supported", ""
}

func (e *evidenceState) truncate(scope string) {
	if !slices.Contains(e.value.Truncated, scope) {
		e.value.Truncated = append(e.value.Truncated, scope)
	}
}

func (e *evidenceState) accept(raw json.RawMessage, redact *textRedactor) {
	if e == nil || e.value.Availability == "invalid" {
		return
	}
	var envelope struct {
		Feature string          `json:"feature"`
		Data    json.RawMessage `json:"data"`
	}
	if len(raw) > evidencePayloadLimit || decodeCore(raw, &envelope) != nil || envelope.Feature == "" {
		e.invalid("invalid_extension")
		return
	}
	if envelope.Feature == failureEvidenceFeature && !e.initialized {
		e.invalid("data_before_init")
		return
	}
	if envelope.Feature != failureEvidenceFeature || e.value.Availability != "supported" {
		// No map/cache indexed by arbitrary advertised feature IDs.
		if e.value.Reason == "" || e.value.Availability == "unsupported" {
			e.value.Reason = "unselected_extension"
		}
		return
	}
	if e.value.ReporterComplete {
		e.invalid("record_after_completion")
		return
	}
	var data struct {
		Record  string                   `json:"record"`
		Test    *synthetic.EvidenceTest  `json:"test"`
		Step    *synthetic.EvidenceStep  `json:"step"`
		Failure *synthetic.FailureDetail `json:"failure"`
		Scope   string                   `json:"scope"`
	}
	if json.Unmarshal(envelope.Data, &data) != nil {
		e.invalid("invalid_record")
		return
	}
	valid := false
	switch data.Record {
	case "test":
		if data.Test != nil && data.Step == nil && data.Failure == nil && data.Scope == "" && validEvidenceTest(*data.Test) {
			redactEvidenceTest(data.Test, redact)
			if data.Test.TextTruncated {
				e.truncate("text")
			}
			valid = e.test(*data.Test)
		}
	case "step":
		if data.Step != nil && data.Test == nil && data.Failure == nil && data.Scope == "" && validEvidenceStep(*data.Step) {
			redactEvidenceStep(data.Step, redact)
			if data.Step.TextTruncated {
				e.truncate("text")
			}
			valid = e.step(*data.Step)
		}
	case "failure":
		if data.Failure != nil && data.Test == nil && data.Step == nil && data.Scope == "" && validFailure(*data.Failure) && e.value.FirstFailure == nil {
			redactEvidenceTest(&data.Failure.Test, redact)
			data.Failure.TextTruncated = data.Failure.TextTruncated || data.Failure.Test.TextTruncated
			for i := range data.Failure.Path {
				redactEvidenceStep(&data.Failure.Path[i], redact)
				data.Failure.TextTruncated = data.Failure.TextTruncated || data.Failure.Path[i].TextTruncated
			}
			var clipped bool
			data.Failure.Message, clipped = evidenceText(data.Failure.Message, 12002, redact)
			locationClipped := redactLocation(data.Failure.Location, redact)
			data.Failure.TextTruncated = data.Failure.TextTruncated || clipped || locationClipped
			if data.Failure.TextTruncated {
				e.truncate("text")
			}
			e.value.FirstFailure = data.Failure
			valid = true
		}
	case "truncated":
		if data.Test == nil && data.Step == nil && data.Failure == nil && slices.Contains([]string{"tests", "user_steps", "other_steps", "text"}, data.Scope) {
			e.truncate(data.Scope)
			valid = true
		}
	case "complete":
		if data.Test == nil && data.Step == nil && data.Failure == nil && data.Scope == "" {
			e.value.ReporterComplete = true
			valid = true
		}
	}
	if !valid {
		e.invalid("invalid_record")
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
			if f := e.value.FirstFailure; f != nil {
				for _, step := range f.Path {
					if step.ID == value.ID {
						e.value.FirstFailure = nil
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
	return synthetic.CloneFailureEvidence(e.value)
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
	if l.File == "" || strings.HasPrefix(l.File, "/") || strings.ContainsAny(l.File, "\\:\x00") || !boundedJSON(l.File, 256) || l.Line <= 0 || l.Column <= 0 || int64(l.Line) > maxSafeInteger || int64(l.Column) > maxSafeInteger {
		return false
	}
	return !slices.Contains(strings.Split(l.File, "/"), "..")
}

func validDuration(d *float64) bool {
	return d == nil || finite(*d) && *d >= 0 && *d <= maxSafeInteger
}

func validEvidenceTest(t synthetic.EvidenceTest) bool {
	return validEvidenceID(t.ID, 128) && boundedJSON(t.Title, 256) && validLocation(t.Location) &&
		slices.Contains([]string{"running", "passed", "failed", "timedOut", "skipped", "interrupted"}, t.Status) &&
		slices.Contains([]string{"passed", "failed", "skipped"}, t.ExpectedStatus) &&
		(t.Status != "running") == t.ReportingComplete && validDuration(t.DurationMS) && boundedJSON(t, 1024)
}

func validEvidenceStep(s synthetic.EvidenceStep) bool {
	return validEvidenceID(s.ID, 64) && validEvidenceID(s.TestID, 128) && (s.ParentID == "" || validEvidenceID(s.ParentID, 64) && s.ParentID != s.ID) &&
		boundedJSON(s.Title, 256) && s.Category != "" && boundedJSON(s.Category, 64) && validLocation(s.Location) &&
		slices.Contains([]string{"running", "passed", "failed"}, s.State) && validDuration(s.DurationMS) && boundedJSON(s.Message, 1024) && boundedJSON(s, 2048)
}

func validFailure(f synthetic.FailureDetail) bool {
	if !validEvidenceTest(f.Test) || !f.Test.ReportingComplete || !slices.Contains([]string{"failed", "timedOut", "passed"}, f.Test.Status) ||
		f.Test.Status == f.Test.ExpectedStatus || f.Test.Status == "passed" && f.Test.ExpectedStatus != "failed" ||
		len(f.Path) > 16 || f.OmittedAncestors < 0 || int64(f.OmittedAncestors) > maxSafeInteger || !validLocation(f.Location) ||
		utf8.RuneCountInString(f.Message) > TextLimit || !boundedJSON(f.Message, 12002) || !boundedJSON(f, evidencePayloadLimit) {
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
	value = redact.redacted(value)
	clipped := utf8.RuneCountInString(value) > TextLimit
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
	l.File, clipped = evidenceText(l.File, 256, redact)
	if clipped {
		l.File = "[external]"
	}
	return clipped
}

func redactEvidenceTest(t *synthetic.EvidenceTest, redact *textRedactor) {
	var clipped bool
	t.Title, clipped = evidenceText(t.Title, 256, redact)
	locationClipped := redactLocation(t.Location, redact)
	t.TextTruncated = t.TextTruncated || clipped || locationClipped
}

func redactEvidenceStep(s *synthetic.EvidenceStep, redact *textRedactor) {
	var clipped bool
	s.Title, clipped = evidenceText(s.Title, 256, redact)
	s.TextTruncated = s.TextTruncated || clipped
	s.Message, clipped = evidenceText(s.Message, 1024, redact)
	locationClipped := redactLocation(s.Location, redact)
	s.TextTruncated = s.TextTruncated || clipped || locationClipped
}
