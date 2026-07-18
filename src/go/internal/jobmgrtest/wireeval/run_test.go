package wireeval

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/observation"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"
)

func TestSetupTrackerAcceptsBothExactShippedForms(t *testing.T) {
	tests := map[string]struct {
		configRecords func(*strings.Builder, string)
	}{
		"baseline create accepted then status running": {
			configRecords: func(output *strings.Builder, key string) {
				fmt.Fprintf(output, "CONFIG poc:collector:perf:job-%s create accepted job /collectors/poc/Jobs stock '' '' 0x0000 0x0000\n\n", key)
				fmt.Fprintf(output, "CONFIG poc:collector:perf:job-%s status running\n\n", key)
			},
		},
		"production create running": {
			configRecords: func(output *strings.Builder, key string) {
				fmt.Fprintf(output, "CONFIG poc:collector:perf:job-%s create running job /collectors/poc/Jobs stock '' '' 0x0000 0x0000\n\n", key)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var output strings.Builder
			output.WriteString("FUNCTION GLOBAL \"poc:payload-digest\" 1 \"\" \"\" 0x0000 0 0\n\n")
			output.WriteString("CONFIG poc:collector:perf create accepted template /collectors/poc/Jobs stock '' '' 0x0000 0x0000\n\n")
			for ordinal := range 256 {
				key := fmt.Sprintf("%03d", ordinal)
				fmt.Fprintf(&output, "FUNCTION GLOBAL \"perf:work-%s\" 1 \"\" \"\" 0x0000 0 0\n\n", key)
				test.configRecords(&output, key)
			}
			var tracker setupTracker
			stream := []byte(output.String())
			ready := false
			for offset, width := 0, 1; offset < len(stream); width++ {
				end := min(len(stream), offset+width)
				becameReady, err := tracker.Feed(stream[offset:end])
				if err != nil {
					t.Fatal(err)
				}
				ready = ready || becameReady
				offset = end
			}
			if !ready || !tracker.complete || tracker.functionCount != 256 ||
				tracker.createdCount != 256 || tracker.runningCount != 256 ||
				len(tracker.pending) != 0 {
				t.Fatalf(
					"exact fragmented setup did not complete: functions=%d created=%d running=%d pending=%d",
					tracker.functionCount,
					tracker.createdCount,
					tracker.runningCount,
					len(tracker.pending),
				)
			}
		})
	}
}

func TestSetupTrackerRejectsDuplicateUnknownSurplusAndInvalidTransitions(t *testing.T) {
	tests := map[string]string{
		"duplicate Function": "FUNCTION GLOBAL \"perf:work-000\" 1 \"\" \"\" 0x0000 0 0\n\nFUNCTION GLOBAL \"perf:work-000\" 1 \"\" \"\" 0x0000 0 0\n\n",
		"unknown Function":   "FUNCTION GLOBAL \"perf:work-256\" 1 \"\" \"\" 0x0000 0 0\n\n",
		"unknown config":     "CONFIG poc:collector:perf:job-256 create accepted job x\n\n",
		"running first":      "CONFIG poc:collector:perf:job-000 status running\n\n",
		"unexpected status":  "CONFIG poc:collector:perf:job-000 status failed\n\n",
		"unexpected create":  "CONFIG poc:collector:perf:job-000 create queued job x\n\n",
		"target then status": "CONFIG poc:collector:perf:job-000 create running job x\n\nCONFIG poc:collector:perf:job-000 status running\n\n",
	}
	for name, stream := range tests {
		t.Run(name, func(t *testing.T) {
			var tracker setupTracker
			_, err := tracker.Feed([]byte(stream))
			if name == "running first" {
				if err == nil {
					t.Fatal("running-before-create setup transition was accepted")
				}
				return
			}
			if err == nil {
				t.Fatalf("invalid setup stream was accepted: %q", stream)
			}
		})
	}
}

func TestSetupTrackerRejectsFixtureRecordAfterCompletion(t *testing.T) {
	var tracker setupTracker
	for ordinal := range 256 {
		key := fmt.Sprintf("%03d", ordinal)
		stream := fmt.Sprintf(
			"FUNCTION GLOBAL \"perf:work-%s\" 1 \"\" \"\" 0x0000 0 0\n\nCONFIG poc:collector:perf:job-%s create accepted job x\n\nCONFIG poc:collector:perf:job-%s status running\n\n",
			key, key, key,
		)
		if _, err := tracker.Feed([]byte(stream)); err != nil {
			t.Fatal(err)
		}
	}
	if !tracker.complete {
		t.Fatal("canonical setup did not complete")
	}
	if _, err := tracker.Feed([]byte("CONFIG poc:collector:perf:job-000 status running\n\n")); err == nil {
		t.Fatal("post-ready setup record was accepted")
	}
}

func TestRejectSurplusEvidenceRequiresExactStableStreams(t *testing.T) {
	results := make(chan observation.FunctionResult, 1)
	events := make(chan observation.PassiveEvent, 1)
	observerErrors := make(chan error, 1)
	if err := rejectSurplusEvidence(results, events, observerErrors); err != nil {
		t.Fatalf("empty stable streams were rejected: %v", err)
	}
	results <- observation.FunctionResult{}
	if err := rejectSurplusEvidence(results, events, observerErrors); err == nil || !strings.Contains(err.Error(), "trailing Function results") {
		t.Fatalf("surplus result was accepted: %v", err)
	}
	<-results
	events <- observation.PassiveEvent{}
	if err := rejectSurplusEvidence(results, events, observerErrors); err == nil || !strings.Contains(err.Error(), "trailing passive events") {
		t.Fatalf("surplus event was accepted: %v", err)
	}
	<-events
	want := errors.New("observer failed")
	observerErrors <- want
	if err := rejectSurplusEvidence(results, events, observerErrors); !errors.Is(err, want) {
		t.Fatalf("stable observer error was lost: %v", err)
	}
}

func TestPerformanceEventKindRejectsEveryOtherValidProtocolEvent(t *testing.T) {
	for _, kind := range []protocol.EventKind{protocol.EventHandlerEntered, protocol.EventDeadlineObserved} {
		if err := validatePerformanceEventKind(kind); err != nil {
			t.Fatalf("required performance event %q was rejected: %v", kind, err)
		}
	}
	for _, kind := range []protocol.EventKind{
		protocol.EventCutReached,
		protocol.EventHandlerReturned,
		protocol.EventWriteAttempt,
		protocol.EventProcessSentinel,
	} {
		if err := validatePerformanceEventKind(kind); err == nil {
			t.Fatalf("surplus valid protocol event %q was accepted", kind)
		}
	}
}
