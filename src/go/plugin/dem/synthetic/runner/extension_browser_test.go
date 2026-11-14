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

func browserRun(n int64) synthetic.BrowserAggregate {
	return synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: n}, Counts: synthetic.BrowserCounts{Uncaught: n}}
}

func browserTestRecord(n int64) map[string]any {
	return map[string]any{"record": "test", "test": synthetic.BrowserTest{ID: fmt.Sprintf("t%d", n), State: "observed", Counts: &synthetic.BrowserCounts{Uncaught: 1}}, "run": browserRun(n)}
}

func consumeBrowser(t *testing.T, lines ...string) sinkSnapshot {
	t.Helper()
	request := synthetic.Request{Kind: synthetic.Journey, BrowserErrorSummary: true}
	return consume(t, request, map[string]string{"DEM_SECRET_X": "secret"}, lines...)
}

func TestBrowserFeatureIsolationAndNegotiation(t *testing.T) {
	for _, body := range []any{nil, 3, []any{}, map[string]any{"record": "future"}, map[string]any{"record": "complete", "run": 3}, map[string]any{"record": "complete", "run": map[string]any{"tests": map[string]any{"observed": 1.5}}}} {
		t.Run(fmt.Sprint(body), func(t *testing.T) {
			s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, body), evidenceRecord("complete", "", nil))
			assert.Equal(t, "invalid", s.browser.ReportingStatus())
			assert.Equal(t, "complete", s.evidence.ReportingStatus())
			assert.NoError(t, s.err)
		})
	}
	for name, lines := range map[string][]string{"old": {}, "declined": {`{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1"]}}`}, "before init": {extensionLine(browserSummaryFeature, map[string]any{"record": "complete", "run": browserRun(0)})}, "duplicate init": {browserInit, browserInit}, "shared invalid": {browserInit, `{"type":"extension","extension":{"feature":"browser_summary.v1","future":true,"data":{}}}`}} {
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
	s := newSink(synthetic.Request{Kind: synthetic.Journey}, newRedactor(nil))
	require.NoError(t, s.consume(strings.NewReader(evidenceInit+"\n"+extensionLine(browserSummaryFeature, 3)+"\n"+evidenceRecord("complete", "", nil))))
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
			s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, initial), extensionLine(browserSummaryFeature, bad))
			assert.Equal(t, "invalid", s.browser.ReportingStatus())
			require.Len(t, s.browser.Tests, 1)
			assert.Empty(t, s.browser.Samples)
			assert.Equal(t, browserRun(1), *s.browser.Run)
		})
	}
	s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, map[string]any{"record": "complete", "run": browserRun(0)}), extensionLine(browserSummaryFeature, map[string]any{"record": "complete", "run": browserRun(0)}))
	assert.Equal(t, "invalid", s.browser.ReportingStatus())
	assert.False(t, s.browser.ReporterComplete)
}

