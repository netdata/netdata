package reducer

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/observation"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"
)

func TestPreparedReducerAcceptsOnlyPrebuiltScheduleOffers(t *testing.T) {
	var nonce [16]byte
	schedule, err := contract.BuildPerformanceSchedule("B-WL-001-balanced", nonce)
	if err != nil {
		t.Fatal(err)
	}
	reduced, err := NewPrepared(schedule)
	if err != nil {
		t.Fatal(err)
	}
	operation := schedule.Operations[0]
	offer := observation.OfferedRequest{
		Sequence: operation.Sequence, Class: string([]byte{byte(operation.Class)}),
		Key: operation.Key, UID: operation.UID,
		RequestSHA256: operation.RequestSHA256, FollowupSHA256: operation.FollowupSHA256,
		UsefulWorkSHA256: operation.UsefulWorkSHA256, OfferedMonoNS: 1,
	}
	if err := reduced.Offer(offer); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*observation.OfferedRequest){
		"sequence": func(value *observation.OfferedRequest) { value.Sequence++ },
		"class":    func(value *observation.OfferedRequest) { value.Class = "X" },
		"key":      func(value *observation.OfferedRequest) { value.Key = "999" },
		"uid":      func(value *observation.OfferedRequest) { value.UID = "other" },
		"request":  func(value *observation.OfferedRequest) { value.RequestSHA256 = "other" },
		"followup": func(value *observation.OfferedRequest) { value.FollowupSHA256 = "other" },
		"work":     func(value *observation.OfferedRequest) { value.UsefulWorkSHA256 = "other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			next := schedule.Operations[1]
			value := observation.OfferedRequest{
				Sequence: next.Sequence, Class: string([]byte{byte(next.Class)}),
				Key: next.Key, UID: next.UID,
				RequestSHA256: next.RequestSHA256, FollowupSHA256: next.FollowupSHA256,
				UsefulWorkSHA256: next.UsefulWorkSHA256, OfferedMonoNS: 2,
			}
			mutate(&value)
			if err := reduced.Offer(value); err == nil {
				t.Fatal("offer differing from prebuilt schedule was accepted")
			}
		})
	}
}

func TestReducerRejectsShortcutResultsAndOrdersByParentSequence(t *testing.T) {
	reduced, err := New(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, offer := range []observation.OfferedRequest{
		{Sequence: 1, Class: "F", Key: "001", UID: "u1", OfferedMonoNS: 10},
		{Sequence: 0, Class: "F", Key: "000", UID: "u0", OfferedMonoNS: 5},
	} {
		if err := reduced.Offer(offer); err != nil {
			t.Fatal(err)
		}
	}
	if err := reduced.ObserveResult(observation.FunctionResult{UID: "unknown", ReadReturnMonoNS: 20}); err == nil {
		t.Fatal("unknown result accepted")
	}
	if err := reduced.ObserveResult(observation.FunctionResult{UID: "u1", ReadReturnMonoNS: 20}); err != nil {
		t.Fatal(err)
	}
	if err := reduced.ObserveResult(observation.FunctionResult{UID: "u0", ReadReturnMonoNS: 21}); err != nil {
		t.Fatal(err)
	}
	completed := reduced.Completed()
	if len(completed) != 2 || completed[0].Offer.Sequence != 0 || completed[1].Offer.Sequence != 1 {
		t.Fatalf("completion projection differs: %#v", completed)
	}
}

func TestReducerRequiresHandlerEnteredBeforeCancelDecision(t *testing.T) {
	reduced, err := New(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := reduced.Offer(observation.OfferedRequest{Sequence: 0, Class: "C", Key: "000", UID: "u0", OfferedMonoNS: 1}); err != nil {
		t.Fatal(err)
	}
	event, err := protocol.NewEvent(1, 1, observation.EventHandlerEntered, "u0", "perf:work-000", nil)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := observation.NewPassiveEvent(event, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := reduced.ObserveEvent(observed); err != nil {
		t.Fatal(err)
	}
	if err := reduced.SetFollowup("u0", 3); err != nil {
		t.Fatal(err)
	}
	if err := reduced.SetFollowup("u0", 4); err == nil {
		t.Fatal("duplicate follow-up accepted")
	}
	if !reduced.HasEvent("u0", observation.EventHandlerEntered) {
		t.Fatal("handler-entered event was not retained")
	}
}

func TestReducerPreservesPhysicalObservationsBeforeWriteReturn(t *testing.T) {
	reduced, err := New(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := reduced.Offer(observation.OfferedRequest{
		Sequence: 0, Class: "C", Key: "000", UID: "u0", OfferedMonoNS: 10,
	}); err != nil {
		t.Fatal(err)
	}
	event, err := protocol.NewEvent(1, 1, observation.EventHandlerEntered, "u0", "perf:work-000", nil)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := observation.NewPassiveEvent(event, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := reduced.ObserveEvent(observed); err != nil {
		t.Fatalf("physical event before write return was rejected: %v", err)
	}
	if err := reduced.ObserveResult(observation.FunctionResult{UID: "u0", ReadReturnMonoNS: 9}); err != nil {
		t.Fatalf("physical result before write return was rejected: %v", err)
	}
	if err := reduced.SetFollowup("u0", 11); err != nil {
		t.Fatalf("semantic follow-up after an earlier physical read was rejected: %v", err)
	}
	completed := reduced.Completed()
	if len(completed) != 1 || completed[0].Result.ReadReturnMonoNS != 9 ||
		completed[0].Events[0].ObservedMonotonicNS != 8 || completed[0].Offer.FollowupMonoNS != 11 {
		t.Fatalf("raw physical timestamps were not preserved: %#v", completed)
	}
}

func TestReducerRejectsChangedGenerationAndGlobalEventReordering(t *testing.T) {
	reduced, err := New(2)
	if err != nil {
		t.Fatal(err)
	}
	for sequence, uid := range []string{"u0", "u1"} {
		if err := reduced.Offer(observation.OfferedRequest{Sequence: sequence, Class: "F", Key: "000", UID: uid, OfferedMonoNS: 1}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := protocol.NewEvent(1, 2, observation.EventHandlerEntered, "u0", "perf:work-000", nil)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := observation.NewPassiveEvent(first, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := reduced.ObserveEvent(observed); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name                 string
		generation, sequence uint64
	}{
		{name: "changed generation", generation: 2, sequence: 3},
		{name: "reordered sequence", generation: 1, sequence: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			event, err := protocol.NewEvent(test.generation, test.sequence, observation.EventHandlerEntered, "u1", "perf:work-001", nil)
			if err != nil {
				t.Fatal(err)
			}
			observed, err := observation.NewPassiveEvent(event, 3)
			if err != nil {
				t.Fatal(err)
			}
			if err := reduced.ObserveEvent(observed); err == nil {
				t.Fatal("invalid event ordering accepted")
			}
		})
	}
}
