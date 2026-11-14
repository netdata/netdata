// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// browser_summary.v1 quotas, enforced by the producer
// (src/dem-runtime/runner/browser-{summary,context,sampler}.cjs) and described
// in src/dem-runtime/README.md, "Optional browser-error summary".
const (
	browserRowLimit            = 64 // interesting test rows the reporter retains
	browserCategorySampleLimit = 4  // distinct samples per category in the run pool
	browserCandidateLimit      = 3  // sampler candidates per category and test
	browserArgLimit            = 4  // console arguments per sample

	// Encoded JSON bytes, quotes included.
	browserTextBytes   = 512 // uncaught message, console text and string argument
	browserURLBytes    = 256
	browserErrorBytes  = 128 // transport error
	browserMethodBytes = 16
)

var browserCategories = [...]string{"uncaught", "console_error", "transport", "http_error"}

const (
	// The pool holds browserCategorySampleLimit samples per category; sample
	// IDs number its entries.
	browserSampleLimit = len(browserCategories) * browserCategorySampleLimit
	// One test record introduces up to browserCandidateLimit samples per category.
	browserRecordSampleLimit = len(browserCategories) * browserCandidateLimit
)

var browserFlags = []string{"args_limit", "rpc_budget", "candidate_limit", "pending", "unavailable", "too_long", "pool_full", "row_limit", "identity_limit"}

type browserState struct {
	featureState
	value synthetic.BrowserSummary
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

// Wire types. The producer never sends null counters, an aggregate without both
// count objects, or an argument without exactly one of value and omitted. Plain
// decoding would read those as zero or absent, so these types reject them;
// stored summaries decode into the plain model types.
type (
	wireBrowserCounts     synthetic.BrowserCounts
	wireBrowserTestCounts synthetic.BrowserTestCounts
	wireBrowserArgument   synthetic.BrowserArgument

	wireBrowserAggregate struct {
		Tests     wireBrowserTestCounts `json:"tests"`
		Counts    wireBrowserCounts     `json:"counts"`
		Omissions []string              `json:"omissions"`
	}
	wireBrowserTest struct {
		synthetic.BrowserTest
		Counts *wireBrowserCounts `json:"counts"`
	}
	wireBrowserSample struct {
		synthetic.BrowserSample
		Args []wireBrowserArgument `json:"args"`
	}
)

// rejectNullMembers requires a JSON object whose named members, when present,
// are not null. Absent counters keep their sparse-zero meaning.
func rejectNullMembers(raw []byte, keys ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("missing object")
	}
	for _, key := range keys {
		if string(fields[key]) == "null" {
			return errors.New("null " + key)
		}
	}
	return nil
}

func (c *wireBrowserCounts) UnmarshalJSON(raw []byte) error {
	if err := rejectNullMembers(raw, "uncaught", "console_error", "transport", "http_error"); err != nil {
		return err
	}
	return json.Unmarshal(raw, (*synthetic.BrowserCounts)(c))
}

func (c *wireBrowserTestCounts) UnmarshalJSON(raw []byte) error {
	if err := rejectNullMembers(raw, "observed", "incomplete", "invalid", "no_summary"); err != nil {
		return err
	}
	return json.Unmarshal(raw, (*synthetic.BrowserTestCounts)(c))
}

func (a *wireBrowserAggregate) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if fields["tests"] == nil || fields["counts"] == nil {
		return errors.New("missing aggregate objects")
	}
	type plain wireBrowserAggregate
	return json.Unmarshal(raw, (*plain)(a))
}

func (s *wireBrowserSample) UnmarshalJSON(raw []byte) error {
	if err := rejectNullMembers(raw, "id", "args_omitted", "status"); err != nil {
		return err
	}
	type plain wireBrowserSample
	return json.Unmarshal(raw, (*plain)(s))
}

