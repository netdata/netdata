// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// wireRequest is the one JSON request the runner reads on stdin.
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

// reporterResult is the reporter's terminal result. It says what the
// workflow reported, not that its process tree completed.
type reporterResult struct {
	Status               synthetic.Outcome           `json:"status"`
	TerminationReason    synthetic.TerminationReason `json:"termination_reason,omitempty"`
	InterruptedArtifacts []synthetic.Capture         `json:"interrupted_artifacts,omitempty"`
	Error                string                      `json:"error,omitempty"`
	Tests                *synthetic.TestCounts       `json:"tests,omitempty"`
	Metrics              *synthetic.LabMetrics       `json:"metrics,omitempty"`
	Artifacts            []synthetic.Capture         `json:"artifacts,omitempty"`
	CaptureState         synthetic.CaptureState      `json:"capture_state"`
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

// decodeFrame strictly decodes one frame: a single JSON object with known members.
func decodeFrame(raw []byte) (wireEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var frame wireEnvelope
	if err := decoder.Decode(&frame); err != nil {
		return frame, fmt.Errorf("invalid runner protocol frame: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return frame, errors.New("multiple values in runner protocol frame")
	}
	return frame, nil
}

// body returns the member named by the frame type. Any other body member,
// even null, invalidates the envelope.
func (f *wireEnvelope) body() (json.RawMessage, error) {
	bodies := [...]struct {
		frameType string
		raw       json.RawMessage
		mixed     string
	}{
		{"event", f.Event, "invalid runner event envelope"},
		{"result", f.Result, "invalid runner result envelope"},
		{"extension_init", f.ExtensionInit, "mixed runner extension envelope"},
		{"extension", f.Extension, "mixed runner extension envelope"},
	}
	selected, others := -1, false
	for i, b := range bodies {
		if b.frameType == f.Type {
			selected = i
		} else if b.raw != nil {
			others = true
		}
	}
	switch {
	case selected < 0:
		return nil, errors.New("unsupported runner frame type")
	case others:
		return nil, errors.New(bodies[selected].mixed)
	}
	return bodies[selected].raw, nil
}

func decodeCore(raw json.RawMessage, value any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return errors.New("missing runner core body")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid runner core body: %w", err)
	}
	return nil
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

// validateResult checks a terminal result against the request: the outcome,
// capture state, termination reason, kind-specific facts and capture kinds,
// in that order.
func validateResult(kind synthetic.Kind, capture, trace bool, result *reporterResult) error {
	switch result.Status {
	case synthetic.Success, synthetic.Failed, synthetic.Timeout, synthetic.Inconclusive, synthetic.Error, synthetic.Cancelled:
	default:
		return errors.New("invalid terminal outcome")
	}
	if err := validateCaptureState(capture, result); err != nil {
		return err
	}
	if err := validateTermination(kind, result); err != nil {
		return err
	}
	if kind == synthetic.Journey {
		if err := validateJourneyResult(result); err != nil {
			return err
		}
	} else if err := validateLighthouseResult(result); err != nil {
		return err
	}
	return validateCaptureKinds(kind, trace, result)
}

func validateCaptureState(capture bool, result *reporterResult) error {
	candidates := len(result.Artifacts) + len(result.InterruptedArtifacts)
	if !capture {
		if result.CaptureState != synthetic.CaptureDisabled || candidates != 0 {
			return errors.New("runner captured content while capture was disabled")
		}
		return nil
	}
	switch result.CaptureState {
	case synthetic.CaptureNotNeeded, synthetic.CaptureUnavailable, synthetic.CaptureAvailable:
	default:
		return errors.New("invalid capture state")
	}
	if (result.CaptureState == synthetic.CaptureAvailable) != (candidates > 0) {
		return errors.New("capture state contradicts artifact candidates")
	}
	return nil
}

func validateTermination(kind synthetic.Kind, result *reporterResult) error {
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
	return nil
}

func validateJourneyResult(result *reporterResult) error {
	if result.Metrics != nil {
		return errors.New("journey result contains lab measurements")
	}
	tests := result.Tests
	if tests != nil {
		if tests.Declared < 0 {
			return errors.New("negative declared test count")
		}
		remaining := tests.Declared
		for _, n := range []int{tests.Passed, tests.Failed, tests.TimedOut, tests.Skipped, tests.ExpectedFailure, tests.NotRun} {
			if n < 0 || n > remaining {
				return errors.New("invalid disjoint test counts")
			}
			remaining -= n
		}
		if remaining != 0 {
			return errors.New("test counts do not cover declared tests")
		}
	}
	switch result.Status {
	case synthetic.Success:
		if tests == nil || tests.Declared == 0 || tests.Passed != tests.Declared {
			return errors.New("journey success requires every declared ordinary test to pass")
		}
	case synthetic.Inconclusive:
		if tests == nil || tests.Failed > 0 || tests.TimedOut > 0 || tests.Declared > 0 && tests.Passed == tests.Declared {
			return errors.New("inconclusive journey contradicts test outcomes")
		}
	case synthetic.Failed:
		if tests != nil && tests.TimedOut > 0 {
			return errors.New("test timeout must dominate unexpected failure")
		}
		if tests == nil || tests.Failed == 0 {
			return errors.New("journey failure lacks an unexpected failed test")
		}
	}
	return nil
}

func validateLighthouseResult(result *reporterResult) error {
	if result.Tests != nil {
		return errors.New("Lighthouse result contains journey counts")
	}
	m := result.Metrics
	if m == nil {
		if result.Status == synthetic.Success {
			return errors.New("Lighthouse success lacks audit measurements")
		}
		return nil
	}
	for _, value := range []*float64{m.Performance, m.FCPMS, m.LCPMS, m.TBTMS, m.SIMS, m.CLS} {
		if value != nil && (!finite(*value) || *value < 0) {
			return errors.New("invalid lab measurement")
		}
	}
	if m.Performance != nil && *m.Performance > 100 {
		return errors.New("performance points exceed 100")
	}
	return nil
}

func validateCaptureKinds(kind synthetic.Kind, trace bool, result *reporterResult) error {
	for _, a := range result.InterruptedArtifacts {
		if !validJourneyCapture(a, trace) {
			return errors.New("unsupported interrupted journey capture")
		}
	}
	for _, a := range result.Artifacts {
		if kind == synthetic.Journey && !validJourneyCapture(a, trace) {
			return errors.New("unsupported journey capture")
		}
		if kind == synthetic.Lighthouse && !validCapture(a, synthetic.ReportCapture) {
			return errors.New("unsupported audit capture")
		}
	}
	return nil
}

func validJourneyCapture(a synthetic.Capture, trace bool) bool {
	return validCapture(a, synthetic.ScreenshotCapture) || trace && validCapture(a, synthetic.TraceCapture)
}

func validCapture(a synthetic.Capture, kind synthetic.CaptureKind) bool {
	return a.Kind == kind && a.MIME == kind.MIME()
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