func TestBrowserRedactionSamplesAndSnapshots(t *testing.T) {
	raw := map[string]any{"record": "test", "run": synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: synthetic.BrowserCounts{HTTPError: 1, ConsoleError: 1}}, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &synthetic.BrowserCounts{HTTPError: 1, ConsoleError: 1}, SampleIDs: []int{1, 2}}, "samples": []any{map[string]any{"id": 1, "test_id": "t", "category": "http_error", "method": "GET", "url": "https://user:pass@example.com/private?q=secret#frag", "status": 1200}, map[string]any{"id": 2, "test_id": "t", "category": "console_error", "state": "resolved", "args": []any{map[string]any{"value": "secret"}, map[string]any{"value": nil}, map[string]any{"omitted": "unrepresentable"}}}}}
	s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, raw))
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
				extensionLine(browserSummaryFeature, map[string]any{"record": "test", "run": run, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &counts, SampleIDs: []int{1}}, "samples": []any{sample}}),
				extensionLine(browserSummaryFeature, map[string]any{"record": "complete", "run": run}), evidenceRecord("complete", "", nil))
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
		lines = append(lines, extensionLine(browserSummaryFeature, browserTestRecord(n)))
	}
	sample := synthetic.BrowserSample{ID: 1, TestID: "late", Category: "uncaught", Message: "late error"}
	lines = append(lines, extensionLine(browserSummaryFeature, map[string]any{"record": "late_sample", "run": browserRun(65), "sample": sample}))
	s := consumeBrowser(t, lines...)
	require.Equal(t, "partial", s.browser.ReportingStatus())
	require.Len(t, s.browser.Tests, 64)
	require.Len(t, s.browser.Samples, 1)
	lines = append(lines, extensionLine(browserSummaryFeature, map[string]any{"record": "late_sample", "run": browserRun(66), "sample": synthetic.BrowserSample{ID: 2, TestID: "other", Category: "uncaught", Message: "another"}}))
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
			s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, base(sample)))
			assert.Equal(t, "invalid", s.browser.ReportingStatus())
			assert.Nil(t, s.browser.Run)
			assert.Empty(t, s.browser.Samples)
		})
	}
	// Count arithmetic is field-wise; independent categories may exceed a safe
	// integer when summed, but no reported field may exceed that boundary.
	counts := synthetic.BrowserCounts{Uncaught: maxSafeInteger, ConsoleError: maxSafeInteger}
	aggregate := synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: counts}
	s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, map[string]any{"record": "test", "run": aggregate, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &counts}}))
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
		lines = append(lines, extensionLine(browserSummaryFeature, record))
	}
	s := consumeBrowser(t, lines...)
	require.Equal(t, "partial", s.browser.ReportingStatus())
	require.Len(t, s.browser.Samples, 4)
	fifth := browserTestRecord(5)
	row := fifth["test"].(synthetic.BrowserTest)
	row.SampleIDs = []int{5}
	fifth["test"] = row
	fifth["samples"] = []synthetic.BrowserSample{{ID: 5, TestID: row.ID, Category: "uncaught", Message: "new"}}
	s = consumeBrowser(t, append(lines, extensionLine(browserSummaryFeature, fifth))...)
	assert.Equal(t, "invalid", s.browser.ReportingStatus())
	assert.Len(t, s.browser.Tests, 4)
	assert.Len(t, s.browser.Samples, 4)
	fifth["samples"] = nil
	row.SampleIDs = []int{1}
	fifth["test"] = row
	s = consumeBrowser(t, append(lines, extensionLine(browserSummaryFeature, fifth))...)
	assert.Equal(t, "invalid", s.browser.ReportingStatus())
}

func TestBrowserPeerInvalidityAndUnknownAdditiveFields(t *testing.T) {
	complete := map[string]any{"record": "complete", "run": browserRun(1), "future": strings.Repeat("secret", 1024)}
	s := consumeBrowser(t, browserInit, evidenceRecord("unknown", "", nil), extensionLine(browserSummaryFeature, complete))
	assert.Equal(t, "invalid", s.evidence.ReportingStatus())
	assert.Equal(t, "complete", s.browser.ReportingStatus())
	raw, err := json.Marshal(s.browser)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "secret")
}

func TestBrowserAdditionalRedactionMayConvergeRepresentatives(t *testing.T) {
	record := map[string]any{"record": "test", "run": synthetic.BrowserAggregate{Tests: synthetic.BrowserTestCounts{Observed: 1}, Counts: synthetic.BrowserCounts{Uncaught: 2}}, "test": synthetic.BrowserTest{ID: "t", State: "observed", Counts: &synthetic.BrowserCounts{Uncaught: 2}, SampleIDs: []int{1, 2}}, "samples": []synthetic.BrowserSample{{ID: 1, TestID: "t", Category: "uncaught", Message: "token=first"}, {ID: 2, TestID: "t", Category: "uncaught", Message: "token=second"}}}
	s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, record))
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
	s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, record(sample)))
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
			s := consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, record(bad)))
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
	s = consumeBrowser(t, browserInit, extensionLine(browserSummaryFeature, record(sample)), extensionLine(browserSummaryFeature, complete))
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

// browserStream builds cumulative browser_summary.v1 records that are valid
// unless a case deliberately breaks one bound.
type browserStream struct {
	t     *testing.T
	run   synthetic.BrowserAggregate
	lines []string
}

func newBrowserStream(t *testing.T) *browserStream {
	return &browserStream{t: t, lines: []string{browserInit}}
}

func (s *browserStream) observe(category string) {
	switch category {
	case "uncaught":
		s.run.Counts.Uncaught++
	case "console_error":
		s.run.Counts.ConsoleError++
	case "transport":
		s.run.Counts.Transport++
	case "http_error":
		s.run.Counts.HTTPError++
	}
}

