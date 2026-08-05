// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

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

// addObservedLabelAggregationHeuristics identifies labels that selected series
// carry but the authored chart does not use for identity, dimensions, promoted
// metadata, selector routing, or explicit by_labels exclusion. Aggregation can
// be intentional; the warning makes the lost comparison/filter explicit.
func addObservedLabelAggregationHeuristics(
	spec *charttpl.Spec,
	reader metrix.Reader,
	routes *planRouteSummary,
	r *report,
) {
	if spec == nil || routes == nil {
		return
	}

	grouped := make(map[string]*observedLabelAggregation)
	for _, ref := range enumerateChartRefs(spec) {
		templateID, ok := chartengine.ChartTemplateIDAt(ref.groupPath, ref.chartIndex)
		if !ok {
			continue // The authoritative compiler reports template identity errors.
		}
		template := routes.templates[templateID]
		if template == nil || len(template.resolvedSeries) == 0 {
			continue
		}
		handled, wildcard := handledChartLabels(ref.chart)
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

		aggregated := make(map[string]struct{})
		reader.ForEachSeriesIdentity(func(
			identity metrix.SeriesIdentity,
			_ metrix.SeriesMeta,
			_ string,
			labels metrix.LabelView,
			_ metrix.SampleValue,
		) {
			if _, resolved := template.resolvedSeries[identity.ID]; !resolved {
				return
			}
			labels.Range(func(key, _ string) bool {
				key = strings.TrimSpace(key)
				if key == "" || key == metrix.HistogramBucketLabel || key == metrix.SummaryQuantileLabel {
					return true
				}
				if _, ok := handled[key]; !ok {
					aggregated[key] = struct{}{}
				}
				return true
			})
		})
		if len(aggregated) == 0 {
			continue
		}

		keys := slices.Sorted(maps.Keys(aggregated))
		groupKey := strings.Join(keys, "\x00")
		group := grouped[groupKey]
		if group == nil {
			group = &observedLabelAggregation{keys: keys}
			grouped[groupKey] = group
		}
		group.paths = append(group.paths, ref.path)
		group.titles = append(group.titles, ref.chart.Title)
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
}

func handledChartLabels(chart charttpl.Chart) (map[string]struct{}, bool) {
	handled := make(map[string]struct{})
	for _, key := range chart.LabelPromoted {
		if key = strings.TrimSpace(key); key != "" {
			handled[key] = struct{}{}
		}
	}

	wildcard := false
	if chart.Instances == nil {
		return handled, wildcard
	}
	for _, identityToken := range chart.Instances.ByLabels {
		identityToken = strings.TrimSpace(identityToken)
		switch {
		case identityToken == "*":
			wildcard = true
		case strings.HasPrefix(identityToken, "!"):
			if key := strings.TrimSpace(strings.TrimPrefix(identityToken, "!")); key != "" {
				handled[key] = struct{}{}
			}
		case identityToken != "":
			handled[identityToken] = struct{}{}
		}
	}
	return handled, wildcard
}
