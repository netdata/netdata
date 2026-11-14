// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type browserState struct {
	featureState
	value synthetic.BrowserSummary
	late  int
}

func newBrowserState() *browserState {
	return &browserState{value: synthetic.BrowserSummary{Coverage: "fixture_context"}}
}

func (b *browserState) snapshot() *synthetic.BrowserSummary {
	if b == nil {
		return nil
	}
	out := synthetic.CloneBrowserSummary(&b.value)
	out.FeatureStatus = b.status()
	return out
}

var browserFlags = []string{"args_limit", "rpc_budget", "candidate_limit", "pending", "unavailable", "too_long", "pool_full", "row_limit", "identity_limit"}

func validFlags(flags, allowed []string) bool {
	seen := map[string]bool{}
	for _, f := range flags {
		if seen[f] || !slices.Contains(allowed, f) {
			return false
		}
		seen[f] = true
	}
	return true
}
func countValues(c synthetic.BrowserCounts) []int64 {
	return []int64{c.Uncaught, c.ConsoleError, c.Transport, c.HTTPError}
}
func testCountValues(c synthetic.BrowserTestCounts) []int64 {
	return []int64{c.Observed, c.Incomplete, c.Invalid, c.NoSummary}
}
func safeCounts(values []int64) bool {
	for _, v := range values {
		if v < 0 || v > maxSafeInteger {
			return false
		}
	}
	return true
}
func sumSafe(values []int64) (int64, bool) {
	var sum int64
	for _, v := range values {
		if v < 0 || v > maxSafeInteger-sum {
			return 0, false
		}
		sum += v
	}
	return sum, true
}
func monotonic(old, new []int64) bool {
	for i, v := range old {
		if new[i] < v {
			return false
		}
	}
	return true
}
func (b *browserState) aggregate(a *synthetic.BrowserAggregate) bool {
	if a == nil || !safeCounts(countValues(a.Counts)) || !safeCounts(testCountValues(a.Tests)) || !validFlags(a.Omissions, browserFlags) {
		return false
	}
	if _, ok := sumSafe(testCountValues(a.Tests)); !ok {
		return false
	}
	if a.Tests.Observed == 0 {
		for _, v := range countValues(a.Counts) {
			if v != 0 {
				return false
			}
		}
	}
	if b.value.Run != nil {
		old := b.value.Run
		if old.Tests.Observed == a.Tests.Observed && old.Counts != a.Counts {
			return false
		}
		if !monotonic(countValues(old.Counts), countValues(a.Counts)) || !monotonic(testCountValues(old.Tests), testCountValues(a.Tests)) {
			return false
		}
		for _, f := range old.Omissions {
			if !slices.Contains(a.Omissions, f) {
				return false
			}
		}
	}
	return true
}

