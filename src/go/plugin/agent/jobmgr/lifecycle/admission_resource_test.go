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

func TestAdmissionSuspensionPreservesRetainedBytes(t *testing.T) {
	tests := map[string]struct {
		retained int64
		resume   bool
	}{
		"ordinary metadata only": {
			resume: true,
		},
		"transferred payload": {
			retained: 40,
			resume:   true,
		},
		"cancel suspended payload": {
			retained: 40,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ledger := NewAdmissionLedger()
			result := ledger.RequestOrdinary(
				1,
				AdmissionLaneRef{Slot: 1, Generation: 1},
				100,
			)
			if result.Rejected != nil {
				t.Fatal(result.Rejected)
			}
			var grants [4]AdmissionGrant
			count, _, err := ledger.TakeGrants(1, &grants)
			if err != nil || count != 1 ||
				grants[0].Ref != result.Ref {
				t.Fatalf(
					"initial grant count=%d grant=%+v error=%v",
					count,
					grants[0],
					err,
				)
			}
			if _, err := ledger.SuspendOrdinary(
				result.Ref,
				test.retained,
			); err != nil {
				t.Fatal(err)
			}
			if census := ledger.Census(); census.ActiveRecords != 1 ||
				census.OrdinaryGranted != 0 ||
				census.OrdinaryWaiting != 0 ||
				census.OrdinarySuspended != 1 ||
				census.OrdinaryBytes != test.retained {
				t.Fatalf("suspended census=%+v", census)
			}
			count, _, err = ledger.TakeGrants(1, &grants)
			if err != nil || count != 0 {
				t.Fatalf(
					"suspended grant count=%d error=%v",
					count,
					err,
				)
			}
			if !test.resume {
				if err := ledger.CancelWaiting(result.Ref); err != nil {
					t.Fatal(err)
				}
				if census := ledger.Census(); census.ActiveRecords != 0 ||
					census.OrdinarySuspended != 0 ||
					census.OrdinaryBytes != 0 {
					t.Fatalf("cancelled census=%+v", census)
				}
				return
			}
			later := ledger.RequestOrdinary(
				1,
				AdmissionLaneRef{Slot: 2, Generation: 1},
				100,
			)
			if later.Rejected != nil {
				t.Fatal(later.Rejected)
			}
			if err := ledger.ResumeOrdinary(result.Ref); err != nil {
				t.Fatal(err)
			}
			if census := ledger.Census(); census.OrdinaryWaiting != 2 ||
				census.OrdinarySuspended != 0 ||
				census.OrdinaryBytes != test.retained {
				t.Fatalf("resumed census=%+v", census)
			}
			count, _, err = ledger.TakeGrants(2, &grants)
			if err != nil || count != 2 ||
				grants[0].Ref != later.Ref ||
				grants[1].Ref != result.Ref {
				t.Fatalf(
					"resumed grants count=%d grants=%+v error=%v",
					count,
					grants[:count],
					err,
				)
			}
			if _, err := ledger.ReleaseOrdinary(
				later.Ref,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.ReleaseOrdinary(
				result.Ref,
			); err != nil {
				t.Fatal(err)
			}
			if census := ledger.Census(); census.ActiveRecords != 0 ||
				census.OrdinaryBytes != 0 {
				t.Fatalf("released census=%+v", census)
			}
		})
	}
}
