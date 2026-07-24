// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"
	"testing"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
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
		"build measure-set autogen route": {
			run: runTestBuildMeasureSetAutogenRoute,
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestResolveAutogenSource(t *testing.T) {
	tests := map[string]struct {
		metricName string
		labels     map[string]string
		meta       metrix.SeriesMeta
		want       autogenSource
	}{
		"scalar uses visible name": {
			metricName: "svc.requests_total",
			want:       autogenSource{seriesName: "svc.requests_total", familyName: "svc.requests_total"},
		},
		"histogram bucket uses base family": {
			metricName: "svc.latency_seconds_bucket",
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramBucket,
			},
			want: autogenSource{seriesName: "svc.latency_seconds_bucket", familyName: "svc.latency_seconds"},
		},
		"empty suffix trim preserves visible name": {
			metricName: "_bucket",
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramBucket,
			},
			want: autogenSource{seriesName: "_bucket", familyName: "_bucket"},
		},
		"histogram count uses base family": {
			metricName: "svc.latency_seconds_count",
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramCount,
			},
			want: autogenSource{seriesName: "svc.latency_seconds_count", familyName: "svc.latency_seconds"},
		},
		"histogram sum uses base family": {
			metricName: "svc.latency_seconds_sum",
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramSum,
			},
			want: autogenSource{seriesName: "svc.latency_seconds_sum", familyName: "svc.latency_seconds"},
		},
		"summary quantile already has base family name": {
			metricName: "svc.latency_seconds",
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummaryQuantile,
			},
			want: autogenSource{seriesName: "svc.latency_seconds", familyName: "svc.latency_seconds"},
		},
		"summary count uses base family": {
			metricName: "svc.latency_seconds_count",
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummaryCount,
			},
			want: autogenSource{seriesName: "svc.latency_seconds_count", familyName: "svc.latency_seconds"},
		},
		"summary sum uses base family": {
			metricName: "svc.latency_seconds_sum",
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummarySum,
			},
			want: autogenSource{seriesName: "svc.latency_seconds_sum", familyName: "svc.latency_seconds"},
		},
		"stateset keeps visible name": {
			metricName: "svc.status",
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindStateSet,
				FlattenRole: metrix.FlattenRoleStateSetState,
			},
			want: autogenSource{seriesName: "svc.status", familyName: "svc.status"},
		},
		"measureset derives source and field": {
			metricName: "svc.usage_used",
			labels:     map[string]string{metrix.MeasureSetFieldLabel: "used"},
			meta: metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindMeasureSet,
				FlattenRole: metrix.FlattenRoleMeasureSetField,
			},
			want: autogenSource{seriesName: "svc.usage_used", familyName: "svc.usage", measureField: "used"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := resolveAutogenSource(test.metricName, sortedLabelView(test.labels), test.meta)
			require.True(t, ok)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestResolveAutogenRouteRulesUseSourceFamily(t *testing.T) {
	structured := map[string]struct {
		metricName string
		labels     map[string]string
		meta       metrix.SeriesMeta
		familyName string
	}{
		"histogram bucket": {
			metricName: "svc.latency_seconds_bucket",
			labels:     map[string]string{metrix.HistogramBucketLabel: "1"},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramBucket,
			},
			familyName: "svc.latency_seconds",
		},
		"histogram count": {
			metricName: "svc.latency_seconds_count",
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramCount,
			},
			familyName: "svc.latency_seconds",
		},
		"histogram sum": {
			metricName: "svc.latency_seconds_sum",
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramSum,
			},
			familyName: "svc.latency_seconds",
		},
		"summary quantile": {
			metricName: "svc.latency_seconds",
			labels:     map[string]string{metrix.SummaryQuantileLabel: "0.9"},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindGauge,
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummaryQuantile,
			},
			familyName: "svc.latency_seconds",
		},
		"summary count": {
			metricName: "svc.latency_seconds_count",
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummaryCount,
			},
			familyName: "svc.latency_seconds",
		},
		"summary sum": {
			metricName: "svc.latency_seconds_sum",
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummarySum,
			},
			familyName: "svc.latency_seconds",
		},
		"stateset": {
			metricName: "svc.status",
			labels:     map[string]string{"svc.status": "ok"},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindGauge,
				SourceKind:  metrix.MetricKindStateSet,
				FlattenRole: metrix.FlattenRoleStateSetState,
			},
			familyName: "svc.status",
		},
		"measureset": {
			metricName: "svc.usage_used",
			labels:     map[string]string{metrix.MeasureSetFieldLabel: "used"},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindGauge,
				SourceKind:  metrix.MetricKindMeasureSet,
				FlattenRole: metrix.FlattenRoleMeasureSetField,
			},
			familyName: "svc.usage",
		},
	}

	for name, test := range structured {
		t.Run(name, func(t *testing.T) {
			_, rules, err := normalizeAutogenPolicy(AutogenPolicy{
				Rules: []AutogenRule{{
					Scope: test.familyName,
					Selector: metrixselector.Expr{
						Deny: []string{test.familyName},
					},
				}},
			})
			require.NoError(t, err)
			e := &Engine{state: engineState{cfg: engineConfig{
				autogen:      AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				autogenRules: rules,
			}}}

			routes, ok, err := e.resolveAutogenRoute(nil, test.metricName, sortedLabelView(test.labels), test.meta)
			require.NoError(t, err)
			assert.False(t, ok)
			assert.Empty(t, routes)

			_, e.state.cfg.autogenRules, err = normalizeAutogenPolicy(AutogenPolicy{
				Rules: []AutogenRule{{
					Scope: "other_*",
					Selector: metrixselector.Expr{
						Deny: []string{"*"},
					},
				}},
			})
			require.NoError(t, err)
			routes, ok, err = e.resolveAutogenRoute(nil, test.metricName, sortedLabelView(test.labels), test.meta)
			require.NoError(t, err)
			assert.True(t, ok)
			assert.NotEmpty(t, routes)
		})
	}
}

