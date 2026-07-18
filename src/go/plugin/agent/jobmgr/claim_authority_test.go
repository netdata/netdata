// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestClaimAuthorityFIFOAndDirectCancellation(t *testing.T) {
	authority := newClaimAuthority()
	first := claimTestOperation(t, authority, 1, "first", []string{"b"}, nil)
	second := claimTestOperation(t, authority, 2, "second", []string{"a", "b"}, nil)
	third := claimTestOperation(t, authority, 3, "third", []string{"a"}, nil)
	if granted, err := authority.Acquire(first); err != nil || !granted {
		t.Fatalf("first acquire: granted=%v err=%v", granted, err)
	}
	if granted, err := authority.Acquire(second); err != nil || granted {
		t.Fatalf("second acquire: granted=%v err=%v", granted, err)
	}
	if granted, err := authority.Acquire(third); err != nil || granted {
		t.Fatalf("third bypassed older conflict: granted=%v err=%v", granted, err)
	}
	granted, err := authority.Release(first)
	if err != nil || len(granted) != 1 || granted[0] != second {
		t.Fatalf("first release granted %#v err=%v", granted, err)
	}
	granted, err = authority.Release(second)
	if err != nil || len(granted) != 1 || granted[0] != third {
		t.Fatalf("second release granted %#v err=%v", granted, err)
	}
	if _, err := authority.Release(third); err != nil {
		t.Fatal(err)
	}

	blocker := claimTestOperation(t, authority, 4, "blocker", []string{"b"}, nil)
	cancelled := claimTestOperation(t, authority, 5, "cancelled", []string{"a", "b"}, nil)
	survivor := claimTestOperation(t, authority, 6, "survivor", []string{"b"}, nil)
	_, _ = authority.Acquire(blocker)
	_, _ = authority.Acquire(cancelled)
	_, _ = authority.Acquire(survivor)
	granted, err = authority.Cancel(cancelled)
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 0 {
		t.Fatalf("cancel granted behind a live holder: granted=%#v", granted)
	}
	granted, err = authority.Release(blocker)
	if err != nil || len(granted) != 1 || granted[0] != survivor {
		t.Fatalf("cancelled waiter did not expose survivor to the holder release: granted=%#v err=%v", granted, err)
	}
	if _, err := authority.Release(survivor); err != nil {
		t.Fatal(err)
	}
	if authority.WaitingCount() != 0 || len(authority.keys) != 0 {
		t.Fatalf("claim census leaked: waiters=%d keys=%d", authority.WaitingCount(), len(authority.keys))
	}
}

func TestClaimAuthorityLexicographicOrderRetainsAndReleasesPrefix(t *testing.T) {
	authority := newClaimAuthority()
	holder := claimTestOperation(t, authority, 1, "holder", []string{"b"}, nil)
	parked := claimTestOperation(t, authority, 2, "parked", []string{"b", "a", "a"}, nil)
	probe := claimTestOperation(t, authority, 3, "probe", []string{"a"}, nil)
	if granted, err := authority.Acquire(holder); err != nil || !granted {
		t.Fatalf("holder acquire: granted=%v err=%v", granted, err)
	}
	if granted, err := authority.Acquire(parked); err != nil || granted {
		t.Fatalf("parked acquire: granted=%v err=%v", granted, err)
	}
	if parked.claimCursor != 1 || !parked.authorityClaimEdges[0].held || parked.authorityClaimEdges[0].claim.key != "a" || !parked.authorityClaimEdges[1].waiting {
		t.Fatalf("lexicographic held prefix differs: cursor=%d edges=%#v", parked.claimCursor, parked.authorityClaimEdges)
	}
	if granted, err := authority.Acquire(probe); err != nil || granted {
		t.Fatalf("probe bypassed held prefix: granted=%v err=%v", granted, err)
	}
	granted, err := authority.Cancel(parked)
	if err != nil || len(granted) != 1 || granted[0] != probe {
		t.Fatalf("prefix cancellation did not expose probe: granted=%#v err=%v", granted, err)
	}
	if _, err := authority.Release(probe); err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Release(holder); err != nil {
		t.Fatal(err)
	}
}

