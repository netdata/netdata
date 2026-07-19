package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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

			require.NoError(t, ledger.ReserveProcessBytes(test.reserveProcess))

			result := ledger.RequestOrdinary(
				1,
				AdmissionLaneRef{Slot: 1, Generation: 1},
				test.request,
			)
			require.EqualValues(t, test.wantAccepted, result.Rejected == nil)
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
			require.EqualValues(t, test.wantAccepted, result.Rejected == nil)
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
			require.Nil(t, result.Rejected)
			var grants [4]AdmissionGrant
			count, _, err := ledger.TakeGrants(1, &grants)
			require.False(t, err != nil || count != 1 || grants[0].Ref != result.Ref)

			_, suspendOrdinaryErr := ledger.SuspendOrdinary(result.Ref, test.retained)
			require.NoError(t, suspendOrdinaryErr)

			census := ledger.Census()
			require.False(t, census.ActiveRecords != 1 ||
				census.OrdinaryGranted != 0 ||
				census.OrdinaryWaiting != 0 ||
				census.OrdinarySuspended != 1 ||
				census.OrdinaryBytes != test.retained)

			count, _, err = ledger.TakeGrants(1, &grants)
			require.False(t, err != nil || count != 0)
			if !test.resume {
				require.NoError(t, ledger.CancelWaiting(result.Ref))

				census := ledger.Census()
				require.False(t, census.ActiveRecords != 0 || census.OrdinarySuspended != 0 || census.OrdinaryBytes != 0)

				return
			}
			later := ledger.RequestOrdinary(
				1,
				AdmissionLaneRef{Slot: 2, Generation: 1},
				100,
			)
			require.Nil(t, later.Rejected)

			require.NoError(t, ledger.ResumeOrdinary(result.Ref))

			ledgerCensus := ledger.Census()
			require.False(t, ledgerCensus.OrdinaryWaiting != 2 ||
				ledgerCensus.OrdinarySuspended != 0 ||
				ledgerCensus.OrdinaryBytes != test.retained)

			count, _, err = ledger.TakeGrants(2, &grants)
			require.False(t, err != nil || count != 2 || grants[0].Ref != later.Ref || grants[1].Ref != result.Ref)

			_, releaseOrdinaryErr := ledger.ReleaseOrdinary(later.Ref)
			require.NoError(t, releaseOrdinaryErr)

			_, releaseOrdinaryErr2 := ledger.ReleaseOrdinary(result.Ref)
			require.NoError(t, releaseOrdinaryErr2)

			ledgerCensus2 := ledger.Census()
			require.False(t, ledgerCensus2.ActiveRecords != 0 || ledgerCensus2.OrdinaryBytes != 0)

		})
	}
}