func TestResolveAutogenRouteRuleSemantics(t *testing.T) {
	tests := map[string]struct {
		rules   []AutogenRule
		enabled bool
		want    bool
	}{
		"disabled remains disabled": {
			rules: []AutogenRule{{
				Scope:    "*",
				Selector: metrixselector.Expr{Deny: []string{"*"}},
			}},
			want: false,
		},
		"no rules preserve autogen": {
			enabled: true,
			want:    true,
		},
		"deny exact rejects": {
			rules: []AutogenRule{{
				Scope:    "*",
				Selector: metrixselector.Expr{Deny: []string{"svc.requests_total"}},
			}},
			enabled: true,
			want:    false,
		},
		"allow exact selects": {
			rules: []AutogenRule{{
				Scope:    "*",
				Selector: metrixselector.Expr{Allow: []string{"svc.requests_total"}},
			}},
			enabled: true,
			want:    true,
		},
		"allow miss rejects": {
			rules: []AutogenRule{{
				Scope:    "*",
				Selector: metrixselector.Expr{Allow: []string{"other_*"}},
			}},
			enabled: true,
			want:    false,
		},
		"allow and deny uses conjunction": {
			rules: []AutogenRule{{
				Scope: "*",
				Selector: metrixselector.Expr{
					Allow: []string{"svc.*"},
					Deny:  []string{"*_total"},
				},
			}},
			enabled: true,
			want:    false,
		},
		"scope miss is neutral": {
			rules: []AutogenRule{{
				Scope:    "other*",
				Selector: metrixselector.Expr{Deny: []string{"*"}},
			}},
			enabled: true,
			want:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, rules, err := normalizeAutogenPolicy(AutogenPolicy{Rules: test.rules})
			require.NoError(t, err)
			e := &Engine{state: engineState{cfg: engineConfig{
				autogen:      AutogenPolicy{Enabled: test.enabled, MaxTypeIDLen: defaultMaxTypeIDLen},
				autogenRules: rules,
			}}}

			routes, ok, err := e.resolveAutogenRoute(
				nil,
				"svc.requests_total",
				sortedLabelView(nil),
				metrix.SeriesMeta{Kind: metrix.MetricKindCounter},
			)
			require.NoError(t, err)
			assert.Equal(t, test.want, ok)
			assert.Equal(t, test.want, len(routes) > 0)
		})
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
		"histogram bucket excludes le from chart id and uses upper-bound dimension": {
			metricName: "svc.latency_seconds_bucket",
			labels: map[string]string{
				"instance": "db1",
				"le":       "0.5",
				"method":   "GET",
			},
			wantID:  "svc.latency_seconds-instance=db1-method=GET",
			wantDim: "0.5",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			labels := sortedLabelView(tc.labels)
			source, ok := resolveAutogenSource(tc.metricName, labels, metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramBucket,
			})
			require.True(t, ok)
			route, ok, err := buildHistogramBucketAutogenRoute(
				source,
				labels,
				AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				"",
			)
			require.NoError(t, err)
			require.True(t, ok)

			assert.Equal(t, tc.wantID, route.chartID)
			assert.Equal(t, tc.wantDim, route.dimensionName)
			assert.Equal(t, metrix.HistogramBucketLabel, route.dimensionKeyLabel)
			assert.Equal(t, program.AlgorithmIncremental, route.algorithm)
			assert.Equal(t, program.ChartTypeHeatmap, route.chartType)
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
		wantUnits  string
	}{
		"summary quantile excludes quantile label and keeps absolute algorithm": {
			metricName: "svc.request_duration_seconds",
			labels: map[string]string{
				"instance": "db1",
				"method":   "GET",
				"quantile": "0.99",
			},
			wantID:    "svc.request_duration_seconds-instance=db1-method=GET",
			wantDim:   "quantile_0.99",
			wantUnits: "seconds",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			labels := sortedLabelView(tc.labels)
			source, ok := resolveAutogenSource(tc.metricName, labels, metrix.SeriesMeta{
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummaryQuantile,
			})
			require.True(t, ok)
			route, ok, err := buildSummaryQuantileAutogenRoute(
				source,
				labels,
				AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				"",
			)
			require.NoError(t, err)
			require.True(t, ok)

			assert.Equal(t, tc.wantID, route.chartID)
			assert.Equal(t, tc.wantDim, route.dimensionName)
			assert.Equal(t, tc.wantUnits, route.units)
			assert.Equal(t, metrix.SummaryQuantileLabel, route.dimensionKeyLabel)
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
			tc.meta.SourceKind = metrix.MetricKindStateSet
			labels := sortedLabelView(tc.labels)
			source, ok := resolveAutogenSource(tc.metricName, labels, tc.meta)
			require.True(t, ok)
			route, ok, err := buildStateSetAutogenRoute(
				source,
				labels,
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

func runTestBuildMeasureSetAutogenRoute(t *testing.T) {
	tests := map[string]struct {
		metricName string
		labels     map[string]string
		meta       metrix.SeriesMeta
		wantOK     bool
		wantID     string
		wantDim    string
		wantAlg    program.Algorithm
		wantUnits  string
	}{
		"MeasureSet gauge uses synthetic field label and absolute algorithm": {
			metricName: "service_latency_seconds_value",
			labels: map[string]string{
				"instance":                  "db1",
				metrix.MeasureSetFieldLabel: "value",
			},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindGauge,
				SourceKind:  metrix.MetricKindMeasureSet,
				FlattenRole: metrix.FlattenRoleMeasureSetField,
			},
			wantOK:    true,
			wantID:    "service_latency_seconds-instance=db1",
			wantDim:   "value",
			wantAlg:   program.AlgorithmAbsolute,
			wantUnits: "seconds",
		},
		"MeasureSet counter uses synthetic field label and incremental algorithm": {
			metricName: "svc_requests_total_ok",
			labels: map[string]string{
				"instance":                  "db1",
				metrix.MeasureSetFieldLabel: "ok",
			},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindMeasureSet,
				FlattenRole: metrix.FlattenRoleMeasureSetField,
			},
			wantOK:    true,
			wantID:    "svc_requests_total-instance=db1",
			wantDim:   "ok",
			wantAlg:   program.AlgorithmIncremental,
			wantUnits: "requests/s",
		},
		"MeasureSet ignores unrelated matching labels and uses reserved field label": {
			metricName: "svc_requests_total_ok",
			labels: map[string]string{
				"instance":                  "db1",
				"svc_requests":              "total_ok",
				metrix.MeasureSetFieldLabel: "ok",
			},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindMeasureSet,
				FlattenRole: metrix.FlattenRoleMeasureSetField,
			},
			wantOK:    true,
			wantID:    "svc_requests_total-instance=db1-svc_requests=total_ok",
			wantDim:   "ok",
			wantAlg:   program.AlgorithmIncremental,
			wantUnits: "requests/s",
		},
		"MeasureSet without reserved field label does not route": {
			metricName: "svc_requests_total_ok",
			labels: map[string]string{
				"instance":     "db1",
				"svc_requests": "total_ok",
			},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindMeasureSet,
				FlattenRole: metrix.FlattenRoleMeasureSetField,
			},
			wantOK: false,
		},
		"MeasureSet with mismatched reserved field label does not route": {
			metricName: "svc_requests_total_ok",
			labels: map[string]string{
				"instance":                  "db1",
				metrix.MeasureSetFieldLabel: "failed",
			},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindMeasureSet,
				FlattenRole: metrix.FlattenRoleMeasureSetField,
			},
			wantOK: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			labels := sortedLabelView(tc.labels)
			source, sourceOK := resolveAutogenSource(tc.metricName, labels, tc.meta)
			require.Equal(t, tc.wantOK, sourceOK)
			if !sourceOK {
				return
			}
			route, ok, err := buildMeasureSetAutogenRoute(
				source,
				labels,
				tc.meta,
				AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				"",
			)
			require.NoError(t, err)
			require.Equal(t, tc.wantOK, ok)
			if !tc.wantOK {
				return
			}

			assert.Equal(t, tc.wantID, route.chartID)
			assert.Equal(t, tc.wantDim, route.dimensionName)
			assert.Equal(t, tc.wantAlg, route.algorithm)
			assert.Equal(t, tc.wantUnits, route.units)
			assert.Equal(t, metrix.MeasureSetFieldLabel, route.dimensionKeyLabel)
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

func TestGetAutogenChartContext(t *testing.T) {
	tests := map[string]struct {
		namespace string
		metric    string
		want      string
	}{
		"empty namespace returns bare metric":        {namespace: "", metric: "foo", want: "foo"},
		"single-segment namespace is prefixed":       {namespace: "prometheus", metric: "foo", want: "prometheus.foo"},
		"multi-segment namespace composes with dot":  {namespace: "prometheus.app", metric: "foo", want: "prometheus.app.foo"},
		"whitespace-only namespace is treated empty": {namespace: "  ", metric: "foo", want: "foo"},
		"empty metric falls back to metric":          {namespace: "", metric: "", want: "metric"},
		"namespace with empty metric":                {namespace: "prometheus", metric: "", want: "prometheus.metric"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, getAutogenChartContext(tc.namespace, tc.metric))
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

func TestBuildScalarAutogenRouteSanitizesDotLabelValues(t *testing.T) {
	route, ok, err := buildScalarAutogenRoute(
		"svc.requests_total",
		sortedLabelView(map[string]string{
			"instance": "db1.eu",
			"job":      "mysql.prod",
		}),
		metrix.SeriesMeta{Kind: metrix.MetricKindCounter},
		AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
		"",
	)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "svc.requests_total-instance=db1_eu-job=mysql_prod", route.chartID)
}

func TestBuildScalarAutogenRouteSanitizesLegacyLabelChars(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "space", value: "a b", want: "a_b"},
		{name: "backslash", value: "a\\b", want: "a_b"},
		{name: "apostrophe", value: "a'b", want: "ab"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			route, ok, err := buildScalarAutogenRoute(
				"svc.requests_total",
				sortedLabelView(map[string]string{"instance": tc.value}),
				metrix.SeriesMeta{Kind: metrix.MetricKindCounter},
				AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
				"",
			)
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, "svc.requests_total-instance="+tc.want, route.chartID)
		})
	}
}
