package lifecycle

import (
	"fmt"
	"testing"
	"time"
)

func TestUIDLedgerTombstoneCardinalityBoundaries(t *testing.T) {
	tests := map[string]struct {
		population int
		wantAdmit  bool
	}{
		"one below capacity": {
			population: MaximumUIDRecords - 1,
			wantAdmit:  true,
		},
		"at capacity": {
			population: MaximumUIDRecords,
			wantAdmit:  false,
		},
	}
	now := time.Unix(100, 0)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ledger := NewUIDLedger()
			seedUIDTombstones(
				t,
				ledger,
				test.population,
				now,
			)
			err := ledger.Admit("fresh", now)
			if (err == nil) != test.wantAdmit {
				t.Fatalf(
					"fresh admission err=%v wantAdmit=%v",
					err,
					test.wantAdmit,
				)
			}
		})
	}

	expiryTests := map[string]struct {
		offset    time.Duration
		wantAdmit bool
	}{
		"before expiry": {
			offset:    UIDTombstoneLifetime - time.Nanosecond,
			wantAdmit: false,
		},
		"at expiry": {
			offset:    UIDTombstoneLifetime,
			wantAdmit: true,
		},
		"after expiry": {
			offset:    UIDTombstoneLifetime + time.Nanosecond,
			wantAdmit: true,
		},
	}
	for name, test := range expiryTests {
		t.Run(name, func(t *testing.T) {
			ledger := NewUIDLedger()
			if err := ledger.Admit("same", now); err != nil {
				t.Fatal(err)
			}
			if err := ledger.Complete("same", true, now); err != nil {
				t.Fatal(err)
			}
			err := ledger.Admit("same", now.Add(test.offset))
			if (err == nil) != test.wantAdmit {
				t.Fatalf(
					"same-UID admission err=%v wantAdmit=%v",
					err,
					test.wantAdmit,
				)
			}
		})
	}
}

func TestUIDLedgerAdmissionReapsOneBoundedExpiredPrefix(t *testing.T) {
	tests := map[string]struct {
		incoming string
	}{
		"fresh tail": {
			incoming: "fresh",
		},
		"expired duplicate at tail": {
			incoming: fmt.Sprintf("uid-%05d", UIDReturnBatch),
		},
	}
	now := time.Unix(100, 0)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ledger := NewUIDLedger()
			seedUIDTombstones(
				t,
				ledger,
				UIDReturnBatch+1,
				now,
			)
			if err := ledger.Admit(
				test.incoming,
				now.Add(UIDTombstoneLifetime),
			); err != nil {
				t.Fatal(err)
			}
			active, tombstones, closed := ledger.Census()
			if active != 1 ||
				tombstones != 1 ||
				closed {
				t.Fatalf(
					"bounded reap census=%d/%d/%v, want 1/1/false",
					active,
					tombstones,
					closed,
				)
			}
		})
	}
}

func TestUIDLedgerCloseUsesBoundedAcknowledgedBatches(t *testing.T) {
	populations := map[string]int{
		"zero":                  0,
		"one":                   1,
		"one full batch":        UIDReturnBatch,
		"one batch plus one":    UIDReturnBatch + 1,
		"maximum tombstone set": MaximumUIDRecords,
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
				if err != nil {
					t.Fatal(err)
				}
				if !more {
					break
				}
			}
			wantTurns := max(
				1,
				(population+UIDReturnBatch-1)/UIDReturnBatch,
			)
			if turns != wantTurns ||
				turns > MaximumUIDRecords/UIDReturnBatch {
				t.Fatalf(
					"close turns=%d, want %d within bound",
					turns,
					wantTurns,
				)
			}
			active, tombstones, closed := ledger.Census()
			if active != 0 || tombstones != 0 || !closed {
				t.Fatalf(
					"closed census=%d/%d/%v",
					active,
					tombstones,
					closed,
				)
			}
		})
	}
}

func seedUIDTombstones(
	t *testing.T,
	ledger *UIDLedger,
	count int,
	now time.Time,
) {
	t.Helper()
	for index := 0; index < count; index++ {
		uid := fmt.Sprintf("uid-%05d", index)
		if err := ledger.Admit(uid, now); err != nil {
			t.Fatalf("admit %d: %v", index, err)
		}
		if err := ledger.Complete(uid, true, now); err != nil {
			t.Fatalf("complete %d: %v", index, err)
		}
	}
}
