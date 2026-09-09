// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// addProfileMatchHeuristics checks whether the auto-selection signature also
// accepts common runtime/instrumentation families. Those families can be
// charted, but one generic hit is enough to select the entire profile.
type observedLabelAggregation struct {
	keys   []string
	paths  []string
	titles []string
}

type observedLabelChart struct {
	path       string
	title      string
	handled    map[string]struct{}
	aggregated map[string]struct{}
}

// addObservedLabelAggregationHeuristics identifies labels that selected series
// carry but the authored chart does not use for identity, dimensions, promoted
// metadata, selector routing, or explicit by_labels exclusion. Aggregation can
// be intentional; the warning makes the lost comparison/filter explicit.
func addObservedLabelAggregationHeuristics(
	spec *charttpl.Spec,
	refs []chartRef,
	reader metrix.Reader,
	routes *planRouteSummary,
	r *Report,
) error {
	if spec == nil || routes == nil {
		return nil
	}

	chartsByTemplate := make(map[string]*observedLabelChart)
	charts := make([]*observedLabelChart, 0)
	for _, ref := range refs {
		templateID, ok := chartengine.ChartTemplateIDAt(ref.groupPath, ref.chartIndex)
		if !ok {
			continue // The authoritative compiler reports template identity errors.
		}
		template := routes.templates[templateID]
		if template == nil {
			continue
		}
		handled, wildcard, err := handledChartLabels(ref.chart)
		if err != nil {
			return err
		}
		for key := range template.dimensionKeyLabels {
			handled[key] = struct{}{}
		}
		for _, dimension := range ref.chart.Dimensions {
			compiled, err := metrixselector.ParseCompiled(dimension.Selector)
			if err != nil {
				continue // The authoritative compiler reports selector errors.
			}
			for _, key := range compiled.Meta().ConstrainedLabelKeys {
				handled[strings.TrimSpace(key)] = struct{}{}
			}
			if key := strings.TrimSpace(dimension.NameFromLabel); key != "" {
				handled[key] = struct{}{}
			}
		}
		if wildcard {
			continue
		}
		chart := &observedLabelChart{
			path:       ref.path,
			title:      ref.chart.Title,
			handled:    handled,
			aggregated: make(map[string]struct{}),
		}
		chartsByTemplate[templateID] = chart
		charts = append(charts, chart)
	}

	reader.ForEachSeriesIdentity(func(
		identity metrix.SeriesIdentity,
		_ metrix.SeriesMeta,
		_ string,
		labels metrix.LabelView,
		_ metrix.SampleValue,
	) {
		for templateID := range routes.resolvedTemplatesBySeries[identity.ID] {
			chart := chartsByTemplate[templateID]
			if chart == nil {
				continue
			}
			labels.Range(func(key, _ string) bool {
				key = strings.TrimSpace(key)
				if key == "" {
					return true
				}
				if _, ok := chart.handled[key]; !ok {
					chart.aggregated[key] = struct{}{}
				}
				return true
			})
		}
	})

	grouped := make(map[string]*observedLabelAggregation)
	for _, chart := range charts {
		if len(chart.aggregated) == 0 {
			continue
		}

		keys := slices.Sorted(maps.Keys(chart.aggregated))
		groupKey := strings.Join(keys, "\x00")
		group := grouped[groupKey]
		if group == nil {
			group = &observedLabelAggregation{keys: keys}
			grouped[groupKey] = group
		}
		group.paths = append(group.paths, chart.path)
		group.titles = append(group.titles, chart.title)
	}

	groupKeys := slices.Sorted(maps.Keys(grouped))
	for _, groupKey := range groupKeys {
		group := grouped[groupKey]
		path := "template"
		message := fmt.Sprintf(
			"%d charts aggregate observed label keys %v without an authored role",
			len(group.paths),
			group.keys,
		)
		if len(group.paths) == 1 {
			path = group.paths[0]
			message = fmt.Sprintf(
				"chart %q aggregates observed label keys %v without an authored role",
				group.titles[0],
				group.keys,
			)
		} else {
			if slices.ContainsFunc(group.paths, func(path string) bool { return strings.HasPrefix(path, "profiles[") }) {
				slices.Sort(group.paths)
				path = strings.Join(group.paths, ", ")
			}
			exampleCount := min(3, len(group.titles))
			examples := make([]string, 0, exampleCount)
			for _, title := range group.titles[:exampleCount] {
				examples = append(examples, fmt.Sprintf("%q", title))
			}
			message += " (examples: " + strings.Join(examples, ", ") + ")"
		}
		r.addWarning(
			"observed_label_aggregation",
			path,
			message,
			"An omitted label removes that comparison and may merge distinct entities when new values appear. Aggregation can be correct, but explain the lost filtering/comparison and expected cardinality; do not add identity or promotion merely to silence the warning.",
		)
	}
	return nil
}

func handledChartLabels(chart charttpl.Chart) (map[string]struct{}, bool, error) {
	handled := make(map[string]struct{})
	for _, key := range chart.LabelPromoted {
		if key = strings.TrimSpace(key); key != "" {
			handled[key] = struct{}{}
		}
	}

	if chart.Instances == nil {
		return handled, false, nil
	}
	policy, err := chartengine.ResolveInstanceLabelPolicy(chart.Instances)
	if err != nil {
		return nil, false, err
	}
	for _, key := range policy.RequiredKeys {
		handled[key] = struct{}{}
	}
	for _, key := range policy.OptionalKeys {
		handled[key] = struct{}{}
	}
	for _, key := range policy.ExcludedKeys {
		handled[key] = struct{}{}
	}
	return handled, policy.IncludeAll, nil
}