func TestClaimAuthorityFIFOReadersWriters(t *testing.T) {
	authority := newClaimAuthority()
	reader1 := claimTestOperation(t, authority, 1, "reader-1", nil, []string{"shared"})
	writer := claimTestOperation(t, authority, 2, "writer", []string{"shared"}, nil)
	reader2 := claimTestOperation(t, authority, 3, "reader-2", nil, []string{"shared"})
	if granted, err := authority.Acquire(reader1); err != nil || !granted {
		t.Fatalf("first reader acquire: granted=%v err=%v", granted, err)
	}
	if granted, err := authority.Acquire(writer); err != nil || granted {
		t.Fatalf("writer acquire: granted=%v err=%v", granted, err)
	}
	if granted, err := authority.Acquire(reader2); err != nil || granted {
		t.Fatalf("later reader bypassed writer: granted=%v err=%v", granted, err)
	}
	granted, err := authority.Release(reader1)
	if err != nil || len(granted) != 1 || granted[0] != writer {
		t.Fatalf("writer was not first after reader release: granted=%#v err=%v", granted, err)
	}
	granted, err = authority.Release(writer)
	if err != nil || len(granted) != 1 || granted[0] != reader2 {
		t.Fatalf("second reader was not resumed after writer: granted=%#v err=%v", granted, err)
	}
	if _, err := authority.Release(reader2); err != nil {
		t.Fatal(err)
	}
}

func TestClaimAuthorityCancelAndReleaseAllocateNothing(t *testing.T) {
	measure := func(cancel bool) float64 {
		fixtures := [2]claimAllocationFixture{}
		for index := range fixtures {
			fixtures[index] = newClaimAllocationFixture(t, cancel, lifecycle.OperationID(index*10+1))
		}
		index := 0
		var measuredErr error
		allocations := testing.AllocsPerRun(1, func() {
			fixture := &fixtures[index]
			index++
			if cancel {
				_, measuredErr = fixture.authority.Cancel(fixture.target)
			} else {
				_, measuredErr = fixture.authority.Release(fixture.target)
			}
		})
		if measuredErr != nil {
			t.Fatal(measuredErr)
		}
		return allocations
	}
	if allocations := measure(true); allocations != 0 {
		t.Fatalf("direct cancellation allocated %.1f objects", allocations)
	}
	if allocations := measure(false); allocations != 0 {
		t.Fatalf("release allocated %.1f objects", allocations)
	}
}

func TestClaimAuthoritySettlementUsesBoundedTurns(t *testing.T) {
	const (
		population = 257
		quantum    = 4
	)
	authority := newClaimAuthority()
	holder := claimTestOperation(
		t,
		authority,
		1,
		"holder",
		[]string{"shared"},
		nil,
	)
	if granted, err := authority.Acquire(holder); err != nil || !granted {
		t.Fatalf("holder acquire: granted=%v err=%v", granted, err)
	}
	for index := range population {
		waiter := claimTestOperation(
			t,
			authority,
			lifecycle.OperationID(index+2),
			fmt.Sprintf("waiter-%d", index),
			nil,
			[]string{"shared"},
		)
		if granted, err := authority.Acquire(waiter); err != nil || granted {
			t.Fatalf(
				"waiter %d acquire: granted=%v err=%v",
				index,
				granted,
				err,
			)
		}
	}
	granted, err := authority.Release(holder)
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) > quantum {
		t.Fatalf(
			"one settlement turn granted %d operations, want at most %d",
			len(granted),
			quantum,
		)
	}
	allGranted := append([]*commandOperation(nil), granted...)
	for authority.PendingSettlements() {
		batch, more, serviceErr := authority.ServiceSettlements(quantum)
		if serviceErr != nil {
			t.Fatal(serviceErr)
		}
		if len(batch) > quantum {
			t.Fatalf(
				"continuation settlement granted %d operations, want at most %d",
				len(batch),
				quantum,
			)
		}
		allGranted = append(allGranted, batch...)
		if more != authority.PendingSettlements() {
			t.Fatalf(
				"settlement continuation differs: returned=%v pending=%v",
				more,
				authority.PendingSettlements(),
			)
		}
	}
	if len(allGranted) != population {
		t.Fatalf(
			"settlement granted %d operations, want %d",
			len(allGranted),
			population,
		)
	}
	for index, operation := range allGranted {
		want := fmt.Sprintf("waiter-%d", index)
		if operation.UID != want {
			t.Fatalf(
				"settlement order[%d]=%q, want %q",
				index,
				operation.UID,
				want,
			)
		}
		if _, err := authority.Release(operation); err != nil {
			t.Fatalf("release %q: %v", operation.UID, err)
		}
	}
	if authority.WaitingCount() != 0 || len(authority.keys) != 0 {
		t.Fatalf(
			"settlement retained waiters=%d keys=%d",
			authority.WaitingCount(),
			len(authority.keys),
		)
	}
}

