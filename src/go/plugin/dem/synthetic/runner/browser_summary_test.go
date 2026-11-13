// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const browserInit = `{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1","browser_summary.v1"]}}`

func browserRecord(t *testing.T, data any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"type": "extension", "extension": map[string]any{"feature": browserSummaryFeature, "data": data}})
	require.NoError(t, err)
	return string(raw)
}
func browserRun(n int64) synthetic.BrowserAggregate {
	return synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: n}, Counts: synthetic.BrowserCounts{Uncaught: n}}
}
func browserTestRecord(n int64) map[string]any {
	return map[string]any{"record": "test", "test": synthetic.BrowserTest{ID: fmt.Sprintf("t%d", n), State: "observed", Counts: &synthetic.BrowserCounts{Uncaught: 1}}, "run": browserRun(n)}
}
func consumeBrowser(t *testing.T, lines ...string) sinkSnapshot {
	t.Helper()
	s := newSink(synthetic.Journey, false, newRedactor(map[string]string{"DEM_SECRET_X": "secret"}), true)
	require.NoError(t, s.consume(strings.NewReader(strings.Join(lines, "\n"))))
	return s.snapshot()
}
func TestBrowserFeatureIsolationAndNegotiation(t *testing.T) {
	for _, body := range []any{nil, 3, []any{}, map[string]any{"record": "future"}, map[string]any{"record": "complete", "run": 3}, map[string]any{"record": "complete", "run": map[string]any{"tests": map[string]any{"observed": 1.5}}}} {
		t.Run(fmt.Sprint(body), func(t *testing.T) {
			s := consumeBrowser(t, browserInit, browserRecord(t, body), evidenceRecord("complete", "", nil))
			assert.Equal(t, "invalid", s.browser.ReportingStatus())
			assert.Equal(t, "complete", s.evidence.ReportingStatus())
			assert.NoError(t, s.err)
		})
	}
	for name, lines := range map[string][]string{"old": {}, "declined": {`{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1"]}}`}, "before init": {browserRecord(t, map[string]any{"record": "complete", "run": browserRun(0)})}, "duplicate init": {browserInit, browserInit}, "shared invalid": {browserInit, `{"type":"extension","extension":{"feature":"browser_summary.v1","future":true,"data":{}}}`}} {
		t.Run(name, func(t *testing.T) {
			s := consumeBrowser(t, lines...)
			want := "invalid"
			if name == "old" {
				want = "unavailable"
			}
			if name == "declined" {
				want = "unsupported"
			}
			assert.Equal(t, want, s.browser.ReportingStatus())
		})
	}
	s := newSink(synthetic.Journey, false, newRedactor(nil), false)
	require.NoError(t, s.consume(strings.NewReader(evidenceInit+"\n"+browserRecord(t, 3)+"\n"+evidenceRecord("complete", "", nil))))
	assert.Nil(t, s.snapshot().browser)
	assert.Equal(t, "complete", s.snapshot().evidence.ReportingStatus())
}
func TestBrowserAtomicValidationAndTerminalPreservesPrior(t *testing.T) {
	initial := browserTestRecord(1)
	cases := map[string]map[string]any{
		"regression":                 {"record": "complete", "run": browserRun(0)},
		"duplicate row":              browserTestRecord(1),
		"unsafe":                     {"record": "complete", "run": synthetic.BrowserAggregate{Counts: synthetic.BrowserCounts{Uncaught: maxSafeInteger + 1}}},
		"negative":                   {"record": "complete", "run": synthetic.BrowserAggregate{Counts: synthetic.BrowserCounts{Uncaught: -1}}},
		"terminal error":             {"record": "invalid", "reason": "counter_overflow"},
		"invalid terminal aggregate": {"record": "invalid", "reason": "annotation_limit", "run": browserRun(2)},
		"wrong ownership":            {"record": "test", "run": browserRun(2), "test": synthetic.BrowserTest{ID: "t2", State: "observed", Counts: &synthetic.BrowserCounts{Uncaught: 1}, SampleIDs: []int{1}}, "samples": []synthetic.BrowserSample{{ID: 1, TestID: "t1", Category: "uncaught", Message: "x"}}},
		"invalid sample":             {"record": "test", "run": browserRun(2), "test": synthetic.BrowserTest{ID: "t2", State: "observed", Counts: &synthetic.BrowserCounts{Uncaught: 1}, SampleIDs: []int{1}}, "samples": []synthetic.BrowserSample{{ID: 1, TestID: "t2", Category: "uncaught", Message: strings.Repeat("x", 513)}}},
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			s := consumeBrowser(t, browserInit, browserRecord(t, initial), browserRecord(t, bad))
			assert.Equal(t, "invalid", s.browser.ReportingStatus())
			require.Len(t, s.browser.Tests, 1)
			assert.Empty(t, s.browser.Samples)
			assert.Equal(t, browserRun(1), *s.browser.Run)
		})
	}
	s := consumeBrowser(t, browserInit, browserRecord(t, map[string]any{"record": "complete", "run": browserRun(0)}), browserRecord(t, map[string]any{"record": "complete", "run": browserRun(0)}))
	assert.Equal(t, "invalid", s.browser.ReportingStatus())
	assert.False(t, s.browser.ReporterComplete)
}
func TestBrowserRedactionSamplesAndSnapshots(t *testing.T) {
	raw := map[string]any{"record": "test", "run": synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: synthetic.BrowserCounts{HTTPError: 1, ConsoleError: 1}}, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &synthetic.BrowserCounts{HTTPError: 1, ConsoleError: 1}, SampleIDs: []int{1, 2}}, "samples": []any{map[string]any{"id": 1, "test_id": "t", "category": "http_error", "method": "GET", "url": "https://user:pass@example.com/private?q=secret#frag", "status": 1200}, map[string]any{"id": 2, "test_id": "t", "category": "console_error", "state": "resolved", "args": []any{map[string]any{"value": "secret"}, map[string]any{"value": nil}, map[string]any{"omitted": "unrepresentable"}}}}}
	s := consumeBrowser(t, browserInit, browserRecord(t, raw))
	require.Equal(t, "partial", s.browser.ReportingStatus())
	require.Len(t, s.browser.Samples, 2)
	assert.Equal(t, "https://example.com/private", *s.browser.Samples[0].URL)
	assert.Equal(t, "[REDACTED]", s.browser.Samples[1].Args[0].Value)
	copy := synthetic.CloneBrowserSummary(s.browser)
	*copy.Samples[0].URL = "changed"
	copy.Tests[0].SampleIDs[0] = 9
	copy.Samples[1].Args[0].Value = "changed"
	assert.Equal(t, "https://example.com/private", *s.browser.Samples[0].URL)
	assert.Equal(t, 1, s.browser.Tests[0].SampleIDs[0])
	assert.Equal(t, "[REDACTED]", s.browser.Samples[1].Args[0].Value)
	encoded, err := json.Marshal(s.browser)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `{"value":null}`)
	assert.Contains(t, string(encoded), `{"omitted":"unrepresentable"}`)
	assert.NotContains(t, string(encoded), "secret")
}
func TestBrowserUnusableURLPreservesEvidenceAndCompletion(t *testing.T) {
	for _, category := range []string{"http_error", "transport"} {
		t.Run(category, func(t *testing.T) {
			counts := synthetic.BrowserCounts{HTTPError: 1}
			if category == "transport" {
				counts = synthetic.BrowserCounts{Transport: 1}
			}
			run := synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: counts}
			sample := map[string]any{"id": 1, "test_id": "t", "category": category, "method": "GET", "url": "https://user:secret@example.com/%zz?private=secret#fragment"}
			if category == "http_error" {
				sample["status"] = 500
			} else {
				sample["error"] = "connection refused"
			}
			s := consumeBrowser(t, browserInit,
				browserRecord(t, map[string]any{"record": "test", "run": run, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &counts, SampleIDs: []int{1}}, "samples": []any{sample}}),
				browserRecord(t, map[string]any{"record": "complete", "run": run}), evidenceRecord("complete", "", nil))
			require.Equal(t, "complete", s.browser.ReportingStatus())
			assert.Equal(t, "complete", s.evidence.ReportingStatus())
			assert.Equal(t, run, *s.browser.Run)
			require.Len(t, s.browser.Samples, 1)
			require.NotNil(t, s.browser.Samples[0].URL)
			assert.Empty(t, *s.browser.Samples[0].URL)
			assert.Empty(t, s.browser.Samples[0].URLOmitted)
			raw, err := json.Marshal(s.browser)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "secret")
			assert.NotContains(t, string(raw), "private=")
		})
	}
}

