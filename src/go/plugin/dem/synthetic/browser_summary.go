// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// BrowserSummary is optional fixture-context evidence, independent of run outcome.
type BrowserSummary struct {
	Availability     string            `json:"availability"`
	Reason           string            `json:"reason,omitempty"`
	ReporterComplete bool              `json:"reporter_complete"`
	Coverage         string            `json:"coverage"`
	Run              *BrowserAggregate `json:"run,omitempty"`
	Tests            []BrowserTest     `json:"tests,omitempty"`
	Samples          []BrowserSample   `json:"samples,omitempty"`
	Truncated        []string          `json:"truncated,omitempty"`
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

// MarshalJSON preserves a null primitive while omission objects carry only their reason.
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
func (a *BrowserArgument) UnmarshalJSON(raw []byte) error {
	var wire struct {
		Value   json.RawMessage `json:"value"`
		Omitted string          `json:"omitted"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	if (wire.Value == nil) == (wire.Omitted == "") {
		return errors.New("argument requires exactly one value or omission")
	}
	a.Omitted = wire.Omitted
	a.Value = nil
	if wire.Value != nil {
		return json.Unmarshal(wire.Value, &a.Value)
	}
	return nil
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
		v := *in.Run
		v.Omissions = slices.Clone(v.Omissions)
		out.Run = &v
	}
	out.Tests = slices.Clone(in.Tests)
	for i := range out.Tests {
		out.Tests[i].SampleIDs = slices.Clone(in.Tests[i].SampleIDs)
		if in.Tests[i].Counts != nil {
			v := *in.Tests[i].Counts
			out.Tests[i].Counts = &v
		}
	}
	out.Samples = slices.Clone(in.Samples)
	for i := range out.Samples {
		s := &out.Samples[i]
		s.Args = slices.Clone(s.Args)
		if s.Text != nil {
			v := *s.Text
			s.Text = &v
		}
		if s.URL != nil {
			v := *s.URL
			s.URL = &v
		}
	}
	return &out
}
func (b *BrowserSummary) ReportingStatus() string {
	if b == nil {
		return "not_recorded"
	}
	if b.Availability != "supported" {
		return b.Availability
	}
	if b.ReporterComplete {
		return "complete"
	}
	return "partial"
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

// Typed JSON boundaries reject explicit null counters; omitted counters within
// the required aggregate objects retain their sparse-zero meaning.
func validateBrowserCounters(raw []byte, keys []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("missing counter object")
	}
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			if string(value) == "null" {
				return errors.New("null counter")
			}
			var n int64
			if err := json.Unmarshal(value, &n); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *BrowserCounts) UnmarshalJSON(raw []byte) error {
	if err := validateBrowserCounters(raw, []string{"uncaught", "console_error", "transport", "http_error"}); err != nil {
		return err
	}
	type plain BrowserCounts
	return json.Unmarshal(raw, (*plain)(c))
}
func (c *BrowserTestCounts) UnmarshalJSON(raw []byte) error {
	if err := validateBrowserCounters(raw, []string{"observed", "incomplete", "invalid", "no_summary"}); err != nil {
		return err
	}
	type plain BrowserTestCounts
	return json.Unmarshal(raw, (*plain)(c))
}
func (a *BrowserAggregate) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if fields["tests"] == nil || fields["counts"] == nil {
		return errors.New("missing aggregate objects")
	}
	type plain BrowserAggregate
	return json.Unmarshal(raw, (*plain)(a))
}

func (s *BrowserSample) UnmarshalJSON(raw []byte) error {
	if err := validateBrowserCounters(raw, []string{"id", "args_omitted", "status"}); err != nil {
		return err
	}
	type plain BrowserSample
	return json.Unmarshal(raw, (*plain)(s))
}
