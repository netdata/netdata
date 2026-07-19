package oracle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/observation"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/reducer"
)

func TestAnalyzeRunUsesRealFramesAndExactSchedule(t *testing.T) {
	run := buildValidRun(t)
	summary, err := AnalyzeRun(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Metrics) != 19 ||
		summary.Metrics[MetricMixedP50] != 10 ||
		summary.Metrics[MetricMixedP95] != 10 ||
		summary.Metrics[MetricMixedP99] != 10 ||
		summary.Metrics[MetricCancelP50] != 10 ||
		summary.Metrics[MetricCancelP95] != 10 ||
		summary.Metrics[MetricCancelP99] != 10 {
		t.Fatalf("metrics differ: %#v", summary.Metrics)
	}
	if !validDigest(summary.TranscriptSHA256) || !validDigest(summary.EqualitySHA256) || summary.TranscriptSHA256 == summary.EqualitySHA256 {
		t.Fatalf("run digests differ: %#v", summary)
	}
}

func TestAnalyzeRunRejectsCancelBeforeHandlerEntered(t *testing.T) {
	run := buildValidRun(t)
	run.Operations = append([]reducer.CompletedOperation(nil), run.Operations...)
	for index := range run.Operations {
		if run.Operations[index].Offer.Class == string(contract.ClassCancel) {
			run.Operations[index].Offer.FollowupMonoNS = run.Operations[index].Offer.OfferedMonoNS
			break
		}
	}
	if _, err := AnalyzeRun(run); err == nil || !strings.Contains(err.Error(), "after handler-entered") {
		t.Fatalf("early cancel was not rejected: %v", err)
	}
}

func TestAnalyzeRunAllowsPhysicalResultReadBeforeCancelWriteReturns(t *testing.T) {
	run := buildValidRun(t)
	for index := range run.Operations {
		operation := &run.Operations[index]
		if operation.Offer.Class != string(contract.ClassCancel) {
			continue
		}
		operation.Result.ReadReturnMonoNS = operation.Offer.FollowupMonoNS - 1
		break
	}
	if _, err := AnalyzeRun(run); err != nil {
		t.Fatalf("physical read-before-write-return ordering was rejected: %v", err)
	}
}

func TestAnalyzeRunTreatsSubSyscallObservationOverlapAsZeroElapsed(t *testing.T) {
	run := buildValidRun(t)
	for index := range run.Operations {
		operation := &run.Operations[index]
		if operation.Offer.Class != string(contract.ClassCancel) {
			continue
		}
		operation.Result.ReadReturnMonoNS = operation.Offer.OfferedMonoNS - 1
		operation.Events[0].ObservedMonotonicNS = operation.Offer.OfferedMonoNS - 2
		break
	}
	if _, err := AnalyzeRun(run); err != nil {
		t.Fatalf("sub-syscall physical overlap was rejected: %v", err)
	}
}

func TestAnalyzeRunRejectsDeadlineWithoutObservation(t *testing.T) {
	run := buildValidRun(t)
	run.Operations = append([]reducer.CompletedOperation(nil), run.Operations...)
	for index := range run.Operations {
		if run.Operations[index].Offer.Class == string(contract.ClassDeadline) {
			run.Operations[index].Events = nil
			break
		}
	}
	if _, err := AnalyzeRun(run); err == nil || !strings.Contains(err.Error(), "deadline-observed") {
		t.Fatalf("unobserved deadline was not rejected: %v", err)
	}
}

