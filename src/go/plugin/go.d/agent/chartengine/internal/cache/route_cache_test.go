// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestRouteCache(t *testing.T) {
	t.Run("stores positive and negative entries", func(t *testing.T) {
		rc := NewRouteCache[string]()

		a := metrix.SeriesIdentity{ID: "a", Hash64: 1}
		b := metrix.SeriesIdentity{ID: "b", Hash64: 1}

		rc.Store(a, 1, 1, []string{"chart-a"})
		rc.Store(b, 1, 1, nil)

		valsA, ok := rc.Lookup(a, 1, 1)
		assert.True(t, ok)
		assert.Equal(t, []string{"chart-a"}, valsA)

		valsB, ok := rc.Lookup(b, 1, 1)
		assert.True(t, ok)
		assert.Nil(t, valsB)
	})

	t.Run("revision mismatch misses", func(t *testing.T) {
		rc := NewRouteCache[string]()
		id := metrix.SeriesIdentity{ID: "a", Hash64: 1}
		rc.Store(id, 1, 1, []string{"chart-a"})

		_, ok := rc.Lookup(id, 2, 2)
		assert.False(t, ok)
	})

	t.Run("revision mismatch does not retain stale entry in same build", func(t *testing.T) {
		rc := NewRouteCache[string]()
		id := metrix.SeriesIdentity{ID: "a", Hash64: 2}
		rc.Store(id, 1, 1, []string{"chart-a"})

		_, ok := rc.Lookup(id, 2, 2)
		assert.False(t, ok)

		stats := rc.RetainSeen(2)
		assert.Equal(t, 1, stats.EntriesBefore)
		assert.Equal(t, 0, stats.EntriesAfter)
		assert.Equal(t, 1, stats.Pruned)
		assert.True(t, stats.FullDrop)
	})

	t.Run("retain prunes by seen build sequence", func(t *testing.T) {
		rc := NewRouteCache[string]()
		a := metrix.SeriesIdentity{ID: "a", Hash64: 10}
		b := metrix.SeriesIdentity{ID: "b", Hash64: 11}
		c := metrix.SeriesIdentity{ID: "c", Hash64: 12}

		rc.Store(a, 1, 1, []string{"chart-a"})
		rc.Store(b, 1, 1, []string{"chart-b"})
		rc.Store(c, 1, 1, nil)

		rc.MarkSeenIfPresent(a, 2)
		rc.MarkSeenIfPresent(c, 2)
		stats := rc.RetainSeen(2)
		assert.Equal(t, 3, stats.EntriesBefore)
		assert.Equal(t, 2, stats.EntriesAfter)
		assert.Equal(t, 1, stats.Pruned)
		assert.False(t, stats.FullDrop)

		_, ok := rc.Lookup(a, 1, 2)
		assert.True(t, ok)
		_, ok = rc.Lookup(b, 1, 2)
		assert.False(t, ok)
		_, ok = rc.Lookup(c, 1, 2)
		assert.True(t, ok)
	})

	t.Run("retain full-drops when no entries seen in build", func(t *testing.T) {
		rc := NewRouteCache[string]()
		a := metrix.SeriesIdentity{ID: "a", Hash64: 20}
		b := metrix.SeriesIdentity{ID: "b", Hash64: 21}
		rc.Store(a, 1, 1, []string{"chart-a"})
		rc.Store(b, 1, 1, []string{"chart-b"})

		stats := rc.RetainSeen(2)
		assert.Equal(t, 2, stats.EntriesBefore)
		assert.Equal(t, 0, stats.EntriesAfter)
		assert.Equal(t, 2, stats.Pruned)
		assert.True(t, stats.FullDrop)

		_, ok := rc.Lookup(a, 1, 2)
		assert.False(t, ok)
		_, ok = rc.Lookup(b, 1, 2)
		assert.False(t, ok)
	})
}
