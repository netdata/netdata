// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestRouteCacheStoresPositiveAndNegativeRoutes(t *testing.T) {
	cache := newRouteCache()

	a := metrix.SeriesIdentity{ID: "a", Hash64: 1}
	b := metrix.SeriesIdentity{ID: "b", Hash64: 1}

	cache.Store(a, 1, []routeBinding{{ChartID: "ca"}})
	cache.Store(b, 1, nil) // negative-cache entry

	routesA, ok := cache.Lookup(a, 1)
	assert.True(t, ok)
	assert.Equal(t, "ca", routesA[0].ChartID)

	routesB, ok := cache.Lookup(b, 1)
	assert.True(t, ok)
	assert.Empty(t, routesB)
}

func TestRouteCacheRetainSeriesPrunesBySnapshotMembership(t *testing.T) {
	cache := newRouteCache()

	a := metrix.SeriesIdentity{ID: "a", Hash64: 10}
	b := metrix.SeriesIdentity{ID: "b", Hash64: 11}
	c := metrix.SeriesIdentity{ID: "c", Hash64: 12}

	cache.Store(a, 1, []routeBinding{{ChartID: "ca"}})
	cache.Store(b, 1, []routeBinding{{ChartID: "cb"}})
	cache.Store(c, 1, nil)

	cache.RetainSeries(map[metrix.SeriesID]struct{}{
		"a": {},
		"c": {},
	})

	_, ok := cache.Lookup(a, 1)
	assert.True(t, ok)

	_, ok = cache.Lookup(b, 1)
	assert.False(t, ok)

	_, ok = cache.Lookup(c, 1)
	assert.True(t, ok)
}

func TestRouteCacheLookupMissOnRevisionChange(t *testing.T) {
	cache := newRouteCache()
	id := metrix.SeriesIdentity{ID: "a", Hash64: 1}

	cache.Store(id, 1, []routeBinding{{ChartID: "ca"}})

	_, ok := cache.Lookup(id, 2)
	assert.False(t, ok)
}
