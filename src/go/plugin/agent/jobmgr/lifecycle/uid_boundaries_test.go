package lifecycle

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUIDLedgerTombstonePopulationGrowsBeyondFormerBoundary(t *testing.T) {
	tests := map[string]struct {
		population int
	}{
		"one below former limit": {population: formerFixedUIDPopulation - 1},
		"at former limit":        {population: formerFixedUIDPopulation},
		"one above former limit": {population: formerFixedUIDPopulation + 1},
	}
	now := time.Unix(100, 0)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ledger := NewUIDLedger()
			seedUIDTombstones(t, ledger, test.population, now)

			require.NoError(t, ledger.Admit("fresh", now))
		})
	}

	expiryTests := map[string]struct {
		offset    time.Duration
		wantAdmit bool
	}{
		"before expiry": {offset: UIDTombstoneLifetime - time.Nanosecond, wantAdmit: false},
		"at expiry":     {offset: UIDTombstoneLifetime, wantAdmit: true},
		"after expiry":  {offset: UIDTombstoneLifetime + time.Nanosecond, wantAdmit: true},
	}
	for name, test := range expiryTests {
		t.Run(name, func(t *testing.T) {
			ledger := NewUIDLedger()

			require.NoError(t, ledger.Admit("same", now))

			require.NoError(t, ledger.Complete("same", true, now))

			err := ledger.Admit("same", now.Add(test.offset))
			require.EqualValues(t, test.wantAdmit, err == nil)
		})
	}
}

func TestUIDLedgerAdmissionReapsOneBoundedExpiredPrefix(t *testing.T) {
	tests := map[string]struct {
		incoming string
	}{
		"fresh tail":                {incoming: "fresh"},
		"expired duplicate at tail": {incoming: fmt.Sprintf("uid-%05d", UIDReturnBatch)},
	}
	now := time.Unix(100, 0)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ledger := NewUIDLedger()
			seedUIDTombstones(t, ledger, UIDReturnBatch+1, now)

			require.NoError(t, ledger.Admit(test.incoming, now.Add(UIDTombstoneLifetime)))

			active, tombstones, closed := ledger.Census()
			require.False(t, active != 1 || tombstones != 1 || closed)
		})
	}
}

func TestUIDLedgerCloseUsesBoundedAcknowledgedBatches(t *testing.T) {
	populations := map[string]int{
		"zero":                  0,
		"one":                   1,
		"one full batch":        UIDReturnBatch,
		"one batch plus one":    UIDReturnBatch + 1,
		"former limit plus one": formerFixedUIDPopulation + 1,
	}
	now := time.Unix(100, 0)
	for name, population := range populations {
		t.Run(name, func(t *testing.T) {
			ledger := NewUIDLedger()
			seedUIDTombstones(t, ledger, population, now)
			turns := 0
			for {
				turns++
				more, err := ledger.CloseBatch(UIDReturnBatch)
				require.NoError(t, err)
				if !more {
					break
				}
			}
			wantTurns := max(1, (population+UIDReturnBatch-1)/UIDReturnBatch)
			require.EqualValues(t, wantTurns, turns)
			active, tombstones, closed := ledger.Census()
			require.False(t, active != 0 || tombstones != 0 || !closed)
		})
	}
}

func seedUIDTombstones(t *testing.T, ledger *UIDLedger, count int, now time.Time) {
	t.Helper()
	for index := range count {
		uid := fmt.Sprintf("uid-%05d", index)

		require.NoError(t, ledger.Admit(uid, now))

		require.NoError(t, ledger.Complete(uid, true, now))
	}
}