func TestBrowserRowsPoolAndLateSamples(t *testing.T) {
	lines := []string{browserInit}
	for n := int64(1); n <= 64; n++ {
		lines = append(lines, browserRecord(t, browserTestRecord(n)))
	}
	sample := synthetic.BrowserSample{ID: 1, TestID: "late", Category: "uncaught", Message: "late error"}
	lines = append(lines, browserRecord(t, map[string]any{"record": "late_sample", "run": browserRun(65), "sample": sample}))
	s := consumeBrowser(t, lines...)
	require.Equal(t, "partial", s.browser.ReportingStatus())
	require.Len(t, s.browser.Tests, 64)
	require.Len(t, s.browser.Samples, 1)
	lines = append(lines, browserRecord(t, map[string]any{"record": "late_sample", "run": browserRun(66), "sample": synthetic.BrowserSample{ID: 2, TestID: "other", Category: "uncaught", Message: "another"}}))
	s = consumeBrowser(t, lines...)
	assert.Equal(t, "invalid", s.browser.ReportingStatus())
	assert.Len(t, s.browser.Samples, 1)
	assert.Equal(t, int64(65), s.browser.Run.Tests.Observed)
}
func TestBrowserSharedEnvelopeCap(t *testing.T) {
	raw := `{"type":"extension","extension":{"feature":"browser_summary.v1","data":{"record":"complete","extra":"` + strings.Repeat("x", 32<<10) + `"}}}`
	s := consumeBrowser(t, browserInit, raw)
	assert.Equal(t, "invalid", s.browser.ReportingStatus())
	assert.Equal(t, "invalid", s.evidence.ReportingStatus())
}

