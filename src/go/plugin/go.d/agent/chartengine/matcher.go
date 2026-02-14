// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

type routeBinding struct {
	ChartTemplateID   string
	ChartID           string
	DimensionIndex    int
	DimensionName     string
	DimensionKeyLabel string
	Algorithm         program.Algorithm
	Hidden            bool
	Static            bool
	Inferred          bool
	Autogen           bool
	Meta              program.ChartMeta
	Lifecycle         program.LifecyclePolicy
}

type routeCandidate struct {
	chartTemplateID string
	dimensionIndex  int
	dimension       program.Dimension
}

type matchIndex struct {
	chartsByID       map[string]program.Chart
	byMetricName     map[string][]routeCandidate
	wildcardMatchers []routeCandidate
}

func buildMatchIndex(charts []program.Chart) matchIndex {
	index := matchIndex{
		chartsByID:       make(map[string]program.Chart, len(charts)),
		byMetricName:     make(map[string][]routeCandidate),
		wildcardMatchers: make([]routeCandidate, 0),
	}

	for _, chart := range charts {
		index.chartsByID[chart.TemplateID] = chart
		for i, dim := range chart.Dimensions {
			candidate := routeCandidate{
				chartTemplateID: chart.TemplateID,
				dimensionIndex:  i,
				dimension:       dim,
			}
			if len(dim.Selector.MetricNames) == 0 {
				index.wildcardMatchers = append(index.wildcardMatchers, candidate)
				continue
			}
			for _, metricName := range dim.Selector.MetricNames {
				index.byMetricName[metricName] = append(index.byMetricName[metricName], candidate)
			}
		}
	}

	return index
}

type routeCacheEntry struct {
	identity metrix.SeriesIdentity
	revision uint64
	bindings []routeBinding
}

type routeCache struct {
	// Intentionally unbounded:
	// - cardinality/retention source-of-truth lives in metrix store ingress/retention,
	// - chartengine must not introduce a second independent cap.
	// Entries are pruned by retainSeries() each successful plan build using
	// snapshot membership provided by planner.
	mu      sync.RWMutex
	buckets map[uint64][]routeCacheEntry
}

func newRouteCache() *routeCache {
	return &routeCache{
		buckets: make(map[uint64][]routeCacheEntry),
	}
}

func (c *routeCache) lookup(identity metrix.SeriesIdentity, revision uint64) ([]routeBinding, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bucket := c.buckets[identity.Hash64]
	for _, entry := range bucket {
		if entry.identity.ID != identity.ID {
			continue
		}
		if entry.revision != revision {
			return nil, false
		}
		return cloneRouteBindings(entry.bindings), true
	}
	return nil, false
}

func (c *routeCache) store(identity metrix.SeriesIdentity, revision uint64, bindings []routeBinding) {
	c.mu.Lock()
	defer c.mu.Unlock()

	bucket := c.buckets[identity.Hash64]
	for i := range bucket {
		if bucket[i].identity.ID != identity.ID {
			continue
		}
		bucket[i].revision = revision
		bucket[i].bindings = cloneRouteBindings(bindings)
		c.buckets[identity.Hash64] = bucket
		return
	}

	c.buckets[identity.Hash64] = append(bucket, routeCacheEntry{
		identity: identity,
		revision: revision,
		bindings: cloneRouteBindings(bindings),
	})
}

// retainSeries keeps cache entries only for series IDs currently present in the
// metrics snapshot passed to planner. This makes cache lifecycle follow metrix
// store retention/source-of-truth.
func (c *routeCache) retainSeries(alive map[metrix.SeriesID]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(alive) == 0 {
		for hash := range c.buckets {
			delete(c.buckets, hash)
		}
		return
	}

	for hash, bucket := range c.buckets {
		kept := bucket[:0]
		for i := range bucket {
			if _, ok := alive[bucket[i].identity.ID]; !ok {
				continue
			}
			kept = append(kept, bucket[i])
		}
		if len(kept) == 0 {
			delete(c.buckets, hash)
			continue
		}
		c.buckets[hash] = kept
	}
}

func cloneRouteBindings(in []routeBinding) []routeBinding {
	if len(in) == 0 {
		return nil
	}
	out := make([]routeBinding, len(in))
	copy(out, in)
	return out
}

func (e *Engine) resolveSeriesRoutes(
	cache *routeCache,
	identity metrix.SeriesIdentity,
	name string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	index matchIndex,
	revision uint64,
) ([]routeBinding, bool, error) {
	if cache == nil {
		return nil, false, fmt.Errorf("chartengine: route cache is not initialized")
	}

	if cached, ok := cache.lookup(identity, revision); ok {
		return cached, true, nil
	}

	candidates := make([]routeCandidate, 0, len(index.byMetricName[name])+len(index.wildcardMatchers))
	candidates = append(candidates, index.byMetricName[name]...)
	candidates = append(candidates, index.wildcardMatchers...)

	routes := make([]routeBinding, 0)
	for _, candidate := range candidates {
		if !candidate.dimension.Selector.Matcher.Matches(name, labels) {
			continue
		}
		chart, ok := index.chartsByID[candidate.chartTemplateID]
		if !ok {
			return nil, false, fmt.Errorf("chartengine: route references unknown chart template %q", candidate.chartTemplateID)
		}
		chartID, ok, err := renderChartInstanceIDFromView(chart.Identity, labels)
		if err != nil {
			return nil, false, err
		}
		if !ok || strings.TrimSpace(chartID) == "" {
			continue
		}
		dimName, dimKeyLabel, ok, err := resolveDimensionName(candidate.dimension, name, labels, meta)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		routes = append(routes, routeBinding{
			ChartTemplateID:   candidate.chartTemplateID,
			ChartID:           chartID,
			DimensionIndex:    candidate.dimensionIndex,
			DimensionName:     dimName,
			DimensionKeyLabel: dimKeyLabel,
			Algorithm:         chart.Meta.Algorithm,
			Hidden:            candidate.dimension.Hidden,
			Static:            !candidate.dimension.Dynamic,
			Inferred:          candidate.dimension.InferNameFromSeriesMeta,
			Autogen:           false,
			Meta:              chart.Meta,
			Lifecycle:         chart.Lifecycle,
		})
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].ChartID != routes[j].ChartID {
			return routes[i].ChartID < routes[j].ChartID
		}
		if routes[i].ChartTemplateID != routes[j].ChartTemplateID {
			return routes[i].ChartTemplateID < routes[j].ChartTemplateID
		}
		if routes[i].DimensionIndex != routes[j].DimensionIndex {
			return routes[i].DimensionIndex < routes[j].DimensionIndex
		}
		return routes[i].DimensionName < routes[j].DimensionName
	})

	cache.store(identity, revision, routes)
	return routes, false, nil
}
