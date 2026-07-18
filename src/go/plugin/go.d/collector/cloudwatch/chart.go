// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"maps"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// buildChartTemplate assembles the dynamic chart-template YAML from the selected
// profiles. It is a pure transform of the profiles (no collector state), so it
// is a free function the collector calls with its resolved profiles.
func buildChartTemplate(profiles []cwprofiles.ResolvedProfile) (string, error) {
	return buildChartSpec(profiles).MarshalTemplate()
}

// buildChartSpec builds the chart-template spec and injects template.metrics.
// Rate normalization is applied to observations using the resolved query period,
// so chart dimensions need no static profile divisor. Each profile's template is deep-copied
// (Group.Clone) before injection so the shared profile catalog is never mutated.
// The assembled spec is validated when buildChartTemplate marshals it.
func buildChartSpec(profiles []cwprofiles.ResolvedProfile) charttpl.Spec {
	groups := make([]charttpl.Group, 0, len(profiles)+1)
	for _, rp := range profiles {
		group := rp.Config.Template.Clone()
		series := profileSeries(rp.Name, rp.Config)
		group.Metrics = slices.Sorted(maps.Keys(series))
		groups = append(groups, group)
	}
	groups = append(groups, activityChartGroup())

	return charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: cwprofiles.ContextNamespace,
		Groups:           groups,
	}
}

func activityChartGroup() charttpl.Group {
	return charttpl.Group{
		Family: "Collector Activity",
		Metrics: []string{
			activityAPICallsMetric,
			activityMetricRequestsMetric,
			activityQueriesMetric,
		},
		Charts: []charttpl.Chart{
			{
				ID: "aws_cloudwatch_collector_api_calls", Title: "CloudWatch API Calls",
				Context: "collector_api_calls", Units: "calls", Algorithm: "absolute", Type: "line",
				Instances: &charttpl.Instances{ByLabels: []string{"account_id", "region", "operation"}},
				Dimensions: []charttpl.Dimension{{
					Selector: activityAPICallsMetric, Name: "calls",
				}},
			},
			{
				ID: "aws_cloudwatch_collector_metric_requests", Title: "CloudWatch Metric Requests",
				Context: "collector_metric_requests", Units: "requests", Algorithm: "absolute", Type: "line",
				Instances: &charttpl.Instances{ByLabels: []string{"account_id", "region"}},
				Dimensions: []charttpl.Dimension{{
					Selector: activityMetricRequestsMetric, Name: "requests",
				}},
			},
			{
				ID: "aws_cloudwatch_collector_queries", Title: "CloudWatch Raw Queries",
				Context: "collector_queries", Units: "queries", Algorithm: "absolute", Type: "line",
				Instances: &charttpl.Instances{ByLabels: []string{"account_id", "region", "profile"}},
				Dimensions: []charttpl.Dimension{{
					Selector: activityQueriesMetric, Name: "queries",
				}},
			},
		},
	}
}

func profileSeries(profileName string, prof cwprofiles.Profile) map[string]struct{} {
	out := make(map[string]struct{})
	for _, m := range prof.Metrics {
		for _, stat := range m.Statistics {
			token := cwprofiles.NormalizeStatistic(stat)
			if token == "" {
				continue
			}
			out[cwprofiles.ExportedSeriesName(profileName, m.ID, token)] = struct{}{}
		}
	}
	return out
}