func TestBrowserFieldAndSemanticBudgets(t *testing.T) {
	base := func(sample any) map[string]any {
		return map[string]any{"record": "test", "run": browserRun(1), "test": synthetic.BrowserTest{ID: "t1", State: "observed", Counts: &synthetic.BrowserCounts{Uncaught: 1}, SampleIDs: []int{1}}, "samples": []any{sample}}
	}
	for name, sample := range map[string]any{
		"missing primitive":        map[string]any{"id": 1, "test_id": "t1", "category": "console_error", "state": "resolved", "args": []any{map[string]any{}}},
		"object value":             map[string]any{"id": 1, "test_id": "t1", "category": "console_error", "state": "resolved", "args": []any{map[string]any{"value": map[string]any{"secret": "x"}}}},
		"both argument fields":     map[string]any{"id": 1, "test_id": "t1", "category": "console_error", "state": "resolved", "args": []any{map[string]any{"value": nil, "omitted": "object"}}},
		"long ID":                  synthetic.BrowserSample{ID: 1, TestID: strings.Repeat("x", 127), Category: "uncaught"},
		"incomplete invented text": map[string]any{"id": 1, "test_id": "t1", "category": "console_error", "state": "pending", "text": "invented"},
		"URL contradiction":        map[string]any{"id": 1, "test_id": "t1", "category": "http_error", "method": "GET", "url": "https://example.com", "url_omitted": "too_long", "status": 400},
	} {
		t.Run(name, func(t *testing.T) {
			s := consumeBrowser(t, browserInit, browserRecord(t, base(sample)))
			assert.Equal(t, "invalid", s.browser.ReportingStatus())
			assert.Nil(t, s.browser.Run)
			assert.Empty(t, s.browser.Samples)
		})
	}
	// Count arithmetic is field-wise; independent categories may exceed a safe
	// integer when summed, but no reported field may exceed that boundary.
	counts := synthetic.BrowserCounts{Uncaught: maxSafeInteger, ConsoleError: maxSafeInteger}
	aggregate := synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: counts}
	s := consumeBrowser(t, browserInit, browserRecord(t, map[string]any{"record": "test", "run": aggregate, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &counts}}))
	assert.Equal(t, "partial", s.browser.ReportingStatus())
}
func TestBrowserStructuralByteCeilings(t *testing.T) {
	// Combine structural maxima conservatively. The limits derive from collection
	// quotas and field bounds; production never reserializes growing state.
	id := strings.Repeat("a", 126)
	text := strings.Repeat("x", 510)
	method := strings.Repeat("G", 14)
	address := "https://example.com/" + strings.Repeat("x", 234)
	aggregate := synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: maxSafeInteger / 4, Incomplete: maxSafeInteger / 4, Invalid: maxSafeInteger / 4, NoSummary: maxSafeInteger / 4}, Counts: synthetic.BrowserCounts{Uncaught: maxSafeInteger, ConsoleError: maxSafeInteger, Transport: maxSafeInteger, HTTPError: maxSafeInteger}, Omissions: slices.Clone(browserFlags)}
	retained := &synthetic.BrowserSummary{Availability: "supported", Coverage: "fixture_context", ReporterComplete: true, Run: &aggregate, Truncated: []string{"tests", "samples", "text"}}
	var newSamples []synthetic.BrowserSample
	for i := 0; i < 16; i++ {
		sample := synthetic.BrowserSample{ID: i + 1, TestID: id, Category: []string{"uncaught", "console_error", "transport", "http_error"}[i/4], TextTruncated: true}
		switch sample.Category {
		case "uncaught":
			sample.Message = text
		case "console_error":
			sample.State = "resolved"
			sample.ArgsOmitted = maxSafeInteger
			sample.Args = make([]synthetic.BrowserArgument, 4)
			for j := range sample.Args {
				sample.Args[j].Value = text
			}
		case "transport":
			sample.Method = method
			sample.URL = &address
			sample.Error = strings.Repeat("x", 126)
		case "http_error":
			sample.Method = method
			sample.URL = &address
			sample.Status = maxSafeInteger
		}
		retained.Samples = append(retained.Samples, sample)
		if i%4 < 3 {
			newSamples = append(newSamples, sample)
		}
	}
	var wireBytes int
	for i := 0; i < 64; i++ {
		row := synthetic.BrowserTest{ID: id, State: "observed", Counts: &aggregate.Counts}
		if i == 0 {
			for j := range retained.Samples {
				row.SampleIDs = append(row.SampleIDs, j+1)
			}
		}
		retained.Tests = append(retained.Tests, row)
		data := map[string]any{"record": "test", "run": aggregate, "test": row}
		raw, err := json.Marshal(data)
		require.NoError(t, err)
		wireBytes += len(raw) + len(`{"type":"extension","extension":{"feature":"browser_summary.v1","data":}}`) + 1
	}
	data, err := json.Marshal(map[string]any{"record": "test", "run": aggregate, "test": retained.Tests[0], "samples": newSamples})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(data), 16<<10)
	raw, err := json.Marshal(retained)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(raw), 40<<10)
	samples, err := json.Marshal(retained.Samples)
	require.NoError(t, err)
	aggregateJSON, err := json.Marshal(aggregate)
	require.NoError(t, err)
	wireBytes += len(samples) + 8*(len(aggregateJSON)+300) + len(browserInit)
	assert.LessOrEqual(t, wireBytes, 72<<10)
}