func TestAnalyzeRunRejectsWrongOrDuplicateClassEvents(t *testing.T) {
	tests := []struct {
		name   string
		class  contract.PerformanceClass
		kind   observation.EventKind
		remove bool
		want   string
	}{
		{name: "job-handler", class: contract.ClassJobManager, kind: observation.EventHandlerEntered, want: "handler-entered"},
		{name: "job-deadline", class: contract.ClassJobManager, kind: observation.EventDeadlineObserved, want: "deadline-observed"},
		{name: "function-handler", class: contract.ClassFunction, kind: observation.EventHandlerEntered, want: "handler-entered"},
		{name: "function-deadline", class: contract.ClassFunction, kind: observation.EventDeadlineObserved, want: "deadline-observed"},
		{name: "cancel-duplicate-handler", class: contract.ClassCancel, kind: observation.EventHandlerEntered, want: "handler-entered"},
		{name: "cancel-deadline", class: contract.ClassCancel, kind: observation.EventDeadlineObserved, want: "deadline-observed"},
		{name: "cancel-missing-handler", class: contract.ClassCancel, remove: true, want: "handler-entered"},
		{name: "deadline-handler", class: contract.ClassDeadline, kind: observation.EventHandlerEntered, want: "handler-entered"},
		{name: "deadline-duplicate", class: contract.ClassDeadline, kind: observation.EventDeadlineObserved, want: "deadline-observed"},
		{name: "capacity-handler", class: contract.ClassCapacity, kind: observation.EventHandlerEntered, want: "handler-entered"},
		{name: "capacity-deadline", class: contract.ClassCapacity, kind: observation.EventDeadlineObserved, want: "deadline-observed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := buildValidRun(t)
			run.Operations = append([]reducer.CompletedOperation(nil), run.Operations...)
			for index := range run.Operations {
				operation := &run.Operations[index]
				if operation.Offer.Class != string(test.class) {
					continue
				}
				operation.Events = append([]observation.PassiveEvent(nil), operation.Events...)
				if test.remove {
					operation.Events = nil
					break
				}
				event, err := protocol.NewEvent(1, 1, test.kind, operation.Offer.UID, "perf:work-"+operation.Offer.Key, nil)
				if err != nil {
					t.Fatal(err)
				}
				observed, err := observation.NewPassiveEvent(event, operation.Offer.OfferedMonoNS+1)
				if err != nil {
					t.Fatal(err)
				}
				operation.Events = append(operation.Events, observed)
				break
			}
			if _, err := AnalyzeRun(run); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("wrong class event was not rejected: %v", err)
			}
		})
	}
}

func TestAnalyzeRunRejectsWrongPassiveEventIdentityAndPayload(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *RunObservation, *observation.PassiveEvent)
		want   string
	}{
		{
			name: "wrong-route",
			mutate: func(_ *testing.T, _ *RunObservation, event *observation.PassiveEvent) {
				event.Message.RouteKey = "perf:work-999"
			},
			want: "passive event route",
		},
		{
			name: "empty-route",
			mutate: func(_ *testing.T, _ *RunObservation, event *observation.PassiveEvent) {
				event.Message.RouteKey = ""
			},
			want: "passive event route",
		},
		{
			name: "nonempty-payload",
			mutate: func(t *testing.T, _ *RunObservation, event *observation.PassiveEvent) {
				message, err := protocol.NewEvent(
					event.Message.ProcessGeneration, event.Message.EventSeq, event.Message.Kind,
					event.Message.Token, event.Message.RouteKey, []byte("unexpected"),
				)
				if err != nil {
					t.Fatal(err)
				}
				event.Message = message
			},
			want: "passive event payload is not empty",
		},
		{
			name: "swapped-token",
			mutate: func(_ *testing.T, run *RunObservation, event *observation.PassiveEvent) {
				event.Message.Token = run.Operations[0].Offer.UID
			},
			want: "passive event token differs",
		},
		{
			name: "corrupt-payload-envelope",
			mutate: func(_ *testing.T, _ *RunObservation, event *observation.PassiveEvent) {
				event.Message.PayloadLen++
			},
			want: "payload length mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := buildValidRun(t)
			_, event := firstRunEvent(t, &run)
			test.mutate(t, &run, event)
			if _, err := AnalyzeRun(run); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("passive event mutation was not rejected: %v", err)
			}
		})
	}
}