func (s *browserStream) test(id string, samples ...synthetic.BrowserSample) *browserStream {
	counts := synthetic.BrowserCounts{}
	var ids []int
	for i := range samples {
		samples[i].TestID = id
		ids = append(ids, samples[i].ID)
		switch samples[i].Category {
		case "uncaught":
			counts.Uncaught++
		case "console_error":
			counts.ConsoleError++
		case "transport":
			counts.Transport++
		case "http_error":
			counts.HTTPError++
		}
		s.observe(samples[i].Category)
	}
	if counts == (synthetic.BrowserCounts{}) {
		counts.Uncaught = 1
		s.observe("uncaught")
	}
	s.run.Tests.Observed++
	data := map[string]any{"record": "test", "run": s.run, "test": synthetic.BrowserTest{ID: id, State: "observed", Counts: &counts, SampleIDs: ids}}
	if len(samples) > 0 {
		data["samples"] = samples
	}
	s.lines = append(s.lines, extensionLine(browserSummaryFeature, data))
	return s
}

func (s *browserStream) rows(n int) *browserStream {
	for i := range n {
		s.test(fmt.Sprintf("row%d", i))
	}
	return s
}

func (s *browserStream) late(sample synthetic.BrowserSample) *browserStream {
	if sample.TestID == "" {
		sample.TestID = fmt.Sprintf("late%d", sample.ID)
	}
	s.run.Tests.Observed++
	s.observe(sample.Category)
	s.lines = append(s.lines, extensionLine(browserSummaryFeature, map[string]any{"record": "late_sample", "run": s.run, "sample": sample}))
	return s
}

func (s *browserStream) status() string {
	return consumeBrowser(s.t, s.lines...).browser.ReportingStatus()
}

func uncaughtSample(id int, message string) synthetic.BrowserSample {
	return synthetic.BrowserSample{ID: id, Category: "uncaught", Message: message}
}

func consoleSample(id int, args ...synthetic.BrowserArgument) synthetic.BrowserSample {
	return synthetic.BrowserSample{ID: id, Category: "console_error", State: "resolved", Args: args}
}

func consoleTextSample(id int, text string) synthetic.BrowserSample {
	return synthetic.BrowserSample{ID: id, Category: "console_error", State: "resolved", Text: &text}
}

func requestSample(id int, category, method, address, errText string) synthetic.BrowserSample {
	sample := synthetic.BrowserSample{ID: id, Category: category, Method: method, URL: &address, Error: errText}
	if category == "http_error" {
		sample.Status = 500
	}
	return sample
}

func categorySamples(firstID, perCategory int) []synthetic.BrowserSample {
	var samples []synthetic.BrowserSample
	id := firstID
	for _, category := range []string{"uncaught", "console_error", "transport", "http_error"} {
		for range perCategory {
			switch category {
			case "uncaught":
				samples = append(samples, uncaughtSample(id, fmt.Sprint(id)))
			case "console_error":
				samples = append(samples, consoleTextSample(id, fmt.Sprint(id)))
			default:
				samples = append(samples, requestSample(id, category, "GET", fmt.Sprintf("https://example.com/%d", id), ""))
			}
			id++
		}
	}
	return samples
}

