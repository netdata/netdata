// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// BrowserSummary is optional fixture-context evidence, independent of run outcome.
type BrowserSummary struct {
	FeatureStatus
	Coverage  string            `json:"coverage"`
	Run       *BrowserAggregate `json:"run,omitempty"`
	Tests     []BrowserTest     `json:"tests,omitempty"`
	Samples   []BrowserSample   `json:"samples,omitempty"`
	Truncated []string          `json:"truncated,omitempty"`
}
type BrowserCounts struct {
	Uncaught     int64 `json:"uncaught,omitempty"`
	ConsoleError int64 `json:"console_error,omitempty"`
	Transport    int64 `json:"transport,omitempty"`
	HTTPError    int64 `json:"http_error,omitempty"`
}
type BrowserTestCounts struct {
	Observed   int64 `json:"observed,omitempty"`
	Incomplete int64 `json:"incomplete,omitempty"`
	Invalid    int64 `json:"invalid,omitempty"`
	NoSummary  int64 `json:"no_summary,omitempty"`
}
type BrowserAggregate struct {
	Tests     BrowserTestCounts `json:"tests"`
	Counts    BrowserCounts     `json:"counts"`
	Omissions []string          `json:"omissions,omitempty"`
}
type BrowserTest struct {
	ID        string         `json:"id"`
	State     string         `json:"state"`
	Counts    *BrowserCounts `json:"counts,omitempty"`
	SampleIDs []int          `json:"sample_ids,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}
type BrowserArgument struct {
	Value   any    `json:"value,omitempty"`
	Omitted string `json:"omitted,omitempty"`
}

// MarshalJSON stores a null primitive as {"value":null} and an omission as its
// reason alone; plain decoding reads both back. The runner decodes the wire
// form with stricter rules.
func (a BrowserArgument) MarshalJSON() ([]byte, error) {
	if a.Omitted != "" {
		return json.Marshal(struct {
			Omitted string `json:"omitted"`
		}{a.Omitted})
	}
	return json.Marshal(struct {
		Value any `json:"value"`
	}{a.Value})
}

type BrowserSample struct {
	ID            int               `json:"id"`
	TestID        string            `json:"test_id"`
	Category      string            `json:"category"`
	State         string            `json:"state,omitempty"`
	Args          []BrowserArgument `json:"args,omitempty"`
	Text          *string           `json:"text,omitempty"`
	TextOmitted   string            `json:"text_omitted,omitempty"`
	ArgsOmitted   int64             `json:"args_omitted,omitempty"`
	Message       string            `json:"message,omitempty"`
	Method        string            `json:"method,omitempty"`
	URL           *string           `json:"url,omitempty"`
	URLOmitted    string            `json:"url_omitted,omitempty"`
	Status        int64             `json:"status,omitempty"`
	Error         string            `json:"error,omitempty"`
	TextTruncated bool              `json:"text_truncated,omitempty"`
}

func CloneBrowserSummary(in *BrowserSummary) *BrowserSummary {
	if in == nil {
		return nil
	}
	out := *in
	out.Truncated = slices.Clone(in.Truncated)
	if in.Run != nil {
		run := *in.Run
		run.Omissions = slices.Clone(run.Omissions)
		out.Run = &run
	}
	out.Tests = slices.Clone(in.Tests)
	for i := range out.Tests {
		out.Tests[i].SampleIDs = slices.Clone(in.Tests[i].SampleIDs)
		out.Tests[i].Counts = clonePtr(in.Tests[i].Counts)
	}
	out.Samples = slices.Clone(in.Samples)
	for i := range out.Samples {
		s := &out.Samples[i]
		s.Args = slices.Clone(s.Args)
		s.Text = clonePtr(s.Text)
		s.URL = clonePtr(s.URL)
	}
	return &out
}

// ReportingStatus is NotRecorded for an absent summary, else its FeatureStatus.
func (b *BrowserSummary) ReportingStatus() string {
	if b == nil {
		return NotRecorded
	}
	return b.FeatureStatus.ReportingStatus()
}

// Summary names the observed scope and reporting status so missing coverage is explicit.
func (b *BrowserSummary) Summary() string {
	if b == nil {
		return ""
	}
	if b.Run == nil {
		return "fixture_context: " + b.ReportingStatus()
	}
	c := b.Run.Counts
	t := b.Run.Tests
	text := fmt.Sprintf("fixture_context (%s; tests: observed=%d, incomplete=%d, invalid=%d, no_summary=%d)", b.ReportingStatus(), t.Observed, t.Incomplete, t.Invalid, t.NoSummary)
	if t.Observed == 0 {
		text += ": no observed fixture contexts"
	} else {
		text += fmt.Sprintf(": observed errors: uncaught=%d, console_error=%d, transport=%d, http_error=%d", c.Uncaught, c.ConsoleError, c.Transport, c.HTTPError)
	}
	if len(b.Run.Omissions) > 0 {
		text += " [omissions: " + strings.Join(b.Run.Omissions, ", ") + "]"
	}
	return text
}