func TestOperationDigestsBindPassiveEventSemantics(t *testing.T) {
	run := buildValidRun(t)
	operation, _ := firstRunEvent(t, &run)
	baselineTranscript := completedOperationDigest(t, run, *operation, true)
	baselineEquality := completedOperationDigest(t, run, *operation, false)
	mutations := []struct {
		name   string
		mutate func(*observation.PassiveEvent)
	}{
		{name: "kind", mutate: func(event *observation.PassiveEvent) { event.Message.Kind = observation.EventDeadlineObserved }},
		{name: "token", mutate: func(event *observation.PassiveEvent) { event.Message.Token = "u999" }},
		{name: "route", mutate: func(event *observation.PassiveEvent) { event.Message.RouteKey = "perf:work-999" }},
		{name: "payload", mutate: func(event *observation.PassiveEvent) { event.Message.Payload = []byte("unexpected") }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := *operation
			mutated.Events = append([]observation.PassiveEvent(nil), operation.Events...)
			mutation.mutate(&mutated.Events[0])
			if got := completedOperationDigest(t, run, mutated, true); got == baselineTranscript {
				t.Fatal("semantic event mutation did not change transcript digest")
			}
			if got := completedOperationDigest(t, run, mutated, false); got == baselineEquality {
				t.Fatal("semantic event mutation did not change equality digest")
			}
		})
	}
}

func TestPassiveEventTimestampChangesOnlyTranscriptDigest(t *testing.T) {
	run := buildValidRun(t)
	baseline, err := AnalyzeRun(run)
	if err != nil {
		t.Fatal(err)
	}
	for index := range run.Operations {
		operation := &run.Operations[index]
		if operation.Offer.Class != string(contract.ClassDeadline) {
			continue
		}
		operation.Events = append([]observation.PassiveEvent(nil), operation.Events...)
		operation.Events[0].ObservedMonotonicNS++
		break
	}
	mutated, err := AnalyzeRun(run)
	if err != nil {
		t.Fatal(err)
	}
	if mutated.TranscriptSHA256 == baseline.TranscriptSHA256 {
		t.Fatal("event timestamp mutation did not change transcript digest")
	}
	if mutated.EqualitySHA256 != baseline.EqualitySHA256 {
		t.Fatal("event timestamp mutation changed semantic equality digest")
	}
}

func TestPassiveEventTransportChangesOnlyTranscriptDigest(t *testing.T) {
	run := buildValidRun(t)
	operation, _ := firstRunEvent(t, &run)
	baselineTranscript := completedOperationDigest(t, run, *operation, true)
	baselineEquality := completedOperationDigest(t, run, *operation, false)
	mutated := *operation
	mutated.Events = append([]observation.PassiveEvent(nil), operation.Events...)
	mutated.Events[0].Message.ProcessGeneration++
	mutated.Events[0].Message.EventSeq++
	if got := completedOperationDigest(t, run, mutated, true); got == baselineTranscript {
		t.Fatal("event transport mutation did not change transcript digest")
	}
	if got := completedOperationDigest(t, run, mutated, false); got != baselineEquality {
		t.Fatal("event transport mutation changed semantic equality digest")
	}
}

func TestAnalyzeRunAllowsDeadlineEventReadAfterResultAcrossSeparatePipes(t *testing.T) {
	run := buildValidRun(t)
	run.Operations = append([]reducer.CompletedOperation(nil), run.Operations...)
	for index := range run.Operations {
		if run.Operations[index].Offer.Class != string(contract.ClassDeadline) {
			continue
		}
		run.Operations[index].Events = append([]observation.PassiveEvent(nil), run.Operations[index].Events...)
		run.Operations[index].Events[0].ObservedMonotonicNS = run.Operations[index].Result.ReadReturnMonoNS + 1
		break
	}
	if _, err := AnalyzeRun(run); err != nil {
		t.Fatalf("cross-pipe read inversion rejected: %v", err)
	}
}