type claimAllocationFixture struct {
	authority *claimAuthority
	target    *commandOperation
}

func newClaimAllocationFixture(tb testing.TB, cancel bool, base lifecycle.OperationID) claimAllocationFixture {
	tb.Helper()
	authority := newClaimAuthority()
	target := claimTestOperation(tb, authority, base, fmt.Sprintf("target-%d", base), []string{"a", "b", "c", "d"}, nil)
	if cancel {
		blocker := claimTestOperation(tb, authority, base+1, fmt.Sprintf("blocker-%d", base), []string{"d"}, nil)
		if granted, err := authority.Acquire(blocker); err != nil || !granted {
			tb.Fatalf("blocker acquire: granted=%v err=%v", granted, err)
		}
		if granted, err := authority.Acquire(target); err != nil || granted {
			tb.Fatalf("target acquire: granted=%v err=%v", granted, err)
		}
	} else if granted, err := authority.Acquire(target); err != nil || !granted {
		tb.Fatalf("target acquire: granted=%v err=%v", granted, err)
	}
	return claimAllocationFixture{authority: authority, target: target}
}

func BenchmarkClaimAuthorityAcquireCancel(b *testing.B) {
	for _, keys := range []int{1, 4, 16} {
		b.Run(fmt.Sprintf("keys-%d", keys), func(b *testing.B) {
			b.ReportAllocs()
			claims := make([]string, keys)
			for index := range claims {
				claims[index] = fmt.Sprintf("claim-%02d", index)
			}
			b.ResetTimer()
			for b.Loop() {
				authority := newClaimAuthority()
				blocker := claimTestOperation(b, authority, 1, "blocker", []string{claims[keys-1]}, nil)
				target := claimTestOperation(b, authority, 2, "target", claims, nil)
				if granted, err := authority.Acquire(blocker); err != nil || !granted {
					b.Fatal(err)
				}
				granted, err := authority.Acquire(target)
				if err != nil || granted {
					b.Fatal(err)
				}
				if _, err := authority.Cancel(target); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func claimTestOperation(tb testing.TB, authority *claimAuthority, id lifecycle.OperationID, uid string, writes, reads []string) *commandOperation {
	tb.Helper()
	generation, err := lifecycle.NewOperation(id, uid, lifecycle.SourceJobManager, uid, true)
	if err != nil {
		tb.Fatal(err)
	}
	normalized, err := normalizeAuthorityClaimModes(writes, reads)
	if err != nil {
		tb.Fatal(err)
	}
	operation := &commandOperation{OperationGeneration: generation}
	prepareClaimEdges(operation, normalized)
	if err := authority.Register(operation); err != nil {
		tb.Fatal(err)
	}
	return operation
}
