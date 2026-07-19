// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestClaimAuthorityFIFOAndDirectCancellation(t *testing.T) {
	authority := newClaimAuthority()
	first := claimTestOperation(t, authority, 1, "first", []string{"b"}, nil)
	second := claimTestOperation(t, authority, 2, "second", []string{"a", "b"}, nil)
	third := claimTestOperation(t, authority, 3, "third", []string{"a"}, nil)

	acquireGranted, acquireErr := authority.acquire(first)
	require.False(t, acquireErr != nil || !acquireGranted)

	acquireGranted2, acquireErr2 := authority.acquire(second)
	require.False(t, acquireErr2 != nil || acquireGranted2)

	acquireGranted3, acquireErr3 := authority.acquire(third)
	require.False(t, acquireErr3 != nil || acquireGranted3)

	granted, err := authority.release(first)
	require.False(t, err != nil || len(granted) != 1 || granted[0] != second)
	granted, err = authority.release(second)
	require.False(t, err != nil || len(granted) != 1 || granted[0] != third)

	_, releaseErr := authority.release(third)
	require.NoError(t, releaseErr)

	blocker := claimTestOperation(t, authority, 4, "blocker", []string{"b"}, nil)
	cancelled := claimTestOperation(t, authority, 5, "cancelled", []string{"a", "b"}, nil)
	survivor := claimTestOperation(t, authority, 6, "survivor", []string{"b"}, nil)
	_, _ = authority.acquire(blocker)
	_, _ = authority.acquire(cancelled)
	_, _ = authority.acquire(survivor)
	granted, err = authority.cancel(cancelled)
	require.NoError(t, err)
	require.EqualValues(t, 0, len(granted))
	granted, err = authority.release(blocker)
	require.False(t, err != nil || len(granted) != 1 || granted[0] != survivor)

	_, releaseErr2 := authority.release(survivor)
	require.NoError(t, releaseErr2)

	require.False(t, authority.waitingCount() != 0 || len(authority.keys) != 0)
}

func TestClaimAuthorityLexicographicOrderRetainsAndReleasesPrefix(t *testing.T) {
	authority := newClaimAuthority()
	holder := claimTestOperation(t, authority, 1, "holder", []string{"b"}, nil)
	parked := claimTestOperation(t, authority, 2, "parked", []string{"b", "a", "a"}, nil)
	probe := claimTestOperation(t, authority, 3, "probe", []string{"a"}, nil)

	acquireGranted, acquireErr := authority.acquire(holder)
	require.False(t, acquireErr != nil || !acquireGranted)

	acquireGranted2, acquireErr2 := authority.acquire(parked)
	require.False(t, acquireErr2 != nil || acquireGranted2)

	require.False(t, parked.claimCursor != 1 || !parked.authorityClaimEdges[0].held || parked.authorityClaimEdges[0].claim.key != "a" || !parked.authorityClaimEdges[1].waiting)

	acquireGranted3, acquireErr3 := authority.acquire(probe)
	require.False(t, acquireErr3 != nil || acquireGranted3)

	granted, err := authority.cancel(parked)
	require.False(t, err != nil || len(granted) != 1 || granted[0] != probe)

	_, releaseErr := authority.release(probe)
	require.NoError(t, releaseErr)

	_, releaseErr2 := authority.release(holder)
	require.NoError(t, releaseErr2)

}

