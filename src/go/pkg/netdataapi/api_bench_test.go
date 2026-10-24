// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"bytes"
	"testing"
)

// BenchmarkAPIValueUpdate measures one chart update with integer, float and empty
// values, the per-cycle output of every chart. ns/op is a development-machine
// trend; allocations are the gate.
func BenchmarkAPIValueUpdate(b *testing.B) {
	var buf bytes.Buffer
	api := New(&buf)
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		api.BEGIN("plugin.job", "chart_with_a_typical_identifier", 1000)
		api.SET("requests", 123456789)
		api.SET("errors", 12)
		api.SETFLOAT("latency", 0.123456)
		api.SETEMPTY("missing")
		api.END()
	}
}

// BenchmarkAPIChartDefinition measures a chart definition with two dimensions and
// labels, emitted when a chart is created or its labels change.
func BenchmarkAPIChartDefinition(b *testing.B) {
	var buf bytes.Buffer
	api := New(&buf)
	chart := ChartOpts{
		TypeID:      "plugin.job",
		ID:          "chart_with_a_typical_identifier",
		Title:       "Requests",
		Units:       "requests/s",
		Family:      "traffic",
		Context:     "app.requests",
		ChartType:   "line",
		Priority:    70000,
		UpdateEvery: 1,
		Plugin:      "go.d",
		Module:      "app",
	}
	dims := []DimensionOpts{
		{ID: "success", Name: "success", Algorithm: "incremental", Multiplier: 1, Divisor: 1},
		{ID: "failed", Name: "failed", Algorithm: "incremental", Multiplier: -1, Divisor: 1000},
	}
	b.ReportAllocs()
	for b.Loop() {
		buf.Reset()
		api.CHART(chart)
		for _, dim := range dims {
			api.DIMENSION(dim)
		}
		api.CLABEL("instance", "node-1", 1)
		api.CLABELCOMMIT()
		api.HOST("11111111-1111-1111-1111-111111111111")
	}
}
