// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

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
		// A fixed order keeps flushed tails and the event cap reproducible.
		for _, key := range slices.Sorted(maps.Keys(s.streams)) {
			s.streams[key].end()
		}
	}()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), frameMaxBytes+1)
	for scanner.Scan() {
		if err := s.frame(scanner.Bytes()); err != nil {
			return s.fail(err)
		}
	}
	if err := scanner.Err(); err != nil {
		return s.fail(fmt.Errorf("read runner protocol: %w", err))
	}
	return nil
}

// frame applies one protocol frame. An error fails the whole stream; invalid
// optional output only invalidates that output.
func (s *sink) frame(raw []byte) error {
	if len(raw) > frameMaxBytes {
		return errors.New("runner frame exceeds 4 MiB transport limit")
	}
	record, err := decodeFrame(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.result != nil {
		return errors.New("runner sent a frame after its terminal result")
	}
	body, err := record.body()
	if err != nil {
		return err
	}
	switch record.Type {
	case "event":
		return s.event(body)
	case "result":
		return s.terminal(body)
	case "extension_init":
		s.extensions.init(body)
	default:
		s.extensions.accept(body, s.redact)
	}
	return nil
}

func (s *sink) event(body json.RawMessage) error {
	var event synthetic.Event
	if err := decodeCore(body, &event); err != nil {
		return err
	}
	if err := validateEvent(event); err != nil {
		return err
	}
	s.extensions.core()
	s.add(event)
	return nil
}

func (s *sink) terminal(body json.RawMessage) error {
	var result reporterResult
	if err := decodeCore(body, &result); err != nil {
		return err
	}
	if err := validateResult(s.kind, s.capture, s.trace, &result); err != nil {
		return err
	}
	s.extensions.core()
	result.Error = s.redact.text(result.Error)
	s.result = &result
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
	if len(s.events) < eventLimit {
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
