// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
)

// captureDetailLimit matches the producer (src/dem-runtime/runner/capture-details.cjs):
// 32 eligible tests with four screenshots need 160 test and link records. It
// bounds optional detail only; terminal candidates have no count cap.
const captureDetailLimit = 256

// captureState decodes capture_details.v1. failure_trace.v1 has no records;
// its lifecycle is stored with the capture report.
type captureState struct {
	featureState
	trace   *featureState // nil unless traces were requested
	value   synthetic.CaptureReport
	tests   map[string]int
	ids     map[string]int
	records int
}

func newCaptureState() *captureState {
	return &captureState{tests: make(map[string]int), ids: make(map[string]int)}
}

func (c *captureState) invalid() { c.invalidate("invalid_record") }

// accept decodes one record of a selected, incomplete stream.
func (c *captureState) accept(raw json.RawMessage, redact *textRedactor) {
	var data struct {
		Record string `json:"record"`
		Test   *struct {
			synthetic.CaptureTest
			Accepted   *int64 `json:"accepted"`
			Unaccepted *int64 `json:"unaccepted"`
		} `json:"test"`
		ID     string `json:"id"`
		TestID string `json:"test_id"`
	}
	if json.Unmarshal(raw, &data) != nil {
		c.invalid()
		return
	}
	switch data.Record {
	case "test":
		if data.Test == nil || data.Test.Accepted == nil || data.Test.Unaccepted == nil || data.ID != "" || data.TestID != "" {
			c.invalid()
			return
		}
		value := data.Test.CaptureTest
		value.Accepted, value.Unaccepted = *data.Test.Accepted, *data.Test.Unaccepted
		if !validCaptureTest(value) {
			c.invalid()
			return
		}
		if i, ok := c.tests[data.Test.ID]; ok {
			// A disputed test cannot leave its earlier associations looking valid.
			c.value.Tests = slices.Delete(c.value.Tests, i, i+1)
			c.value.Associations = slices.DeleteFunc(c.value.Associations, func(a synthetic.CaptureAssociation) bool { return a.TestID == data.Test.ID })
			c.invalid()
			return
		}
		if !c.admit() {
			return
		}
		var clipped bool
		value.Title, clipped = evidenceText(value.Title, titleBytes, redact)
		value.TextTruncated = value.TextTruncated || clipped
		c.tests[data.Test.ID] = len(c.value.Tests)
		c.value.Tests = append(c.value.Tests, value)
	case "capture":
		if data.Test != nil || !artifacts.ValidArtifactID(data.ID) || !validEvidenceID(data.TestID, testIDBytes) {
			c.invalid()
			return
		}
		if i, ok := c.ids[data.ID]; ok {
			c.value.Associations = slices.Delete(c.value.Associations, i, i+1)
			c.invalid()
			return
		}
		if c.records == captureDetailLimit {
			c.value.Truncated = true
			return
		}
		if _, ok := c.tests[data.TestID]; !ok {
			c.invalid()
			return
		}
		if !c.admit() {
			return
		}
		c.ids[data.ID] = len(c.value.Associations)
		c.value.Associations = append(c.value.Associations, synthetic.CaptureAssociation{ID: data.ID, TestID: data.TestID})
	case "truncated", "complete":
		if data.Test != nil || data.ID != "" || data.TestID != "" {
			c.invalid()
			return
		}
		if data.Record == "truncated" {
			c.value.Truncated = true
		} else {
			c.complete()
		}
	default:
		c.invalid()
	}
}

func (c *captureState) admit() bool {
	if c.records == captureDetailLimit {
		c.value.Truncated = true
		return false
	}
	c.records++
	return true
}

func validCaptureTest(t synthetic.CaptureTest) bool {
	return validEvidenceID(t.ID, testIDBytes) && boundedJSON(t.Title, titleBytes) &&
		slices.Contains([]string{"failed", "timedOut", "interrupted"}, t.Status) &&
		slices.Contains([]string{"passed", "failed", "skipped"}, t.ExpectedStatus) &&
		t.Accepted >= 0 && t.Accepted <= maxSafeInteger && t.Unaccepted >= 0 && t.Unaccepted <= maxSafeInteger
}

func (c *captureState) snapshot() *synthetic.CaptureReport {
	if c == nil {
		return nil
	}
	out := synthetic.CloneCaptureReport(&c.value)
	out.Reporting = c.word("partial")
	if c.trace != nil {
		out.Trace = c.trace.word("supported")
	}
	return out
}
