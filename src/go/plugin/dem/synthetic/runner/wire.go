// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// result is terminal reporter evidence, not process-tree completion evidence.
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
	if !capture && (result.CaptureState != synthetic.CaptureDisabled || candidateCount != 0) {
		return errors.New("runner captured content while capture was disabled")
	}
	if capture {
		switch result.CaptureState {
		case synthetic.CaptureNotNeeded, synthetic.CaptureUnavailable, synthetic.CaptureAvailable:
		default:
			return errors.New("invalid capture state")
		}
		if (result.CaptureState == synthetic.CaptureAvailable) != (candidateCount > 0) {
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