// TestBrowserWireBounds accepts each browser_summary.v1 quota at its limit and
// invalidates the feature one past it.
func TestBrowserWireBounds(t *testing.T) {
	encodedText := func(n int) string { return strings.Repeat("x", n-2) }
	address := func(n int) string {
		return "https://example.com/" + strings.Repeat("x", n-2-len("https://example.com/"))
	}
	arg := synthetic.BrowserArgument{Value: "a"}
	cases := map[string]struct {
		stream func(*browserStream)
		want   string
	}{
		"rows at limit":             {func(s *browserStream) { s.rows(64) }, "partial"},
		"rows past limit":           {func(s *browserStream) { s.rows(65) }, "invalid"},
		"record samples at limit":   {func(s *browserStream) { s.test("t", categorySamples(1, 3)...) }, "partial"},
		"record samples past limit": {func(s *browserStream) { s.test("t", append(categorySamples(1, 3), uncaughtSample(13, "x"))...) }, "invalid"},
		"record category at limit": {func(s *browserStream) {
			s.test("t", uncaughtSample(1, "a"), uncaughtSample(2, "b"), uncaughtSample(3, "c"))
		}, "partial"},
		"record category past limit": {func(s *browserStream) {
			s.test("t", uncaughtSample(1, "a"), uncaughtSample(2, "b"), uncaughtSample(3, "c"), uncaughtSample(4, "d"))
		}, "invalid"},
		"pool category at limit": {func(s *browserStream) { s.test("a", categorySamples(1, 3)...).test("b", uncaughtSample(13, "x")) }, "partial"},
		"pool category past limit": {func(s *browserStream) {
			s.test("a", categorySamples(1, 3)...).test("b", uncaughtSample(13, "x")).test("c", uncaughtSample(14, "y"))
		}, "invalid"},
		"pool full": {func(s *browserStream) { s.test("a", categorySamples(1, 3)...).test("b", categorySamples(13, 1)...) }, "partial"},
		"pool past full": {func(s *browserStream) {
			s.test("a", categorySamples(1, 3)...).test("b", categorySamples(13, 1)...).test("c", uncaughtSample(17, "x"))
		}, "invalid"},
		"sample ID at limit":   {func(s *browserStream) { s.test("t", uncaughtSample(16, "x")) }, "partial"},
		"sample ID past limit": {func(s *browserStream) { s.test("t", uncaughtSample(17, "x")) }, "invalid"},
		"sample ID zero":       {func(s *browserStream) { s.test("t", uncaughtSample(0, "x")) }, "invalid"},
		"late samples at limit": {func(s *browserStream) {
			s.rows(64)
			for _, sample := range categorySamples(1, 1) {
				s.late(sample)
			}
		}, "partial"},
		"late samples past limit": {func(s *browserStream) {
			s.rows(64)
			for _, sample := range categorySamples(1, 1) {
				s.late(sample)
			}
			s.late(uncaughtSample(5, "x"))
		}, "invalid"},
		"late sample before row cap":   {func(s *browserStream) { s.rows(63).late(uncaughtSample(1, "x")) }, "invalid"},
		"late sample in used category": {func(s *browserStream) { s.test("a", uncaughtSample(1, "x")).rows(63).late(uncaughtSample(2, "y")) }, "invalid"},
		"console args at limit":        {func(s *browserStream) { s.test("t", consoleSample(1, arg, arg, arg, arg)) }, "partial"},
		"console args past limit":      {func(s *browserStream) { s.test("t", consoleSample(1, arg, arg, arg, arg, arg)) }, "invalid"},
		"message at limit":             {func(s *browserStream) { s.test("t", uncaughtSample(1, encodedText(512))) }, "partial"},
		"message past limit":           {func(s *browserStream) { s.test("t", uncaughtSample(1, encodedText(513))) }, "invalid"},
		"console text at limit":        {func(s *browserStream) { s.test("t", consoleTextSample(1, encodedText(512))) }, "partial"},
		"console text past limit":      {func(s *browserStream) { s.test("t", consoleTextSample(1, encodedText(513))) }, "invalid"},
		"argument at limit": {func(s *browserStream) {
			s.test("t", consoleSample(1, synthetic.BrowserArgument{Value: encodedText(512)}))
		}, "partial"},
		"argument past limit": {func(s *browserStream) {
			s.test("t", consoleSample(1, synthetic.BrowserArgument{Value: encodedText(513)}))
		}, "invalid"},
		"URL at limit":   {func(s *browserStream) { s.test("t", requestSample(1, "http_error", "GET", address(256), "")) }, "partial"},
		"URL past limit": {func(s *browserStream) { s.test("t", requestSample(1, "http_error", "GET", address(257), "")) }, "invalid"},
		"error at limit": {func(s *browserStream) {
			s.test("t", requestSample(1, "transport", "GET", "https://example.com/", encodedText(128)))
		}, "partial"},
		"error past limit": {func(s *browserStream) {
			s.test("t", requestSample(1, "transport", "GET", "https://example.com/", encodedText(129)))
		}, "invalid"},
		"method at limit": {func(s *browserStream) {
			s.test("t", requestSample(1, "transport", encodedText(16), "https://example.com/", ""))
		}, "partial"},
		"method past limit": {func(s *browserStream) {
			s.test("t", requestSample(1, "transport", encodedText(17), "https://example.com/", ""))
		}, "invalid"},
		"row ID at limit":   {func(s *browserStream) { s.test(encodedText(128)) }, "partial"},
		"row ID past limit": {func(s *browserStream) { s.test(encodedText(129)) }, "invalid"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newBrowserStream(t)
			tc.stream(s)
			assert.Equal(t, tc.want, s.status())
		})
	}
}