func buildValidRun(t *testing.T) RunObservation {
	t.Helper()
	workload := contract.PerformanceWorkloads()[0]
	reduced, err := reducer.New(contract.PerformanceOperations)
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := observation.NewFunctionResultDecoder(8_192)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [16]byte
	copy(nonce[:], "oracle-pair-0001")
	var eventSequence uint64
	for sequence := range contract.PerformanceOperations {
		class, key, err := workload.ClassAndKey(sequence)
		if err != nil {
			t.Fatal(err)
		}
		uid := contract.PerformanceUID(nonce, uint32(sequence))
		request, err := contract.PerformanceRequest(class, uid, key)
		if err != nil {
			t.Fatal(err)
		}
		followup, err := contract.PerformanceFollowup(class, uid)
		if err != nil {
			t.Fatal(err)
		}
		usefulWork, err := contract.UsefulWorkSHA256(class)
		if err != nil {
			t.Fatal(err)
		}
		t0 := int64(sequence*100 + 1)
		offer := observation.OfferedRequest{
			Sequence: sequence, Class: string(class), Key: key, UID: uid,
			RequestSHA256: testDigest(request), FollowupSHA256: testDigest(followup), UsefulWorkSHA256: usefulWork,
			OfferedMonoNS: t0,
		}
		if class == contract.ClassCancel {
			offer.FollowupMonoNS = t0 + 2
		}
		if err := reduced.Offer(offer); err != nil {
			t.Fatal(err)
		}
		var eventKind observation.EventKind
		switch class {
		case contract.ClassCancel:
			eventKind = observation.EventHandlerEntered
		case contract.ClassDeadline:
			eventKind = observation.EventDeadlineObserved
		}
		if eventKind != "" {
			eventSequence++
			event, err := protocol.NewEvent(1, eventSequence, eventKind, uid, "perf:work-"+key, nil)
			if err != nil {
				t.Fatal(err)
			}
			observed, err := observation.NewPassiveEvent(event, t0+1)
			if err != nil {
				t.Fatal(err)
			}
			if err := reduced.ObserveEvent(observed); err != nil {
				t.Fatal(err)
			}
		}
		want, err := contract.ExpectedResult(class)
		if err != nil {
			t.Fatal(err)
		}
		raw := fmt.Appendf(nil, "FUNCTION_RESULT_BEGIN %s %d %s 100\n", uid, want.Status, want.ContentType)
		raw = append(raw, want.Deferred...)
		raw = append(raw, []byte("FUNCTION_RESULT_END\n\n")...)
		results, noise, err := decoder.Feed(raw, t0+10)
		if err != nil {
			t.Fatal(err)
		}
		if len(noise) != 0 || len(results) != 1 {
			t.Fatalf("sequence %d decode: results=%d noise=%q", sequence, len(results), noise)
		}
		if err := reduced.ObserveResult(results[0]); err != nil {
			t.Fatal(err)
		}
	}
	remaining, err := decoder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("decoder retained %q", remaining)
	}
	return RunObservation{
		WorkloadID: workload.ID, Population: 1, PairIndex: 1, Side: SideBaseline, RanFirst: true,
		PairNonce: nonce, EnvironmentSHA256: testDigest([]byte("test-environment")), WallLowerUnix: 100, WallUpperUnix: 100,
		Operations: reduced.Completed(),
	}
}

func firstRunEvent(t *testing.T, run *RunObservation) (*reducer.CompletedOperation, *observation.PassiveEvent) {
	t.Helper()
	for index := range run.Operations {
		operation := &run.Operations[index]
		if len(operation.Events) == 0 {
			continue
		}
		operation.Events = append([]observation.PassiveEvent(nil), operation.Events...)
		return operation, &operation.Events[0]
	}
	t.Fatal("valid run has no passive event")
	return nil, nil
}

func completedOperationDigest(t *testing.T, run RunObservation, operation reducer.CompletedOperation, includeTiming bool) string {
	t.Helper()
	class := contract.PerformanceClass(operation.Offer.Class[0])
	target := sha256.New()
	if err := appendOperationDigest(target, operation, class, operation.Offer.Key, run.WallLowerUnix, run.WallUpperUnix, includeTiming); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(target.Sum(nil))
}

func testDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