// accept decodes one record of a selected, incomplete stream.
func (b *browserState) accept(raw json.RawMessage, r *textRedactor) {
	var d struct {
		Record  string                      `json:"record"`
		Run     *synthetic.BrowserAggregate `json:"run"`
		Test    *synthetic.BrowserTest      `json:"test"`
		Samples []synthetic.BrowserSample   `json:"samples"`
		Sample  *synthetic.BrowserSample    `json:"sample"`
		Scope   string                      `json:"scope"`
		Reason  string                      `json:"reason"`
	}
	if json.Unmarshal(raw, &d) != nil {
		b.invalidate("invalid_record")
		return
	}
	bad := func() { b.invalidate("invalid_record") }
	if d.Record == "invalid" {
		if d.Run != nil || d.Test != nil || d.Sample != nil || len(d.Samples) > 0 || d.Scope != "" || !slices.Contains([]string{"counter_overflow", "annotation_limit"}, d.Reason) {
			bad()
			return
		}
		b.invalidate(d.Reason)
		return
	}
	if !b.aggregate(d.Run) || d.Reason != "" {
		bad()
		return
	}
	// All validation and sanitization is local until the entire record is accepted.
	switch d.Record {
	case "test":
		t := d.Test
		if t == nil || d.Sample != nil || d.Scope != "" || len(b.value.Tests) >= 64 || !validEvidenceID(t.ID, 128) || len(d.Samples) > 12 || len(t.SampleIDs) != len(d.Samples) {
			bad()
			return
		}
		for _, old := range b.value.Tests {
			if old.ID == t.ID {
				bad()
				return
			}
		}
		if !slices.Contains([]string{"observed", "incomplete", "invalid"}, t.State) || t.State == "observed" && (t.Counts == nil || !safeCounts(countValues(*t.Counts)) || t.Reason != "") || t.State != "observed" && (t.Counts != nil || len(t.SampleIDs) > 0) || t.State == "incomplete" && t.Reason != "" || !boundedJSON(t.Reason, 64) || t.Reason != "" && !validBrowserReason(t.Reason) {
			bad()
			return
		}
		oldTests := synthetic.BrowserTestCounts{}
		if b.value.Run != nil {
			oldTests = b.value.Run.Tests
		}
		previous, current := testCountValues(oldTests), testCountValues(d.Run.Tests)
		class := 0
		if t.State == "incomplete" {
			class = 1
		}
		if t.State == "invalid" {
			class = 2
		}
		if current[class] <= previous[class] {
			bad()
			return
		}
		if t.State == "observed" {
			if t.Counts.Uncaught == 0 && t.Counts.ConsoleError == 0 && t.Counts.Transport == 0 && t.Counts.HTTPError == 0 {
				bad()
				return
			}
			for i, v := range countValues(*t.Counts) {
				old := int64(0)
				if b.value.Run != nil {
					old = countValues(b.value.Run.Counts)[i]
				}
				if v > countValues(d.Run.Counts)[i]-old {
					bad()
					return
				}
			}
		}
		introducedCounts := map[string]int64{}
		for _, sample := range d.Samples {
			introducedCounts[sample.Category]++
			if t.Counts == nil || browserCategoryCount(*t.Counts, sample.Category) < introducedCounts[sample.Category] {
				bad()
				return
			}
		}
		if !b.samples(d.Samples, t.ID, t.SampleIDs, false, r) {
			bad()
			return
		}
		b.value.Tests = append(b.value.Tests, *t)
		b.value.Samples = append(b.value.Samples, d.Samples...)
	case "late_sample":
		if d.Test != nil || len(d.Samples) > 0 || d.Sample == nil || d.Scope != "" || len(b.value.Tests) != 64 || b.late >= 4 {
			bad()
			return
		}
		for _, test := range b.value.Tests {
			if test.ID == d.Sample.TestID {
				bad()
				return
			}
		}
		late := []synthetic.BrowserSample{*d.Sample}
		if !b.samples(late, d.Sample.TestID, []int{d.Sample.ID}, true, r) {
			bad()
			return
		}
		d.Sample = &late[0]
		if browserCategoryCount(d.Run.Counts, d.Sample.Category) == 0 {
			bad()
			return
		}
		b.value.Samples = append(b.value.Samples, *d.Sample)
		b.late++
	case "truncated":
		if d.Test != nil || d.Sample != nil || len(d.Samples) > 0 || !slices.Contains([]string{"tests", "samples", "text"}, d.Scope) || slices.Contains(b.value.Truncated, d.Scope) {
			bad()
			return
		}
		b.value.Truncated = append(b.value.Truncated, d.Scope)
	case "complete":
		if d.Test != nil || d.Sample != nil || len(d.Samples) > 0 || d.Scope != "" {
			bad()
			return
		}
		b.complete()
	default:
		bad()
		return
	}
	b.value.Run = d.Run
}
func validBrowserReason(s string) bool {
	return slices.Contains([]string{"invalid_annotation", "duplicate_annotation"}, s)
}
func (b *browserState) samples(samples []synthetic.BrowserSample, testID string, ids []int, late bool, r *textRedactor) bool {
	if len(b.value.Samples)+len(samples) > 16 {
		return false
	}
	cats := map[string]int{}
	seen := map[int]bool{}
	for _, s := range b.value.Samples {
		cats[s.Category]++
		seen[s.ID] = true
	}
	refs := map[int]bool{}
	for _, id := range ids {
		if refs[id] {
			return false
		}
		refs[id] = true
	}
	candidates := map[string]int{}
	for i := range samples {
		s := &samples[i]
		if s.ID < 1 || s.ID > 16 || seen[s.ID] || !refs[s.ID] || s.TestID != testID || !validEvidenceID(s.TestID, 128) || !slices.Contains([]string{"uncaught", "console_error", "transport", "http_error"}, s.Category) || cats[s.Category] >= 4 || late && cats[s.Category] > 0 {
			return false
		}
		seen[s.ID] = true
		cats[s.Category]++
		candidates[s.Category]++
		if candidates[s.Category] > 3 || !validBrowserSample(s, r) {
			return false
		}

	}
	return true
}
func validBrowserSample(s *synthetic.BrowserSample, r *textRedactor) bool {
	if s.TextOmitted != "" && (s.Category != "console_error" || s.State != "unavailable" || s.TextOmitted != "too_long" || s.Text != nil || s.Args != nil) {
		return false
	}
	if s.ArgsOmitted < 0 || s.ArgsOmitted > maxSafeInteger || !boundedJSON(s.Message, 512) || !boundedJSON(s.Error, 128) || s.Text != nil && !boundedJSON(*s.Text, 512) || s.URL != nil && !boundedJSON(*s.URL, 256) {
		return false
	}
	switch s.Category {
	case "console_error":
		if !slices.Contains([]string{"resolved", "pending", "unavailable"}, s.State) || s.Message != "" || s.Method != "" || s.URL != nil || s.URLOmitted != "" || s.Status != 0 || s.Error != "" || len(s.Args) > 4 || s.Text != nil && len(s.Args) > 0 || s.State != "resolved" && (s.Text != nil || len(s.Args) > 0) || s.State == "resolved" && s.Text == nil && len(s.Args) == 0 || s.TextTruncated {
			return false
		}
		if s.Text != nil {
			v := browserSafeText(*s.Text, r)
			if !boundedJSON(v, 512) {
				s.Text = nil
				s.State = "unavailable"
				s.TextOmitted = "too_long"
			} else {
				s.Text = &v
			}
		}
		for i := range s.Args {
			a := &s.Args[i]
			if a.Omitted != "" {
				if a.Value != nil || !slices.Contains([]string{"object", "symbol", "function", "unrepresentable", "non_finite", "too_long"}, a.Omitted) {
					return false
				}
				continue
			}
			switch v := a.Value.(type) {
			case nil, bool:
			case float64:
				if !finite(v) {
					return false
				}
			case string:
				if !boundedJSON(v, 512) {
					return false
				}
				v = browserSafeText(v, r)
				if !boundedJSON(v, 512) {
					a.Value = nil
					a.Omitted = "too_long"
				} else {
					a.Value = v
				}
			default:
				return false
			}
		}
	case "uncaught":
		if s.State != "" || s.Args != nil || s.Text != nil || s.ArgsOmitted != 0 || s.Method != "" || s.URL != nil || s.URLOmitted != "" || s.Status != 0 || s.Error != "" {
			return false
		}
		var c bool
		s.Message, c = browserText(s.Message, 512, r)
		s.TextTruncated = s.TextTruncated || c
	case "transport", "http_error":
		if s.State != "" || s.Args != nil || s.Text != nil || s.ArgsOmitted != 0 || s.Message != "" || !boundedJSON(s.Method, 16) || s.URL == nil && s.URLOmitted != "too_long" || s.URL != nil && s.URLOmitted != "" {
			return false
		}
		if s.Category == "http_error" && (s.Status < 400 || s.Status > maxSafeInteger || s.Error != "") || s.Category == "transport" && s.Status != 0 {
			return false
		}
		var c bool
		s.Method, c = browserText(s.Method, 16, r)
		s.TextTruncated = s.TextTruncated || c
		s.Error, c = browserText(s.Error, 128, r)
		s.TextTruncated = s.TextTruncated || c
		if s.URL != nil {
			v := browserSafeURL(r.redacted(*s.URL))
			if !boundedJSON(v, 256) {
				s.URL = nil
				s.URLOmitted = "too_long"
			} else {
				s.URL = &v
			}
		}
	}
	return true
}

func browserCategoryCount(c synthetic.BrowserCounts, category string) int64 {
	switch category {
	case "uncaught":
		return c.Uncaught
	case "console_error":
		return c.ConsoleError
	case "http_error":
		return c.HTTPError
	case "transport":
		return c.Transport
	}
	return 0
}

var browserURLText = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)

func browserSafeText(value string, r *textRedactor) string {
	return browserURLText.ReplaceAllStringFunc(r.redacted(value), func(value string) string {
		if safe := browserSafeURL(value); safe != "" {
			return safe
		}
		return "[unavailable]"
	})
}

func browserSafeURL(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		// WHATWG URLs can contain escapes rejected by net/url (for example %zz).
		// Omit the unusable URL without discarding the remaining browser evidence.
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

func browserText(value string, limit int, r *textRedactor) (string, bool) {
	return clippedEvidenceText(browserSafeText(value, r), limit)
}