// TestBrowserWireRejectsNullAndAmbiguousMembers pins encodings the producer
// never emits but untyped decoding would read as zero or as an absent value.
func TestBrowserWireRejectsNullAndAmbiguousMembers(t *testing.T) {
	sample := func(fields string) string {
		return `{"record":"test","run":{"tests":{"observed":1},"counts":{"console_error":1}},"test":{"id":"t","state":"observed","counts":{"console_error":1},"sample_ids":[1]},"samples":[{"test_id":"t","category":"console_error","state":"resolved",` + fields + `}]}`
	}
	cases := map[string]struct {
		data string
		want string
	}{
		"sparse aggregate":         {`{"record":"complete","run":{"tests":{},"counts":{}}}`, "complete"},
		"null run counter":         {`{"record":"complete","run":{"tests":{},"counts":{"uncaught":null}}}`, "invalid"},
		"null test counter":        {`{"record":"complete","run":{"tests":{"observed":null},"counts":{}}}`, "invalid"},
		"null run tests":           {`{"record":"complete","run":{"tests":null,"counts":{}}}`, "invalid"},
		"null run counts":          {`{"record":"complete","run":{"tests":{},"counts":null}}`, "invalid"},
		"missing run counts":       {`{"record":"complete","run":{"tests":{}}}`, "invalid"},
		"missing run":              {`{"record":"complete"}`, "invalid"},
		"null row counter":         {`{"record":"test","run":{"tests":{"observed":1},"counts":{"uncaught":1}},"test":{"id":"t","state":"observed","counts":{"uncaught":null}}}`, "invalid"},
		"null incomplete counts":   {`{"record":"test","run":{"tests":{"incomplete":1},"counts":{}},"test":{"id":"t","state":"incomplete","counts":null}}`, "partial"},
		"valid sample":             {sample(`"id":1,"args":[{"value":null},{"omitted":"object"}]`), "partial"},
		"null sample ID":           {sample(`"id":null,"args":[{"value":1}]`), "invalid"},
		"null args omitted":        {sample(`"id":1,"args":[{"value":1}],"args_omitted":null`), "invalid"},
		"null status":              {sample(`"id":1,"args":[{"value":1}],"status":null`), "invalid"},
		"argument without members": {sample(`"id":1,"args":[{}]`), "invalid"},
		"argument with both":       {sample(`"id":1,"args":[{"value":1,"omitted":"object"}]`), "invalid"},
		"null omission":            {sample(`"id":1,"args":[{"omitted":null}]`), "invalid"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			line := `{"type":"extension","extension":{"feature":"browser_summary.v1","data":` + tc.data + `}}`
			s := consumeBrowser(t, browserInit, line)
			assert.Equal(t, tc.want, s.browser.ReportingStatus())
		})
	}
}

// TestBrowserRowStates pins which test-row states, counts and reasons a
// test record may carry.
func TestBrowserRowStates(t *testing.T) {
	// Errors are counted only for observed fixture contexts.
	record := func(state, extra string) string {
		run := `"tests":{"observed":1},"counts":{"uncaught":1}`
		switch state {
		case "incomplete", "invalid":
			run = `"tests":{"` + state + `":1},"counts":{}`
		}
		data := `{"record":"test","run":{` + run + `},"test":{"id":"t","state":"` + state + `"` + extra + `}}`
		return `{"type":"extension","extension":{"feature":"browser_summary.v1","data":` + data + `}}`
	}
	for name, tc := range map[string]struct {
		line string
		want string
	}{
		"observed":                     {record("observed", `,"counts":{"uncaught":1}`), "partial"},
		"observed without counts":      {record("observed", ``), "invalid"},
		"observed with zero counts":    {record("observed", `,"counts":{}`), "invalid"},
		"observed with reason":         {record("observed", `,"counts":{"uncaught":1},"reason":"invalid_annotation"`), "invalid"},
		"observed with unsafe count":   {record("observed", `,"counts":{"uncaught":9007199254740992}`), "invalid"},
		"row counts exceed run counts": {record("observed", `,"counts":{"uncaught":2}`), "invalid"},
		"incomplete":                   {record("incomplete", ``), "partial"},
		"incomplete with counts":       {record("incomplete", `,"counts":{"uncaught":1}`), "invalid"},
		"incomplete with sample IDs":   {record("incomplete", `,"sample_ids":[1]`), "invalid"},
		"incomplete with reason":       {record("incomplete", `,"reason":"invalid_annotation"`), "invalid"},
		"invalid":                      {record("invalid", ``), "partial"},
		"invalid annotation":           {record("invalid", `,"reason":"invalid_annotation"`), "partial"},
		"duplicate annotation":         {record("invalid", `,"reason":"duplicate_annotation"`), "partial"},
		"invalid with unknown reason":  {record("invalid", `,"reason":"other"`), "invalid"},
		"invalid with counts":          {record("invalid", `,"counts":{"uncaught":1}`), "invalid"},
		"unknown state":                {record("future", `,"counts":{"uncaught":1}`), "invalid"},
		"row class does not advance":   {strings.Replace(record("incomplete", ``), `"incomplete":1`, `"invalid":1`, 1), "invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, consumeBrowser(t, browserInit, tc.line).browser.ReportingStatus())
		})
	}
}