func TestBrowserRepresentativePoolAndReferenceOwnership(t *testing.T) {
	lines := []string{browserInit}
	for n := int64(1); n <= 4; n++ {
		record := browserTestRecord(n)
		row := record["test"].(synthetic.BrowserTest)
		row.SampleIDs = []int{int(n)}
		record["test"] = row
		record["samples"] = []synthetic.BrowserSample{{ID: int(n), TestID: row.ID, Category: "uncaught", Message: fmt.Sprint(n)}}
		lines = append(lines, browserRecord(t, record))
	}
	s := consumeBrowser(t, lines...)
	require.Equal(t, "partial", s.browser.ReportingStatus())
	require.Len(t, s.browser.Samples, 4)
	fifth := browserTestRecord(5)
	row := fifth["test"].(synthetic.BrowserTest)
	row.SampleIDs = []int{5}
	fifth["test"] = row
	fifth["samples"] = []synthetic.BrowserSample{{ID: 5, TestID: row.ID, Category: "uncaught", Message: "new"}}
	s = consumeBrowser(t, append(lines, browserRecord(t, fifth))...)
	assert.Equal(t, "invalid", s.browser.ReportingStatus())
	assert.Len(t, s.browser.Tests, 4)
	assert.Len(t, s.browser.Samples, 4)
	fifth["samples"] = nil
	row.SampleIDs = []int{1}
	fifth["test"] = row
	s = consumeBrowser(t, append(lines, browserRecord(t, fifth))...)
	assert.Equal(t, "invalid", s.browser.ReportingStatus())
}
func TestBrowserPeerInvalidityAndUnknownAdditiveFields(t *testing.T) {
	complete := map[string]any{"record": "complete", "run": browserRun(1), "future": strings.Repeat("secret", 1024)}
	s := consumeBrowser(t, browserInit, evidenceRecord("unknown", "", nil), browserRecord(t, complete))
	assert.Equal(t, "invalid", s.evidence.ReportingStatus())
	assert.Equal(t, "complete", s.browser.ReportingStatus())
	raw, err := json.Marshal(s.browser)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "secret")
}