func (a *wireBrowserArgument) UnmarshalJSON(raw []byte) error {
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

func (a *wireBrowserAggregate) model() *synthetic.BrowserAggregate {
	if a == nil {
		return nil
	}
	return &synthetic.BrowserAggregate{
		Tests:     synthetic.BrowserTestCounts(a.Tests),
		Counts:    synthetic.BrowserCounts(a.Counts),
		Omissions: a.Omissions,
	}
}

func (t *wireBrowserTest) model() *synthetic.BrowserTest {
	if t == nil {
		return nil
	}
	out := t.BrowserTest
	out.Counts = (*synthetic.BrowserCounts)(t.Counts)
	return &out
}

func (s *wireBrowserSample) model() synthetic.BrowserSample {
	out := s.BrowserSample
	if s.Args != nil {
		out.Args = make([]synthetic.BrowserArgument, len(s.Args))
		for i, a := range s.Args {
			out.Args[i] = synthetic.BrowserArgument(a)
		}
	}
	return out
}

// accept decodes one record of a selected, incomplete stream. Validation and
// sanitization stay local until the whole record is accepted.
func (b *browserState) accept(raw json.RawMessage, r *textRedactor) {
	var d struct {
		Record  string                `json:"record"`
		Run     *wireBrowserAggregate `json:"run"`
		Test    *wireBrowserTest      `json:"test"`
		Samples []wireBrowserSample   `json:"samples"`
		Sample  *wireBrowserSample    `json:"sample"`
		Scope   string                `json:"scope"`
		Reason  string                `json:"reason"`
	}
	if json.Unmarshal(raw, &d) != nil {
		b.invalidate("invalid_record")
		return
	}
	run, test := d.Run.model(), d.Test.model()
	var samples []synthetic.BrowserSample
	for i := range d.Samples {
		samples = append(samples, d.Samples[i].model())
	}
	if d.Record == "invalid" {
		// The producer ends the feature; earlier records keep their counts.
		if run == nil && test == nil && d.Sample == nil && len(samples) == 0 && d.Scope == "" &&
			slices.Contains([]string{"counter_overflow", "annotation_limit"}, d.Reason) {
			b.invalidate(d.Reason)
		} else {
			b.invalidate("invalid_record")
		}
		return
	}
	valid := b.aggregate(run) && d.Reason == ""
	if valid {
		switch d.Record {
		case "test":
			valid = d.Sample == nil && d.Scope == "" && b.acceptTest(run, test, samples, r)
		case "late_sample":
			valid = test == nil && len(samples) == 0 && d.Sample != nil && d.Scope == "" && b.acceptLateSample(run, d.Sample.model(), r)
		case "truncated":
			valid = test == nil && d.Sample == nil && len(samples) == 0 &&
				slices.Contains([]string{"tests", "samples", "text"}, d.Scope) && !slices.Contains(b.value.Truncated, d.Scope)
			if valid {
				b.value.Truncated = append(b.value.Truncated, d.Scope)
			}
		case "complete":
			valid = test == nil && d.Sample == nil && len(samples) == 0 && d.Scope == ""
			if valid {
				b.complete()
			}
		default:
			valid = false
		}
	}
	if !valid {
		b.invalidate("invalid_record")
		return
	}
	b.value.Run = run
}

// aggregate checks a record's cumulative run counts against the previous record.
func (b *browserState) aggregate(a *synthetic.BrowserAggregate) bool {
	if a == nil || !safeCounts(countValues(a.Counts)) || !safeCounts(testCountValues(a.Tests)) || !validFlags(a.Omissions, browserFlags) {
		return false
	}
	if _, ok := sumSafe(testCountValues(a.Tests)); !ok {
		return false
	}
	// Errors are counted only in observed fixture contexts.
	if a.Tests.Observed == 0 && slices.ContainsFunc(countValues(a.Counts), func(v int64) bool { return v != 0 }) {
		return false
	}
	old := b.value.Run
	if old == nil {
		return true
	}
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
	return true
}

// acceptTest admits a new row with the samples it introduces. The row's state
// class must advance in the run aggregate, and its counts must fit the
// aggregate's increase.
func (b *browserState) acceptTest(run *synthetic.BrowserAggregate, t *synthetic.BrowserTest, samples []synthetic.BrowserSample, r *textRedactor) bool {
	if t == nil || len(b.value.Tests) >= browserRowLimit || !validEvidenceID(t.ID, testIDBytes) ||
		len(samples) > browserRecordSampleLimit || len(t.SampleIDs) != len(samples) || !validBrowserRow(*t) {
		return false
	}
	if slices.ContainsFunc(b.value.Tests, func(old synthetic.BrowserTest) bool { return old.ID == t.ID }) {
		return false
	}
	var previous synthetic.BrowserAggregate
	if b.value.Run != nil {
		previous = *b.value.Run
	}
	class := map[string]int{"observed": 0, "incomplete": 1, "invalid": 2}[t.State]
	if testCountValues(run.Tests)[class] <= testCountValues(previous.Tests)[class] {
		return false
	}
	if t.State == "observed" {
		if *t.Counts == (synthetic.BrowserCounts{}) {
			return false
		}
		for i, v := range countValues(*t.Counts) {
			if v > countValues(run.Counts)[i]-countValues(previous.Counts)[i] {
				return false
			}
		}
	}
	introduced := map[string]int64{}
	for _, sample := range samples {
		introduced[sample.Category]++
		if t.Counts == nil || browserCategoryCount(*t.Counts, sample.Category) < introduced[sample.Category] {
			return false
		}
	}
	if !b.samples(samples, t.ID, t.SampleIDs, false, r) {
		return false
	}
	b.value.Tests = append(b.value.Tests, *t)
	b.value.Samples = append(b.value.Samples, samples...)
	return true
}

// acceptLateSample admits one sample of a test seen after the rows ran out. It
// must introduce a category absent from the pool, so there is at most one per
// category.
func (b *browserState) acceptLateSample(run *synthetic.BrowserAggregate, sample synthetic.BrowserSample, r *textRedactor) bool {
	if len(b.value.Tests) != browserRowLimit {
		return false
	}
	if slices.ContainsFunc(b.value.Tests, func(test synthetic.BrowserTest) bool { return test.ID == sample.TestID }) {
		return false
	}
	late := []synthetic.BrowserSample{sample}
	if !b.samples(late, sample.TestID, []int{sample.ID}, true, r) || browserCategoryCount(run.Counts, late[0].Category) == 0 {
		return false
	}
	b.value.Samples = append(b.value.Samples, late[0])
	return true
}

// validBrowserRow checks the counts and annotation reason a row state allows.
func validBrowserRow(t synthetic.BrowserTest) bool {
	switch t.State {
	case "observed":
		return t.Counts != nil && safeCounts(countValues(*t.Counts)) && t.Reason == ""
	case "incomplete":
		return t.Counts == nil && len(t.SampleIDs) == 0 && t.Reason == ""
	case "invalid":
		return t.Counts == nil && len(t.SampleIDs) == 0 &&
			(t.Reason == "" || slices.Contains([]string{"invalid_annotation", "duplicate_annotation"}, t.Reason))
	}
	return false
}

// samples validates and sanitizes, in place, the samples a record introduces.
func (b *browserState) samples(samples []synthetic.BrowserSample, testID string, ids []int, late bool, r *textRedactor) bool {
	pooled := map[string]int{}
	seen := map[int]bool{}
	for _, s := range b.value.Samples {
		pooled[s.Category]++
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
		if s.ID < 1 || s.ID > browserSampleLimit || seen[s.ID] || !refs[s.ID] || s.TestID != testID || !validEvidenceID(s.TestID, testIDBytes) ||
			!slices.Contains(browserCategories[:], s.Category) || pooled[s.Category] >= browserCategorySampleLimit || late && pooled[s.Category] > 0 {
			return false
		}
		seen[s.ID] = true
		pooled[s.Category]++
		candidates[s.Category]++
		if candidates[s.Category] > browserCandidateLimit || !validBrowserSample(s, r) {
			return false
		}
	}
	return true
}

// validBrowserSample checks a sample's category-specific shape and sanitizes
// its text in place: secrets are redacted and URLs lose credentials, query and
// fragment.
func validBrowserSample(s *synthetic.BrowserSample, r *textRedactor) bool {
	if s.TextOmitted != "" && (s.Category != "console_error" || s.State != "unavailable" || s.TextOmitted != "too_long" || s.Text != nil || s.Args != nil) {
		return false
	}
	if s.ArgsOmitted < 0 || s.ArgsOmitted > maxSafeInteger || !boundedJSON(s.Message, browserTextBytes) || !boundedJSON(s.Error, browserErrorBytes) ||
		s.Text != nil && !boundedJSON(*s.Text, browserTextBytes) || s.URL != nil && !boundedJSON(*s.URL, browserURLBytes) {
		return false
	}
	switch s.Category {
	case "console_error":
		return validConsoleSample(s, r)
	case "uncaught":
		return validUncaughtSample(s, r)
	case "transport", "http_error":
		return validRequestSample(s, r)
	}
	return false
}

// validConsoleSample: only a resolved console.error has a value, either whole
// text or primitive arguments.
func validConsoleSample(s *synthetic.BrowserSample, r *textRedactor) bool {
	if !slices.Contains([]string{"resolved", "pending", "unavailable"}, s.State) || s.TextTruncated ||
		s.Message != "" || s.Method != "" || s.URL != nil || s.URLOmitted != "" || s.Status != 0 || s.Error != "" {
		return false
	}
	resolved, hasValue := s.State == "resolved", s.Text != nil || len(s.Args) > 0
	if resolved != hasValue || s.Text != nil && len(s.Args) > 0 || len(s.Args) > browserArgLimit {
		return false
	}
	if s.Text != nil {
		// Redaction can push whole text past its budget; omit it rather than clip.
		if v := browserSafeText(*s.Text, r); boundedJSON(v, browserTextBytes) {
			s.Text = &v
		} else {
			s.Text, s.State, s.TextOmitted = nil, "unavailable", "too_long"
		}
	}
	for i := range s.Args {
		if !validConsoleArgument(&s.Args[i], r) {
			return false
		}
	}
	return true
}

func validConsoleArgument(a *synthetic.BrowserArgument, r *textRedactor) bool {
	if a.Omitted != "" {
		return a.Value == nil && slices.Contains([]string{"object", "symbol", "function", "unrepresentable", "non_finite", "too_long"}, a.Omitted)
	}
	switch v := a.Value.(type) {
	case nil, bool:
	case float64:
		return finite(v)
	case string:
		if !boundedJSON(v, browserTextBytes) {
			return false
		}
		if v = browserSafeText(v, r); boundedJSON(v, browserTextBytes) {
			a.Value = v
		} else {
			a.Value, a.Omitted = nil, "too_long"
		}
	default:
		return false
	}
	return true
}

func validUncaughtSample(s *synthetic.BrowserSample, r *textRedactor) bool {
	if s.State != "" || s.Args != nil || s.Text != nil || s.ArgsOmitted != 0 ||
		s.Method != "" || s.URL != nil || s.URLOmitted != "" || s.Status != 0 || s.Error != "" {
		return false
	}
	var clipped bool
	s.Message, clipped = browserText(s.Message, browserTextBytes, r)
	s.TextTruncated = s.TextTruncated || clipped
	return true
}

// validRequestSample covers transport failures and HTTP error responses. An
// oversized URL is omitted whole.
func validRequestSample(s *synthetic.BrowserSample, r *textRedactor) bool {
	if s.State != "" || s.Args != nil || s.Text != nil || s.ArgsOmitted != 0 || s.Message != "" || !boundedJSON(s.Method, browserMethodBytes) ||
		s.URL == nil && s.URLOmitted != "too_long" || s.URL != nil && s.URLOmitted != "" {
		return false
	}
	if s.Category == "http_error" && (s.Status < 400 || s.Status > maxSafeInteger || s.Error != "") || s.Category == "transport" && s.Status != 0 {
		return false
	}
	var clipped bool
	s.Method, clipped = browserText(s.Method, browserMethodBytes, r)
	s.TextTruncated = s.TextTruncated || clipped
	s.Error, clipped = browserText(s.Error, browserErrorBytes, r)
	s.TextTruncated = s.TextTruncated || clipped
	if s.URL != nil {
		if v := browserSafeURL(r.redacted(*s.URL)); boundedJSON(v, browserURLBytes) {
			s.URL = &v
		} else {
			s.URL, s.URLOmitted = nil, "too_long"
		}
	}
	return true
}

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
	return !slices.ContainsFunc(values, func(v int64) bool { return v < 0 || v > maxSafeInteger })
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
