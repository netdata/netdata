package contract

import "testing"

func TestPerformanceSchedulePrebuildsExactImmutableInputs(t *testing.T) {
	tests := map[string]struct {
		workload string
	}{
		"balanced":       {workload: "B-WL-001-balanced"},
		"job manager":    {workload: "B-WL-002-jm-heavy"},
		"function heavy": {workload: "B-WL-003-function-heavy"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var nonce [16]byte
			nonce[15] = 1
			schedule, err := BuildPerformanceSchedule(test.workload, nonce)
			if err != nil {
				t.Fatal(err)
			}
			if err := schedule.Validate(test.workload, nonce); err != nil {
				t.Fatal(err)
			}
			if len(schedule.Operations) != PerformanceOperations ||
				len(schedule.ByUID) != PerformanceOperations ||
				len(schedule.SHA256) != 64 {
				t.Fatalf("schedule shape differs: %#v", schedule)
			}
			for sequence, operation := range schedule.Operations {
				if operation.Sequence != sequence ||
					schedule.ByUID[operation.UID] != sequence ||
					operation.RequestSHA256 == "" ||
					operation.FollowupSHA256 == "" ||
					operation.UsefulWorkSHA256 == "" {
					t.Fatalf("operation %d differs: %#v", sequence, operation)
				}
			}
		})
	}
}

func TestPerformanceScheduleRejectsCoordinateMismatch(t *testing.T) {
	var nonce [16]byte
	schedule, err := BuildPerformanceSchedule("B-WL-001-balanced", nonce)
	if err != nil {
		t.Fatal(err)
	}
	other := nonce
	other[0] = 1
	tests := map[string]struct {
		workload string
		nonce    [16]byte
	}{
		"workload": {workload: "B-WL-002-jm-heavy", nonce: nonce},
		"nonce":    {workload: "B-WL-001-balanced", nonce: other},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := schedule.Validate(test.workload, test.nonce); err == nil {
				t.Fatal("mismatched schedule coordinate was accepted")
			}
		})
	}
}
