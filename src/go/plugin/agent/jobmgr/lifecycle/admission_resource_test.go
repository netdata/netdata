package lifecycle

import "testing"

func TestAdmissionResourceAlgebraBoundaries(t *testing.T) {
	tests := map[string]struct {
		reserveProcess int64
		request        int64
		wantAccepted   bool
	}{
		"exact remaining capacity": {
			reserveProcess: 1,
			request:        OrdinaryBudgetBytes - 1,
			wantAccepted:   true,
		},
		"one over remaining capacity": {
			reserveProcess: 1,
			request:        OrdinaryBudgetBytes,
			wantAccepted:   false,
		},
		"exact total capacity": {
			request:      OrdinaryBudgetBytes,
			wantAccepted: true,
		},
		"one over total capacity": {
			request:      OrdinaryBudgetBytes + 1,
			wantAccepted: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ledger := NewAdmissionLedger()
			if err := ledger.ReserveProcessBytes(
				test.reserveProcess,
			); err != nil {
				t.Fatal(err)
			}
			result := ledger.RequestOrdinary(
				1,
				AdmissionLaneRef{Slot: 1, Generation: 1},
				test.request,
			)
			if (result.Rejected == nil) != test.wantAccepted {
				t.Fatalf(
					"ordinary admission=%v wantAccepted=%v",
					result.Rejected,
					test.wantAccepted,
				)
			}
		})
	}

	cleanupTests := map[string]struct {
		bytes        int64
		wantAccepted bool
	}{
		"one below": {
			bytes: CleanupBudgetBytes - 1, wantAccepted: true,
		},
		"exact": {
			bytes: CleanupBudgetBytes, wantAccepted: true,
		},
		"one above": {
			bytes: CleanupBudgetBytes + 1,
		},
	}
	for name, test := range cleanupTests {
		t.Run("cleanup "+name, func(t *testing.T) {
			result := NewAdmissionLedger().RequestCleanup(
				1,
				AdmissionLaneRef{Slot: 1, Generation: 1},
				test.bytes,
			)
			if (result.Rejected == nil) != test.wantAccepted {
				t.Fatalf(
					"cleanup admission=%v wantAccepted=%v",
					result.Rejected,
					test.wantAccepted,
				)
			}
		})
	}
}
