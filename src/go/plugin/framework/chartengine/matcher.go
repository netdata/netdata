// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	routecache "github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/cache"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

type routeBinding struct {
	ChartTemplateID   string
	ChartID           string
	DimensionIndex    int
	DimensionName     string
	DimensionKeyLabel string
	Algorithm         program.Algorithm
	Hidden            bool
	Multiplier        int
	Divisor           int
	Float             bool
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

type routeCache = routecache.RouteCache[routeBinding]

func newRouteCache() *routeCache {
	return routecache.NewRouteCache[routeBinding]()
}

func (e *Engine) resolveSeriesRoutes(
	cache *routeCache,
	identity metrix.SeriesIdentity,
	name string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
	index matchIndex,
	revision uint64,
	buildSeq uint64,
) ([]routeBinding, bool, error) {
	if cache == nil {
		return nil, false, fmt.Errorf("chartengine: route cache is not initialized")
	}

	if cached, ok := cache.Lookup(identity, revision, buildSeq); ok {
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
			Multiplier:        candidate.dimension.Multiplier,
			Divisor:           candidate.dimension.Divisor,
			Float:             candidate.dimension.Float,
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

	cache.Store(identity, revision, buildSeq, routes)
	return routes, false, nil
}
