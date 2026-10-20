// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"maps"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures are built fresh per call so a test case can mutate its copy.

var testStatesetStates = map[string][]string{"worker_state": {"idle", "busy"}}

func testCharts() map[string]charttpl.Chart {
	return map[string]charttpl.Chart{
		"app.requests": {
			Title: "Requests", Context: "requests", Units: "requests/s", Type: "line",
			Dimensions: []charttpl.Dimension{
				{Selector: `requests_total{code="200"}`, Name: "ok"},
				{Selector: `requests_total{code="500"}`, Name: "failed"},
			},
		},
		"app.latency": {
			Title: "Latency", Context: "latency", Units: "seconds", Type: "line",
			Dimensions: []charttpl.Dimension{{Selector: "latency_bucket"}},
		},
		"app.worker.state": {
			Title: "Worker State", Context: "state", Units: "state", Type: "stacked",
			Dimensions: []charttpl.Dimension{{Selector: "worker_state"}},
		},
	}
}

func testMetrics() []MetadataMetric {
	return []MetadataMetric{
		{Scope: "global", Name: "app.requests", Description: "Requests", Unit: "requests/s", ChartType: "line", Dimensions: []string{"ok", "failed"}},
		{Scope: "global", Name: "app.latency", Description: "Latency", Unit: "seconds", ChartType: "line", Dimensions: []string{"a dimension per bucket"}},
		{Scope: "worker", Name: "app.worker.state", Description: "Worker State", Unit: "state", ChartType: "stacked", Dimensions: []string{"idle", "busy"}},
	}
}

func testHealthAlerts() []HealthAlert {
	return []HealthAlert{
		{Name: "app_requests_failed", On: "app.requests", Info: "failed requests", Units: "requests/s", Lookup: "max -1m unaligned of failed"},
	}
}

func testMetadataAlerts() []MetadataAlert {
	return []MetadataAlert{{Name: "app_requests_failed", Metric: "app.requests", Info: "failed requests"}}
}

func TestChartTemplateCharts(t *testing.T) {
	const template = `
version: v1
context_namespace: app
groups:
  - family: Root
    metrics: [requests_total, worker_state]
    groups:
      - family: Workers
        context_namespace: worker
        charts:
          - title: Worker State
            context: state
            units: state
            type: stacked
            dimensions:
              - selector: worker_state
    charts:
      - title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: requests_total
            name: total
`
	charts, err := ChartTemplateCharts(template)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"app.requests", "app.worker.state"}, slices.Collect(maps.Keys(charts)), "contexts compose spec and group namespaces")
	assert.Equal(t, "line", charts["app.requests"].Type, "decode applies the type default")
	assert.Equal(t, "stacked", charts["app.worker.state"].Type)

	_, err = ChartTemplateCharts(template + "      - title: Requests Again\n        context: requests\n        units: requests/s\n        dimensions:\n          - selector: requests_total\n            name: total\n")
	require.ErrorContains(t, err, `duplicate chart context "app.requests"`)
}

