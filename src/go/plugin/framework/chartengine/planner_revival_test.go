// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/stretchr/testify/assert"
)

func TestChartDefinitionComparison(t *testing.T) {
	// Context/type are fixed within an authored entry, but remain emitted settings
	// when comparing definitions. The chart's algorithm policy is not emitted.
	original := program.ChartMeta{
		Title:    "Title",
		Units:    "items",
		Family:   "Family",
		Context:  "context",
		Type:     program.ChartTypeLine,
		Priority: 100,
	}
	for name, tc := range map[string]struct {
		change  func(*program.ChartMeta)
		changed bool
	}{
		"unchanged":             {change: func(*program.ChartMeta) {}},
		"title":                 {change: func(m *program.ChartMeta) { m.Title = "Other" }, changed: true},
		"units":                 {change: func(m *program.ChartMeta) { m.Units = "bytes" }, changed: true},
		"family":                {change: func(m *program.ChartMeta) { m.Family = "Other" }, changed: true},
		"context":               {change: func(m *program.ChartMeta) { m.Context = "other" }, changed: true},
		"type":                  {change: func(m *program.ChartMeta) { m.Type = program.ChartTypeArea }, changed: true},
		"priority":              {change: func(m *program.ChartMeta) { m.Priority = 200 }, changed: true},
		"algorithm policy only": {change: func(m *program.ChartMeta) { m.Algorithm = program.AlgorithmAbsolute }},
	} {
		t.Run(name, func(t *testing.T) {
			next := original
			tc.change(&next)
			assert.Equal(t, tc.changed, chartDefinitionChanged(original, next))
		})
	}
}

func TestDimensionDefinitionComparisonUsesWireSettings(t *testing.T) {
	previous := &materializedDimensionState{
		algorithm:  program.AlgorithmAbsolute,
		multiplier: 1,
		divisor:    1,
		hidden:     true,
		float:      true,
	}
	for name, tc := range map[string]struct {
		change  func(*dimensionState)
		changed bool
	}{
		"unchanged":            {change: func(*dimensionState) {}},
		"zero scale means one": {change: func(d *dimensionState) { d.multiplier = 0; d.divisor = 0 }},
		"ordering only":        {change: func(d *dimensionState) { d.static = true; d.order = 5; d.sortKey.kind = dimensionSortHistogramBucket }},
		"aggregation only":     {change: func(d *dimensionState) { d.aggregation = program.AggregationMax }},
		"unhide":               {change: func(d *dimensionState) { d.hidden = false }, changed: true},
		"float to int":         {change: func(d *dimensionState) { d.float = false }, changed: true},
		"algorithm":            {change: func(d *dimensionState) { d.algorithm = dimensionAlgorithmIncremental }, changed: true},
		"multiplier":           {change: func(d *dimensionState) { d.multiplier = -1 }, changed: true},
		"divisor":              {change: func(d *dimensionState) { d.divisor = 1000 }, changed: true},
	} {
		t.Run(name, func(t *testing.T) {
			next := dimensionState{
				algorithm:  dimensionAlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
				hidden:     true,
				float:      true,
			}
			tc.change(&next)
			assert.Equal(t, tc.changed, dimensionDefinitionChanged(previous, next))
		})
	}
}
