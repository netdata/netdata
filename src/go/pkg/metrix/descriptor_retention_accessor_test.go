// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDescriptorRetentionAccessor pins the optional-interface contract a descriptor-caching
// consumer (the prometheus writer) uses to keep its per-name state alive at least as long as
// metrix keeps the descriptor: the window is expire+grace, the clock advances only on
// successful commits, and both the view and the managed store expose it.
func TestDescriptorRetentionAccessor(t *testing.T) {
	t.Run("window is expire+grace", func(t *testing.T) {
		s := NewCollectorStore(WithExpireAfterSuccessCycles(4), WithDescriptorGraceCycles(6))
		dr, ok := s.(DescriptorRetention)
		require.True(t, ok, "collector store must expose DescriptorRetention")
		require.Equal(t, uint64(10), dr.DescriptorRetentionWindow())
		require.Equal(t, uint64(0), dr.SuccessfulCommits())
	})

	t.Run("default window couples grace to the default expire", func(t *testing.T) {
		s := NewCollectorStore() // expire=10, grace defaults to expire=10
		require.Equal(t, uint64(20), s.(DescriptorRetention).DescriptorRetentionWindow())
	})

	t.Run("disabled series expiry reports an unbounded window", func(t *testing.T) {
		s := NewCollectorStore(WithExpireAfterSuccessCycles(0))
		require.Equal(t, DescriptorRetentionUnbounded, s.(DescriptorRetention).DescriptorRetentionWindow())
	})

	t.Run("disabled series expiry is unbounded even with an explicit grace", func(t *testing.T) {
		s := NewCollectorStore(WithExpireAfterSuccessCycles(0), WithDescriptorGraceCycles(5))
		require.Equal(t, DescriptorRetentionUnbounded, s.(DescriptorRetention).DescriptorRetentionWindow())
	})

	t.Run("an overflowing expire+grace saturates to unbounded", func(t *testing.T) {
		s := NewCollectorStore(WithExpireAfterSuccessCycles(10), WithDescriptorGraceCycles(math.MaxUint64-5))
		require.Equal(t, DescriptorRetentionUnbounded, s.(DescriptorRetention).DescriptorRetentionWindow())
	})

	t.Run("the clock advances only on successful commits", func(t *testing.T) {
		s := NewCollectorStore()
		cc := cycleController(t, s)

		cc.BeginCycle()
		require.NoError(t, cc.CommitCycleSuccess())
		cc.BeginCycle()
		require.NoError(t, cc.CommitCycleSuccess())
		cc.BeginCycle()
		cc.AbortCycle() // an aborted cycle must not advance the clock

		require.Equal(t, uint64(2), s.(DescriptorRetention).SuccessfulCommits())
	})

	t.Run("the managed store also exposes it", func(t *testing.T) {
		s := NewCollectorStore()
		m, ok := AsCycleManagedStore(s)
		require.True(t, ok)
		_, ok = m.(DescriptorRetention)
		require.True(t, ok, "the managed store must also expose DescriptorRetention")
	})
}
