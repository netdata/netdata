// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