func TestClaimAuthorityFIFOReadersWriters(t *testing.T) {
	authority := newClaimAuthority()
	reader1 := claimTestOperation(t, authority, 1, "reader-1", nil, []string{"shared"})
	writer := claimTestOperation(t, authority, 2, "writer", []string{"shared"}, nil)
	reader2 := claimTestOperation(t, authority, 3, "reader-2", nil, []string{"shared"})

	acquireGranted, acquireErr := authority.acquire(reader1)
	require.False(t, acquireErr != nil || !acquireGranted)

	acquireGranted2, acquireErr2 := authority.acquire(writer)
	require.False(t, acquireErr2 != nil || acquireGranted2)

	acquireGranted3, acquireErr3 := authority.acquire(reader2)
	require.False(t, acquireErr3 != nil || acquireGranted3)

	granted, err := authority.release(reader1)
	require.False(t, err != nil || len(granted) != 1 || granted[0] != writer)
	granted, err = authority.release(writer)
	require.False(t, err != nil || len(granted) != 1 || granted[0] != reader2)

	_, releaseErr := authority.release(reader2)
	require.NoError(t, releaseErr)

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
				_, measuredErr = fixture.authority.cancel(fixture.target)
			} else {
				_, measuredErr = fixture.authority.release(fixture.target)
			}
		})
		require.NoError(t, measuredErr)
		return allocations
	}

	require.EqualValues(t, 0, measure(true))

	require.EqualValues(t, 0, measure(false))
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

	acquireGranted, acquireErr := authority.acquire(holder)
	require.False(t, acquireErr != nil || !acquireGranted)

	for index := range population {
		waiter := claimTestOperation(
			t,
			authority,
			lifecycle.OperationID(index+2),
			fmt.Sprintf("waiter-%d", index),
			nil,
			[]string{"shared"},
		)

		granted, err := authority.acquire(waiter)
		require.False(t, err != nil || granted)
	}
	granted, err := authority.release(holder)
	require.NoError(t, err)
	require.False(t, len(granted) > quantum)
	allGranted := append([]*commandOperation(nil), granted...)
	for authority.pendingSettlements() {
		batch, more, serviceErr := authority.serviceSettlements(quantum)
		require.NoError(t, serviceErr)
		require.False(t, len(batch) > quantum)
		allGranted = append(allGranted, batch...)
		require.EqualValues(t, authority.pendingSettlements(), more)
	}
	require.EqualValues(t, population, len(allGranted))
	for index, operation := range allGranted {
		want := fmt.Sprintf("waiter-%d", index)
		require.EqualValues(t, want, operation.UID)

		_, releaseErr := authority.release(operation)
		require.NoError(t, releaseErr)

	}
	require.False(t, authority.waitingCount() != 0 || len(authority.keys) != 0)
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
		if granted, err := authority.acquire(blocker); err != nil || !granted {
			require.FailNowf(tb, "benchmark failed", "blocker acquire: granted=%v err=%v", granted, err)
		}
		if granted, err := authority.acquire(target); err != nil || granted {
			require.FailNowf(tb, "benchmark failed", "target acquire: granted=%v err=%v", granted, err)
		}
	} else if granted, err := authority.acquire(target); err != nil || !granted {
		require.FailNowf(tb, "benchmark failed", "target acquire: granted=%v err=%v", granted, err)
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
				if granted, err := authority.acquire(blocker); err != nil || !granted {
					require.FailNow(b, "benchmark failed", err)
				}
				granted, err := authority.acquire(target)
				if err != nil || granted {
					require.FailNow(b, "benchmark failed", err)
				}
				if _, err := authority.cancel(target); err != nil {
					require.FailNow(b, "benchmark failed", err)
				}
			}
		})
	}
}

func claimTestOperation(tb testing.TB, authority *claimAuthority, id lifecycle.OperationID, uid string, writes, reads []string) *commandOperation {
	tb.Helper()
	generation, err := lifecycle.NewOperation(id, uid, lifecycle.SourceJobManager, uid, true)
	if err != nil {
		require.FailNow(tb, "benchmark failed", err)
	}
	normalized, err := normalizeAuthorityClaimModes(writes, reads)
	if err != nil {
		require.FailNow(tb, "benchmark failed", err)
	}
	operation := &commandOperation{OperationGeneration: generation}
	prepareClaimEdges(operation, normalized)
	if err := authority.register(operation); err != nil {
		require.FailNow(tb, "benchmark failed", err)
	}
	return operation
}
