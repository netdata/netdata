// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type textRedactor struct {
	known    []string
	longest  int
	exact    *strings.Replacer
	patterns *redact.Redactor
}

func newRedactor(values map[string]string) *textRedactor {
	var known []string
	for _, value := range values {
		if value != "" {
			known = append(known, value)
		}
	}
	sort.Slice(known, func(i, j int) bool { return len(known[i]) > len(known[j]) })
	var pairs []string
	for _, value := range known {
		pairs = append(pairs, value, "[REDACTED]")
	}
	longest := 1
	for _, value := range known {
		longest = max(longest, len(value))
	}
	return &textRedactor{
		known:    known,
		longest:  longest,
		exact:    strings.NewReplacer(pairs...),
		patterns: redact.NewRedactor(),
	}
}
func (r *textRedactor) text(value string) string {
	return clip(r.redacted(value))
}

// redacted leaves clipping to typed consumers that must disclose it.
func (r *textRedactor) redacted(value string) string {
	value = r.patterns.Apply(r.exact.Replace(value))
	// An upstream text cap or cancellation may remove a PEM closing marker.
	// Retain no suffix of an unfinished block just because it no longer matches.
	if start := strings.Index(strings.ToLower(value), "-----begin "); start >= 0 {
		value = value[:start] + "[REDACTED]"
	}
	return value
}
func clip(s string) string {
	if utf8.RuneCountInString(s) <= TextLimit {
		return s
	}
	return string([]rune(s)[:TextLimit])
}

// result is terminal reporter evidence, not process-tree completion evidence.
type reporterResult struct {
	Status               synthetic.Outcome           `json:"status"`
	TerminationReason    synthetic.TerminationReason `json:"termination_reason,omitempty"`
	InterruptedArtifacts []synthetic.Capture         `json:"interrupted_artifacts,omitempty"`
	Error                string                      `json:"error,omitempty"`
	Tests                *synthetic.TestCounts       `json:"tests,omitempty"`
	Metrics              *synthetic.LabMetrics       `json:"metrics,omitempty"`
	Artifacts            []synthetic.Capture         `json:"artifacts,omitempty"`
	CaptureState         string                      `json:"capture_state"`
}