func TestDecodeMetadataModule(t *testing.T) {
	const single = `
modules:
  - meta:
      id: collector-app
    alerts:
      - name: app_requests_failed
        metric: app.requests
        info: "failed requests"
    metrics:
      scopes:
        - name: global
          metrics:
            - name: app.requests
              description: Requests
              unit: requests/s
              chart_type: line
              dimensions:
                - name: ok
                - name: failed
`
	const other = `
  - meta:
      id: collector-other
    alerts:
      - name: other_alert
        metric: other.chart
        info: other
`
	app := MetadataModule{
		ID:     "collector-app",
		Alerts: []MetadataAlert{{Name: "app_requests_failed", Metric: "app.requests", Info: "failed requests"}},
		Metrics: []MetadataMetric{
			{Scope: "global", Name: "app.requests", Description: "Requests", Unit: "requests/s", ChartType: "line", Dimensions: []string{"ok", "failed"}},
		},
	}
	tests := map[string]struct {
		metadata string
		moduleID string
		want     MetadataModule
		wantErr  string
	}{
		"single module":            {metadata: single, want: app},
		"single module by id":      {metadata: single, moduleID: "collector-app", want: app},
		"selected among several":   {metadata: single + other, moduleID: "collector-app", want: app},
		"other selected":           {metadata: single + other, moduleID: "collector-other", want: MetadataModule{ID: "collector-other", Alerts: []MetadataAlert{{Name: "other_alert", Metric: "other.chart", Info: "other"}}}},
		"several without selector": {metadata: single + other, wantErr: "expected exactly one module, got 2"},
		"unknown id":               {metadata: single + other, moduleID: "ghost", wantErr: `no module with meta.id "ghost"`},
		"duplicate id":             {metadata: single + other + other, moduleID: "collector-other", wantErr: `several modules with meta.id "collector-other"`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeMetadataModule([]byte(test.metadata), test.moduleID)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestParseHealthAlerts(t *testing.T) {
	const health = `
# comment lines and unknown keys are ignored
 template: app_requests_failed
       on: app.requests
    class: Errors
   lookup: max -1m unaligned of failed
    units: requests/s
     warn: $this > 0
     info: failed requests
       to: sysadmin

    alarm: app_latency_high
       on: app.latency
     calc: $p99
    units: seconds
     info: slow requests
`
	assert.Equal(t, []HealthAlert{
		{Name: "app_requests_failed", On: "app.requests", Info: "failed requests", Units: "requests/s", Lookup: "max -1m unaligned of failed"},
		{Name: "app_latency_high", On: "app.latency", Info: "slow requests", Units: "seconds"},
	}, ParseHealthAlerts([]byte(health)))
}

func TestChartDimensionNames(t *testing.T) {
	tests := map[string]struct {
		chart   charttpl.Chart
		want    []string
		wantOK  bool
		wantErr string
	}{
		"static names":         {chart: testCharts()["app.requests"], want: []string{"ok", "failed"}, wantOK: true},
		"stateset states":      {chart: testCharts()["app.worker.state"], want: []string{"idle", "busy"}, wantOK: true},
		"histogram buckets":    {chart: testCharts()["app.latency"]},
		"summary quantiles":    {chart: charttpl.Chart{Dimensions: []charttpl.Dimension{{Selector: `latency{quantile="0.99"}`}}}},
		"name from label":      {chart: charttpl.Chart{Dimensions: []charttpl.Dimension{{Selector: "pool_size", NameFromLabel: "pool"}}}},
		"undeclared stateset":  {chart: charttpl.Chart{Dimensions: []charttpl.Dimension{{Selector: "pool_state"}}}, wantErr: `selector "pool_state" has no dimension name and metric "pool_state" has no declared states`},
		"static before states": {chart: charttpl.Chart{Dimensions: []charttpl.Dimension{{Selector: "worker_total", Name: "total"}, {Selector: "worker_state"}}}, want: []string{"total", "idle", "busy"}, wantOK: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok, err := ChartDimensionNames(test.chart, testStatesetStates)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantOK, ok)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestCheckMetadataMetricsMatchCharts(t *testing.T) {
	tests := map[string]struct {
		mutate  func(metrics *[]MetadataMetric, charts map[string]charttpl.Chart)
		wantErr []string
	}{
		"aligned": {},
		"undocumented chart": {
			mutate:  func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) { *metrics = (*metrics)[:2] },
			wantErr: []string{"app.worker.state: in the chart template but not documented"},
		},
		"unknown metric": {
			mutate: func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) {
				*metrics = append(*metrics, MetadataMetric{Name: "app.ghost"})
			},
			wantErr: []string{"app.ghost: documented but not in the chart template"},
		},
		"documented twice": {
			mutate: func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) {
				*metrics = append(*metrics, (*metrics)[0])
			},
			wantErr: []string{"app.requests: documented twice"},
		},
		"title drift": {
			mutate: func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) {
				(*metrics)[0].Description = "Request rate"
			},
			wantErr: []string{`app.requests: description "Request rate", chart title "Requests"`},
		},
		"unit drift": {
			mutate:  func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) { (*metrics)[1].Unit = "ms" },
			wantErr: []string{`app.latency: unit "ms", chart units "seconds"`},
		},
		"type drift": {
			mutate:  func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) { (*metrics)[2].ChartType = "line" },
			wantErr: []string{`app.worker.state: chart_type "line", chart type "stacked"`},
		},
		"dimension drift": {
			mutate: func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) {
				(*metrics)[2].Dimensions = []string{"idle", "running"}
			},
			wantErr: []string{"app.worker.state: dimensions [idle running], chart renders [idle busy]"},
		},
		"runtime-named dimensions are not compared": {
			mutate: func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) { (*metrics)[1].Dimensions = nil },
		},
		"unresolvable stateset": {
			mutate: func(metrics *[]MetadataMetric, charts map[string]charttpl.Chart) {
				charts["app.pool"] = charttpl.Chart{Title: "Pool State", Units: "state", Type: "line", Dimensions: []charttpl.Dimension{{Selector: "pool_state"}}}
				*metrics = append(*metrics, MetadataMetric{Name: "app.pool", Description: "Pool State", Unit: "state", ChartType: "line", Dimensions: []string{"active"}})
			},
			wantErr: []string{`app.pool: selector "pool_state" has no dimension name and metric "pool_state" has no declared states`},
		},
		"several problems are reported together": {
			mutate: func(metrics *[]MetadataMetric, _ map[string]charttpl.Chart) {
				(*metrics)[0].Description = "Request rate"
				(*metrics)[1].Unit = "ms"
			},
			wantErr: []string{`app.requests: description "Request rate", chart title "Requests"`, `app.latency: unit "ms", chart units "seconds"`},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			metrics, charts := testMetrics(), testCharts()
			if test.mutate != nil {
				test.mutate(&metrics, charts)
			}
			err := CheckMetadataMetricsMatchCharts(metrics, charts, testStatesetStates)
			if len(test.wantErr) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range test.wantErr {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestCheckHealthAlertsTargetCharts(t *testing.T) {
	foreign := HealthAlert{Name: "other_alert", On: "other.chart"}
	ghost := HealthAlert{Name: "app_ghost", On: "app.ghost"}
	tests := map[string]struct {
		alerts  []HealthAlert
		opts    HealthAlertsCheck
		wantErr string
	}{
		"aligned":                    {alerts: testHealthAlerts()},
		"unknown context":            {alerts: append(testHealthAlerts(), ghost), wantErr: `app_ghost: on "app.ghost" is not a chart template context`},
		"shared file without prefix": {alerts: append(testHealthAlerts(), foreign), wantErr: `other_alert: on "other.chart" is not a chart template context`},
		"shared file with prefix":    {alerts: append(testHealthAlerts(), foreign), opts: HealthAlertsCheck{ContextPrefix: "app."}},
		"prefix keeps checking own":  {alerts: append(testHealthAlerts(), foreign, ghost), opts: HealthAlertsCheck{ContextPrefix: "app."}, wantErr: "app_ghost"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckHealthAlertsTargetCharts(test.alerts, testCharts(), test.opts)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestCheckMetadataAlertsMatchHealthAlerts(t *testing.T) {
	other := HealthAlert{Name: "app_other", On: "app.latency", Info: "other"}
	tests := map[string]struct {
		documented []MetadataAlert
		shipped    []HealthAlert
		shared     bool
		wantErr    string
	}{
		"aligned":            {documented: testMetadataAlerts(), shipped: testHealthAlerts()},
		"undocumented alert": {documented: testMetadataAlerts(), shipped: append(testHealthAlerts(), other), wantErr: "app_other: in the health configuration but not documented"},
		"shared health file": {documented: testMetadataAlerts(), shipped: append(testHealthAlerts(), other), shared: true},
		"unknown alert": {
			documented: []MetadataAlert{{Name: "app_requests_dropped", Metric: "app.requests", Info: "failed requests"}},
			shipped:    testHealthAlerts(),
			wantErr:    "app_requests_dropped: documented but not in the health configuration",
		},
		"documented twice": {
			documented: append(testMetadataAlerts(), testMetadataAlerts()...),
			shipped:    testHealthAlerts(),
			wantErr:    "app_requests_failed: documented twice",
		},
		"metric drift": {
			documented: []MetadataAlert{{Name: "app_requests_failed", Metric: "app.latency", Info: "failed requests"}},
			shipped:    testHealthAlerts(),
			wantErr:    `app_requests_failed: metric "app.latency", alert on "app.requests"`,
		},
		"info drift": {
			documented: []MetadataAlert{{Name: "app_requests_failed", Metric: "app.requests", Info: "requests failing"}},
			shipped:    testHealthAlerts(),
			wantErr:    `app_requests_failed: info "requests failing", alert info "failed requests"`,
		},
		"identical variants": {documented: testMetadataAlerts(), shipped: append(testHealthAlerts(), testHealthAlerts()...)},
		"conflicting variant": {
			documented: testMetadataAlerts(),
			shipped:    append(testHealthAlerts(), HealthAlert{Name: "app_requests_failed", On: "app.latency", Info: "failed requests"}),
			wantErr:    `app_requests_failed: metric "app.requests", alert on "app.latency"`,
		},
		"shared file still checks documented alerts": {
			documented: []MetadataAlert{{Name: "app_requests_failed", Metric: "app.requests", Info: "drifted"}},
			shipped:    append(testHealthAlerts(), other),
			shared:     true,
			wantErr:    `app_requests_failed: info "drifted"`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckMetadataAlertsMatchHealthAlerts(test.documented, test.shipped, test.shared)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestCheckHealthAlertsMatchMetadataMetrics(t *testing.T) {
	alert := func(mutate func(*HealthAlert)) []HealthAlert {
		alerts := testHealthAlerts()
		mutate(&alerts[0])
		return alerts
	}
	tests := map[string]struct {
		alerts  []HealthAlert
		opts    HealthAlertsCheck
		wantErr string
	}{
		"aligned":             {alerts: testHealthAlerts()},
		"undocumented metric": {alerts: alert(func(a *HealthAlert) { a.On = "app.ghost" }), wantErr: `app_requests_failed: on "app.ghost" is not a documented metric`},
		"unit drift":          {alerts: alert(func(a *HealthAlert) { a.Units = "requests" }), wantErr: `app_requests_failed: units "requests", documented unit "requests/s"`},
		"missing units":       {alerts: alert(func(a *HealthAlert) { a.Units = "" }), wantErr: `app_requests_failed: units "", documented unit "requests/s"`},
		"undocumented lookup dimension": {
			alerts:  alert(func(a *HealthAlert) { a.Lookup = "max -1m unaligned of dropped" }),
			wantErr: `app_requests_failed: lookup dimension "dropped" is not documented for app.requests`,
		},
		"comma separated dimensions": {
			alerts:  alert(func(a *HealthAlert) { a.Lookup = "sum -5m unaligned of ok,failed,dropped" }),
			wantErr: `lookup dimension "dropped"`,
		},
		"pipe separated dimensions":  {alerts: alert(func(a *HealthAlert) { a.Lookup = "sum -5m unaligned of ok|failed" })},
		"pattern dimensions skipped": {alerts: alert(func(a *HealthAlert) { a.Lookup = "average -5m anomaly-bit of *" })},
		"negated pattern skipped":    {alerts: alert(func(a *HealthAlert) { a.Lookup = "sum -5m of !ok *" })},
		"all keyword skipped":        {alerts: alert(func(a *HealthAlert) { a.Lookup = "sum -5m unaligned of all" })},
		"foreach clause ignored":     {alerts: alert(func(a *HealthAlert) { a.Lookup = "average -5m anomaly-bit of failed foreach *" })},
		"foreach without of":         {alerts: alert(func(a *HealthAlert) { a.Lookup = "average -5m anomaly-bit foreach *" })},
		"lookup without dimensions":  {alerts: alert(func(a *HealthAlert) { a.Lookup = "min -5m unaligned" })},
		"calc without lookup":        {alerts: alert(func(a *HealthAlert) { a.Lookup = "" })},
		"shared file with prefix": {
			alerts: append(testHealthAlerts(), HealthAlert{Name: "other_alert", On: "other.chart", Units: "x"}),
			opts:   HealthAlertsCheck{ContextPrefix: "app."},
		},
		"shared file without prefix": {
			alerts:  append(testHealthAlerts(), HealthAlert{Name: "other_alert", On: "other.chart", Units: "x"}),
			wantErr: `other_alert: on "other.chart" is not a documented metric`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckHealthAlertsMatchMetadataMetrics(test.alerts, testMetrics(), test.opts)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}

	t.Run("duplicate metric is reported, not chosen", func(t *testing.T) {
		metrics := append(testMetrics(), MetadataMetric{Name: "app.requests", Unit: "requests", Dimensions: []string{"dropped"}})
		err := CheckHealthAlertsMatchMetadataMetrics(testHealthAlerts(), metrics, HealthAlertsCheck{})
		require.ErrorContains(t, err, "app.requests: documented twice")
		assert.NotContains(t, err.Error(), "documented unit", "the first declaration stays the validation target")
	})
}
