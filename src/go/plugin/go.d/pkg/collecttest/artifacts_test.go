// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const artifactsTemplate = `
version: v1
groups:
  - family: Root
    context_namespace: app
    metrics: [requests_total, worker_state, latency_bucket]
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
          - selector: requests_total{code="200"}
            name: ok
          - selector: requests_total{code="500"}
            name: failed
      - title: Latency
        context: latency
        units: seconds
        dimensions:
          - selector: latency_bucket
`

var artifactsStates = map[string][]string{"worker_state": {"idle", "busy"}}

const artifactsMetadata = `
modules:
  - alerts:
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
            - name: app.latency
              description: Latency
              unit: seconds
              chart_type: line
              dimensions:
                - name: a dimension per bucket
        - name: worker
          metrics:
            - name: app.worker.state
              description: Worker State
              unit: state
              chart_type: stacked
              dimensions:
                - name: idle
                - name: busy
`

const artifactsHealth = `
 template: app_requests_failed
       on: app.requests
     calc: $failed
     warn: $this > 0
     info: failed requests
`

func TestChartTemplateChartsComposesContexts(t *testing.T) {
	charts, err := ChartTemplateCharts(artifactsTemplate)
	require.NoError(t, err)
	var contexts []string
	for context := range charts {
		contexts = append(contexts, context)
	}
	assert.ElementsMatch(t, []string{"app.requests", "app.latency", "app.worker.state"}, contexts)
	assert.Equal(t, "line", string(charts["app.requests"].Type), "decode applies the type default")
}

func TestChartDimensionNames(t *testing.T) {
	charts, err := ChartTemplateCharts(artifactsTemplate)
	require.NoError(t, err)
	names, ok, err := ChartDimensionNames(charts["app.requests"], artifactsStates)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []string{"ok", "failed"}, names)
	names, ok, err = ChartDimensionNames(charts["app.worker.state"], artifactsStates)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []string{"idle", "busy"}, names, "stateset dimensions follow the declared states")
	_, ok, err = ChartDimensionNames(charts["app.latency"], artifactsStates)
	require.NoError(t, err)
	assert.False(t, ok, "histogram bucket names are only known at runtime")
}

func TestCheckMetadataDocumentsChartTemplate(t *testing.T) {
	tests := map[string]struct {
		metadata string
		template string
		wantErr  []string
	}{
		"aligned":               {metadata: artifactsMetadata},
		"undocumented chart":    {metadata: removeBlock(t, artifactsMetadata, "- name: app.latency"), wantErr: []string{"app.latency: in the chart template but not documented"}},
		"unresolvable stateset": {template: replaceOnce(t, artifactsTemplate, "metrics: [requests_total, worker_state, latency_bucket]", "metrics: [requests_total, worker_state, latency_bucket, pool_state]") + "      - title: Pool State\n        context: pool\n        units: state\n        dimensions:\n          - selector: pool_state\n", metadata: artifactsMetadata + "            - name: app.pool\n              description: Pool State\n              unit: state\n              chart_type: line\n              dimensions:\n                - name: active\n", wantErr: []string{`app.pool: selector "pool_state" has no dimension name and metric "pool_state" has no declared states`}},
		"unknown metric":        {metadata: artifactsMetadata + "            - name: app.ghost\n              description: Ghost\n", wantErr: []string{"app.ghost: documented but not in the chart template"}},
		"title drift":           {metadata: replaceOnce(t, artifactsMetadata, "description: Requests", "description: Request rate"), wantErr: []string{`app.requests: description "Request rate", chart title "Requests"`}},
		"unit drift":            {metadata: replaceOnce(t, artifactsMetadata, "unit: seconds", "unit: ms"), wantErr: []string{`app.latency: unit "ms", chart units "seconds"`}},
		"type drift":            {metadata: replaceOnce(t, artifactsMetadata, "chart_type: stacked", "chart_type: line"), wantErr: []string{`app.worker.state: chart_type "line", chart type "stacked"`}},
		"dimension drift":       {metadata: replaceOnce(t, artifactsMetadata, "- name: busy", "- name: running"), wantErr: []string{"app.worker.state: dimensions [idle running], chart renders [idle busy]"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			template := artifactsTemplate
			if test.template != "" {
				template = test.template
			}
			err := CheckMetadataDocumentsChartTemplate([]byte(test.metadata), template, artifactsStates)
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

func TestCheckHealthAlertsTargetChartTemplate(t *testing.T) {
	require.NoError(t, CheckHealthAlertsTargetChartTemplate([]byte(artifactsHealth), artifactsTemplate))
	err := CheckHealthAlertsTargetChartTemplate([]byte(artifactsHealth+"\n alarm: app_ghost\n    on: app.ghost\n"), artifactsTemplate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `app_ghost: on "app.ghost" is not a chart template context`)
}

func TestCheckMetadataAlertsMatchHealthConfig(t *testing.T) {
	tests := map[string]struct {
		metadata string
		health   string
		wantErr  string
	}{
		"aligned":             {metadata: artifactsMetadata, health: artifactsHealth},
		"undocumented alert":  {metadata: artifactsMetadata, health: artifactsHealth + "\n template: app_other\n       on: app.latency\n     info: other\n", wantErr: "app_other: in the health configuration but not documented"},
		"unknown alert":       {metadata: replaceOnce(t, artifactsMetadata, "name: app_requests_failed", "name: app_requests_dropped"), health: artifactsHealth, wantErr: "app_requests_dropped: documented but not in the health configuration"},
		"metric drift":        {metadata: replaceOnce(t, artifactsMetadata, "metric: app.requests", "metric: app.latency"), health: artifactsHealth, wantErr: `app_requests_failed: metric "app.latency", alert on "app.requests"`},
		"info drift":          {metadata: replaceOnce(t, artifactsMetadata, `info: "failed requests"`, `info: "requests failing"`), health: artifactsHealth, wantErr: `app_requests_failed: info "requests failing", alert info "failed requests"`},
		"identical variants":  {metadata: artifactsMetadata, health: artifactsHealth + artifactsHealth},
		"conflicting variant": {metadata: artifactsMetadata, health: artifactsHealth + replaceOnce(t, artifactsHealth, "on: app.requests", "on: app.latency"), wantErr: `app_requests_failed: metric "app.requests", alert on "app.latency"`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckMetadataAlertsMatchHealthConfig([]byte(test.metadata), []byte(test.health))
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func replaceOnce(t *testing.T, s, old, new string) string {
	t.Helper()
	require.Contains(t, s, old)
	return strings.Replace(s, old, new, 1)
}

// removeBlock drops the YAML list item whose first line contains marker: that
// line and every following line indented deeper than it.
func removeBlock(t *testing.T, s, marker string) string {
	t.Helper()
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if !strings.Contains(line, marker) {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		end := i + 1
		for end < len(lines) && (strings.TrimSpace(lines[end]) == "" || len(lines[end])-len(strings.TrimLeft(lines[end], " ")) > indent) {
			end++
		}
		return strings.Join(append(lines[:i:i], lines[end:]...), "\n")
	}
	t.Fatalf("removeBlock: %q not found", marker)
	return ""
}
