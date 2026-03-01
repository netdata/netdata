// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestAutogenRouteBuilderScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"build scalar autogen route": {
			run: runTestBuildScalarAutogenRoute,
		},
		"build histogram bucket autogen route": {
			run: runTestBuildHistogramBucketAutogenRoute,
		},
		"build summary quantile autogen route": {
			run: runTestBuildSummaryQuantileAutogenRoute,
		},
		"build state-set autogen route": {
			run: runTestBuildStateSetAutogenRoute,
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func runTestBuildScalarAutogenRoute(t *testing.T) {
	tests := map[string]struct {
		metricName string
		labels     map[string]string
		meta       metrix.SeriesMeta
		wantID     string
		wantDim    string
		wantAlg    program.Algorithm
		wantUnits  string
	}{
		"counter metric uses incremental algorithm and static dimension": {
			metricName: "svc.requests_total",
			labels: map[string]string{
				"instance": "db1",
				"job":      "mysql",
			},
			meta:      metrix.SeriesMeta{Kind: metrix.MetricKindCounter},
			wantID:    "svc.requests_total-instance=db1-job=mysql",
			wantDim:   "requests_total",
			wantAlg:   program.AlgorithmIncremental,
			wantUnits: "events/s",
		},
		"gauge metric uses absolute algorithm": {
			metricName: "svc.temperature_celsius",
			labels: map[string]string{
				"instance": "db1",
			},
			meta:      metrix.SeriesMeta{Kind: metrix.MetricKindGauge},
			wantID:    "svc.temperature_celsius-instance=db1",
			wantDim:   "temperature_celsius",
			wantAlg:   program.AlgorithmAbsolute,
			wantUnits: "celsius",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			route, ok, err := buildScalarAutogenRoute(
				tc.metricName,
				sortedLabelView(tc.labels),
				tc.meta,
				AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				"",
			)
			require.NoError(t, err)
			require.True(t, ok)

			assert.Equal(t, tc.wantID, route.chartID)
			assert.Equal(t, tc.wantDim, route.dimensionName)
			assert.Equal(t, tc.wantAlg, route.algorithm)
			assert.Equal(t, tc.wantUnits, route.units)
			assert.True(t, route.staticDimension)
		})
	}
}

func runTestBuildHistogramBucketAutogenRoute(t *testing.T) {
	tests := map[string]struct {
		metricName string
		labels     map[string]string
		wantID     string
		wantDim    string
	}{
		"histogram bucket excludes le from chart id and uses bucket dimension": {
			metricName: "svc.latency_seconds_bucket",
			labels: map[string]string{
				"instance": "db1",
				"le":       "0.5",
				"method":   "GET",
			},
			wantID:  "svc.latency_seconds-instance=db1-method=GET",
			wantDim: "bucket_0.5",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			route, ok, err := buildHistogramBucketAutogenRoute(
				tc.metricName,
				sortedLabelView(tc.labels),
				AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				"",
			)
			require.NoError(t, err)
			require.True(t, ok)

			assert.Equal(t, tc.wantID, route.chartID)
			assert.Equal(t, tc.wantDim, route.dimensionName)
			assert.Equal(t, histogramBucketLabel, route.dimensionKeyLabel)
			assert.Equal(t, program.AlgorithmIncremental, route.algorithm)
			assert.False(t, route.staticDimension)
		})
	}
}

func runTestBuildSummaryQuantileAutogenRoute(t *testing.T) {
	tests := map[string]struct {
		metricName string
		labels     map[string]string
		wantID     string
		wantDim    string
	}{
		"summary quantile excludes quantile label and keeps absolute algorithm": {
			metricName: "svc.request_duration_seconds",
			labels: map[string]string{
				"instance": "db1",
				"method":   "GET",
				"quantile": "0.99",
			},
			wantID:  "svc.request_duration_seconds-instance=db1-method=GET",
			wantDim: "quantile_0.99",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			route, ok, err := buildSummaryQuantileAutogenRoute(
				tc.metricName,
				sortedLabelView(tc.labels),
				AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				"",
			)
			require.NoError(t, err)
			require.True(t, ok)

			assert.Equal(t, tc.wantID, route.chartID)
			assert.Equal(t, tc.wantDim, route.dimensionName)
			assert.Equal(t, summaryQuantileLabel, route.dimensionKeyLabel)
			assert.Equal(t, program.AlgorithmAbsolute, route.algorithm)
			assert.False(t, route.staticDimension)
		})
	}
}

func runTestBuildStateSetAutogenRoute(t *testing.T) {
	tests := map[string]struct {
		metricName string
		labels     map[string]string
		meta       metrix.SeriesMeta
		wantID     string
		wantDim    string
	}{
		"stateset uses metric-name label as dimension key and excludes it from chart id": {
			metricName: "service_status",
			labels: map[string]string{
				"instance":       "db1",
				"service_status": "ready",
			},
			meta:    metrix.SeriesMeta{FlattenRole: metrix.FlattenRoleStateSetState},
			wantID:  "service_status-instance=db1",
			wantDim: "ready",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			route, ok, err := buildStateSetAutogenRoute(
				tc.metricName,
				sortedLabelView(tc.labels),
				tc.meta,
				AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				"",
			)
			require.NoError(t, err)
			require.True(t, ok)

			assert.Equal(t, tc.wantID, route.chartID)
			assert.Equal(t, tc.wantDim, route.dimensionName)
			assert.Equal(t, "service_status", route.dimensionKeyLabel)
			assert.Equal(t, "state", route.units)
			assert.Equal(t, program.AlgorithmAbsolute, route.algorithm)
			assert.False(t, route.staticDimension)
		})
	}
}

func TestFitsTypeIDBudget(t *testing.T) {
	tests := map[string]struct {
		maxLen       int
		typeIDPrefix string
		chartID      string
		want         bool
	}{
		"empty type id at exact limit passes": {
			maxLen:  5,
			chartID: "abcde",
			want:    true,
		},
		"empty type id over limit fails": {
			maxLen:  5,
			chartID: "abcdef",
			want:    false,
		},
		"type id includes separator in budget": {
			maxLen:       16,
			typeIDPrefix: "collector.job",
			chartID:      "abc",
			want:         false, // len("collector.job")+1+len("abc") == 17 > 16
		},
		"type id budget overflow fails": {
			maxLen:       15,
			typeIDPrefix: "collector.job",
			chartID:      "abc",
			want:         false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, fitsTypeIDBudget(tc.maxLen, tc.typeIDPrefix, tc.chartID))
		})
	}
}

func sortedLabelView(labels map[string]string) metrix.LabelView {
	items := make([]metrix.Label, 0, len(labels))
	for key, value := range labels {
		items = append(items, metrix.Label{Key: key, Value: value})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})
	return labelSliceView{items: items}
}