func TestBrowserAdditionalRedactionMayConvergeRepresentatives(t *testing.T) {
	record := map[string]any{"record": "test", "run": synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: synthetic.BrowserCounts{Uncaught: 2}}, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &synthetic.BrowserCounts{Uncaught: 2}, SampleIDs: []int{1, 2}}, "samples": []synthetic.BrowserSample{{ID: 1, TestID: "t", Category: "uncaught", Message: "token=first"}, {ID: 2, TestID: "t", Category: "uncaught", Message: "token=second"}}}
	s := consumeBrowser(t, browserInit, browserRecord(t, record))
	require.Equal(t, "partial", s.browser.ReportingStatus())
	require.Len(t, s.browser.Samples, 2)
	assert.Equal(t, "token=[REDACTED]", s.browser.Samples[0].Message)
	assert.Equal(t, s.browser.Samples[0].Message, s.browser.Samples[1].Message)
}

func TestBrowserWholeTextOmission(t *testing.T) {
	record := func(sample synthetic.BrowserSample) map[string]any {
		return map[string]any{"record": "test", "run": synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: synthetic.BrowserCounts{ConsoleError: 1}}, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &synthetic.BrowserCounts{ConsoleError: 1}, SampleIDs: []int{1}}, "samples": []synthetic.BrowserSample{sample}}
	}
	sample := synthetic.BrowserSample{ID: 1, TestID: "t", Category: "console_error", State: "unavailable", TextOmitted: "too_long"}
	s := consumeBrowser(t, browserInit, browserRecord(t, record(sample)))
	require.Equal(t, "partial", s.browser.ReportingStatus())
	assert.Equal(t, "too_long", s.browser.Samples[0].TextOmitted)
	for name, bad := range map[string]synthetic.BrowserSample{
		"resolved":       {ID: 1, TestID: "t", Category: "console_error", State: "resolved", TextOmitted: "too_long"},
		"pending":        {ID: 1, TestID: "t", Category: "console_error", State: "pending", TextOmitted: "too_long"},
		"wrong reason":   {ID: 1, TestID: "t", Category: "console_error", State: "unavailable", TextOmitted: "object"},
		"wrong category": {ID: 1, TestID: "t", Category: "uncaught", Message: "error", TextOmitted: "too_long"},
		"invented args":  {ID: 1, TestID: "t", Category: "console_error", State: "unavailable", TextOmitted: "too_long", Args: []synthetic.BrowserArgument{{Value: "no"}}},
	} {
		t.Run(name, func(t *testing.T) {
			s := consumeBrowser(t, browserInit, browserRecord(t, record(bad)))
			assert.Equal(t, "invalid", s.browser.ReportingStatus())
			assert.Empty(t, s.browser.Samples)
		})
	}
	// Exact known-secret replacement can expand a valid whole browser text beyond
	// its encoded budget. Preserve why it disappeared without changing canonical
	// aggregate flags that later producer records must validate against.
	text := strings.Repeat("secret", 80)
	sample = synthetic.BrowserSample{ID: 1, TestID: "t", Category: "console_error", State: "resolved", Text: &text}
	complete := map[string]any{"record": "complete", "run": synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: synthetic.BrowserCounts{ConsoleError: 1}}}
	s = consumeBrowser(t, browserInit, browserRecord(t, record(sample)), browserRecord(t, complete))
	require.Equal(t, "complete", s.browser.ReportingStatus())
	require.Len(t, s.browser.Samples, 1)
	assert.Nil(t, s.browser.Samples[0].Text)
	assert.Equal(t, "unavailable", s.browser.Samples[0].State)
	assert.Equal(t, "too_long", s.browser.Samples[0].TextOmitted)
	assert.Empty(t, s.browser.Run.Omissions)
	assert.Empty(t, s.browser.Truncated)
}
func TestBrowserEmbeddedURLSanitization(t *testing.T) {
	r := newRedactor(nil)
	assert.Equal(t, "failed https://example.com/assets response", browserSafeText("failed https://user:password@example.com/assets?tracking=private#fragment response", r))
	assert.Equal(t, "https://example.com/ https://second.example.com/path", browserSafeText("https://first:pass@example.com/?q=1 https://second.example.com/path#private", r))
}