// wireEnvelope separates strict core decoding from optional-body validation.
// Raw optional data never crosses the sink boundary.
type wireEnvelope struct {
	Type          string          `json:"type"`
	Event         json.RawMessage `json:"event,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
	ExtensionInit json.RawMessage `json:"extension_init,omitempty"`
	Extension     json.RawMessage `json:"extension,omitempty"`
}

type sink struct {
	extensions   *extensions
	streams      map[string]*diagnosticStream
	streamEvents map[string]synthetic.Event
	mu           sync.Mutex
	kind         synthetic.Kind
	capture      bool
	trace        bool
	redact       *textRedactor
	events       []synthetic.Event
	dropped      int
	result       *reporterResult
	err          error
	declared     int
	tests        map[string]synthetic.Event
}
type sinkSnapshot struct {
	captures *synthetic.CaptureReport
	browser  *synthetic.BrowserSummary
	evidence *synthetic.FailureEvidence
	events   []synthetic.Event
	dropped  int
	result   *reporterResult
	err      error
	partial  *synthetic.TestCounts
}

// newSink reads one attempt's output; the request decides which results and
// optional outputs are legal.
func newSink(request synthetic.Request, redact *textRedactor) *sink {
	return &sink{
		extensions:   newExtensions(request),
		streams:      make(map[string]*diagnosticStream),
		streamEvents: make(map[string]synthetic.Event),
		kind:         request.Kind,
		capture:      request.Capture,
		trace:        request.Trace,
		redact:       redact,
		events:       []synthetic.Event{},
		tests:        make(map[string]synthetic.Event),
	}
}
func (s *sink) consume(reader io.Reader) error {
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, stream := range s.streams {
			stream.end()
		}
	}()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), FrameMaxBytes+1)
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) > FrameMaxBytes {
			return s.fail(errors.New("runner frame exceeds 4 MiB transport limit"))
		}
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		var record wireEnvelope
		if err := decoder.Decode(&record); err != nil {
			return s.fail(fmt.Errorf("invalid runner protocol frame: %w", err))
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return s.fail(errors.New("multiple values in runner protocol frame"))
		}
		s.mu.Lock()
		if s.result != nil {
			s.mu.Unlock()
			return s.fail(errors.New("runner sent a frame after its terminal result"))
		}
		var protocolErr error
		switch record.Type {
		case "event":
			if record.Result != nil || record.ExtensionInit != nil || record.Extension != nil {
				protocolErr = errors.New("invalid runner event envelope")
				break
			}
			var event synthetic.Event
			if err := decodeCore(record.Event, &event); err != nil {
				protocolErr = err
			} else if err := validateEvent(event); err != nil {
				protocolErr = err
			} else {
				s.extensions.core()
				s.add(event)
			}
		case "result":
			if record.Event != nil || record.ExtensionInit != nil || record.Extension != nil {
				protocolErr = errors.New("invalid runner result envelope")
				break
			}
			var result reporterResult
			if err := decodeCore(record.Result, &result); err != nil {
				protocolErr = err
			} else if err := validateResult(s.kind, s.capture, s.trace, &result); err != nil {
				protocolErr = err
			} else {
				s.extensions.core()
				result.Error = s.redact.text(result.Error)
				s.result = &result
			}
		case "extension_init", "extension":
			if record.Event != nil || record.Result != nil ||
				record.Type == "extension_init" && record.Extension != nil ||
				record.Type == "extension" && record.ExtensionInit != nil {
				protocolErr = errors.New("mixed runner extension envelope")
				break
			}
			if record.Type == "extension_init" {
				s.extensions.init(record.ExtensionInit)
			} else {
				s.extensions.accept(record.Extension, s.redact)
			}
		default:
			protocolErr = errors.New("unsupported runner frame type")
		}
		if protocolErr != nil {
			s.mu.Unlock()
			return s.fail(protocolErr)
		}
		s.mu.Unlock()
	}
	if err := scanner.Err(); err != nil {
		return s.fail(fmt.Errorf("read runner protocol: %w", err))
	}
	return nil
}
func decodeCore(raw json.RawMessage, value any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return errors.New("missing runner core body")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid runner core body: %w", err)
	}
	return nil
}

func (s *sink) fail(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
	return err
}

// add is called under the sink lock. Diagnostic accounting continues after the
// retention cap so truncation cannot manufacture missing test results.
func (s *sink) add(event synthetic.Event) {
	if event.Kind == "stdout" || event.Kind == "stderr" {
		// The two shipped producers are distinct streams; test IDs and callbacks
		// must not split redaction state. Unknown phases share the protocol stream.
		source := event.Phase
		if source != "worker" && source != "cli" {
			source = "protocol"
		}
		key := event.Kind + ":" + source
		s.streamEvents[key] = event
		stream := s.streams[key]
		if stream == nil {
			stream = newDiagnosticStream(s.redact, func(message string) {
				e := s.streamEvents[key]
				e.Message = message
				s.retain(e)
			})
			s.streams[key] = stream
		}
		stream.write(event.Message)
		return
	}
	s.retain(event)
}
func (s *sink) retain(event synthetic.Event) {
	if event.Kind == "suite" && event.Phase == "discovery" {
		s.declared = *event.DeclaredTests
	}
	if event.Kind == "test" && event.TestID != "" {
		s.tests[event.TestID] = event
	}
	event.Title = s.redact.text(event.Title)
	event.Message = s.redact.text(event.Message)
	event.Phase = s.redact.text(event.Phase)
	if len(s.events) < EventLimit {
		s.events = append(s.events, event)
	} else {
		s.dropped++
	}
}
func (s *sink) stderr(reader io.Reader) {
	stream := newDiagnosticStream(s.redact, func(message string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.retain(synthetic.Event{
			Kind:    "stderr",
			AtMS:    time.Now().UnixMilli(),
			Phase:   "bootstrap",
			Message: message,
		})
	})
	defer stream.end()
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			stream.write(string(buffer[:n]))
		}
		if err != nil {
			return
		}
	}
}
func (s *sink) snapshot() sinkSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	var partial *synthetic.TestCounts
	if s.kind == synthetic.Journey && (s.declared > 0 || len(s.tests) > 0) {
		counts := synthetic.TestCounts{
			Declared: max(s.declared, len(s.tests)),
		}
		for _, event := range s.tests {
			if event.Phase != "end" {
				continue
			}
			switch {
			case event.Status == "skipped":
				counts.Skipped++
			case event.Status == "timedOut":
				counts.TimedOut++
			case event.ExpectedStatus == "failed":
				counts.ExpectedFailure++
			case event.Status == "failed":
				counts.Failed++
			case event.Status == "passed" && event.ExpectedStatus == "passed":
				counts.Passed++
			}
		}
		counts.NotRun = counts.Declared - counts.Passed - counts.Failed - counts.TimedOut - counts.Skipped - counts.ExpectedFailure
		partial = &counts
	}
	return sinkSnapshot{
		captures: s.extensions.captures.snapshot(),
		evidence: s.extensions.evidence.snapshot(),
		browser:  s.extensions.browser.snapshot(),
		events:   append([]synthetic.Event{}, s.events...),
		dropped:  s.dropped,
		result:   s.result,
		err:      s.err,
		partial:  partial,
	}
}
func validateEvent(event synthetic.Event) error {
	if event.Kind == "suite" && event.Phase == "discovery" {
		if event.DeclaredTests == nil || *event.DeclaredTests < 0 {
			return errors.New("discovery event requires a nonnegative declared test count")
		}
	} else if event.DeclaredTests != nil {
		return errors.New("declared test count is only valid in a discovery event")
	}
	switch event.Kind {
	case "suite", "test", "step", "error", "stdout", "stderr", "audit":
	default:
		return errors.New("unsupported runner event kind")
	}
	if event.AtMS < 0 || event.DurationMS != nil && (!finite(*event.DurationMS) || *event.DurationMS < 0) {
		return errors.New("invalid runner event time")
	}
	return nil
}
func validateResult(kind synthetic.Kind, capture, trace bool, result *reporterResult) error {
	switch result.Status {
	case synthetic.Success,
		synthetic.Failed,
		synthetic.Timeout,
		synthetic.Inconclusive,
		synthetic.Error,
		synthetic.Cancelled:
	default:
		return errors.New("invalid terminal outcome")
	}
	candidateCount := len(result.Artifacts) + len(result.InterruptedArtifacts)
	if !capture && (result.CaptureState != "disabled" || candidateCount != 0) {
		return errors.New("runner captured content while capture was disabled")
	}
	if capture {
		switch result.CaptureState {
		case "not_needed", "unavailable", "available":
		default:
			return errors.New("invalid capture state")
		}
		if (result.CaptureState == "available") != (candidateCount > 0) {
			return errors.New("capture state contradicts artifact candidates")
		}
	}
	switch result.TerminationReason {
	case "":
	case synthetic.ExecutionBudget, synthetic.TestTimeout, synthetic.SuiteTimeout:
		if kind != synthetic.Journey || result.Status != synthetic.Timeout {
			return errors.New("journey timeout reason contradicts terminal outcome")
		}
	case synthetic.CallerCancelled:
		if result.Status != synthetic.Cancelled {
			return errors.New("cancellation reason contradicts terminal outcome")
		}
	default:
		return errors.New("unsupported runner termination reason")
	}
	if len(result.InterruptedArtifacts) > 0 && (kind != synthetic.Journey || result.TerminationReason != synthetic.ExecutionBudget) {
		return errors.New("interrupted captures require a journey budget stop")
	}
	if kind == synthetic.Journey {
		if result.Metrics != nil {
			return errors.New("journey result contains lab measurements")
		}
		if result.Tests != nil {
			counts := result.Tests
			if counts.Declared < 0 {
				return errors.New("negative declared test count")
			}
			remaining := counts.Declared
			for _, n := range []int{counts.Passed, counts.Failed, counts.TimedOut, counts.Skipped, counts.ExpectedFailure, counts.NotRun} {
				if n < 0 || n > remaining {
					return errors.New("invalid disjoint test counts")
				}
				remaining -= n
			}
			if remaining != 0 {
				return errors.New("test counts do not cover declared tests")
			}
		}
		if result.Status == synthetic.Success &&
			(result.Tests == nil || result.Tests.Declared == 0 || result.Tests.Passed != result.Tests.Declared) {
			return errors.New("journey success requires every declared ordinary test to pass")
		}
		if result.Status == synthetic.Inconclusive &&
			(result.Tests == nil || result.Tests.Failed > 0 || result.Tests.TimedOut > 0 || result.Tests.Declared > 0 && result.Tests.Passed == result.Tests.Declared) {
			return errors.New("inconclusive journey contradicts test outcomes")
		}
		if result.Status == synthetic.Failed && result.Tests != nil && result.Tests.TimedOut > 0 {
			return errors.New("test timeout must dominate unexpected failure")
		}
		if result.Status == synthetic.Failed && (result.Tests == nil || result.Tests.Failed == 0) {
			return errors.New("journey failure lacks an unexpected failed test")
		}
	} else {
		if result.Tests != nil {
			return errors.New("Lighthouse result contains journey counts")
		}
		if result.Status == synthetic.Success && result.Metrics == nil {
			return errors.New("Lighthouse success lacks audit measurements")
		}
		if result.Metrics != nil {
			m := result.Metrics
			for _, value := range []*float64{m.Performance, m.FCPMS, m.LCPMS, m.TBTMS, m.SIMS, m.CLS} {
				if value != nil && (!finite(*value) || *value < 0) {
					return errors.New("invalid lab measurement")
				}
			}
			if m.Performance != nil && *m.Performance > 100 {
				return errors.New("performance points exceed 100")
			}
		}
	}
	for _, a := range result.InterruptedArtifacts {
		if !validJourneyCapture(a, trace) {
			return errors.New("unsupported interrupted journey capture")
		}
	}
	for _, a := range result.Artifacts {
		if kind == synthetic.Journey && !validJourneyCapture(a, trace) {
			return errors.New("unsupported journey capture")
		}
		if kind == synthetic.Lighthouse && (a.Kind != "report" || a.MIME != "text/html") {
			return errors.New("unsupported audit capture")
		}
	}
	return nil
}
func validJourneyCapture(a synthetic.Capture, trace bool) bool {
	return a.Kind == "screenshot" && a.MIME == "image/png" || trace && a.Kind == "trace" && a.MIME == "application/zip"
}
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

type wireRequest struct {
	Extensions        []string          `json:"extensions,omitempty"`
	Version           int               `json:"version"`
	Kind              synthetic.Kind    `json:"kind"`
	RunID             string            `json:"run_id"`
	WorkDir           string            `json:"work_dir"`
	Script            string            `json:"script,omitempty"`
	ScriptPath        string            `json:"script_path,omitempty"`
	URL               string            `json:"url,omitempty"`
	TimeoutMS         int64             `json:"timeout_ms"`
	StopAtMonotonicMS int64             `json:"stop_at_monotonic_ms,omitempty"`
	Capture           bool              `json:"capture"`
	Secrets           map[string]string `json:"secrets,omitempty"`
}
