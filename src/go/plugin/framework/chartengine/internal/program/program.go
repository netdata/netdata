// SPDX-License-Identifier: GPL-3.0-or-later

package program

import (
	"fmt"
	"slices"
)

// Program is an immutable compiled chart-template snapshot consumed by chartengine.
//
// Immutability contract:
//   - New(...) deep-copies all caller-provided slices/maps.
//   - getters return copies, never internal backing storage.
//   - all internal indexes are built once at construction time.
//
// This keeps runtime planning deterministic and race-free while templates are
// recompiled/reloaded in parallel runtime paths.
type Program struct {
	version  string
	revision uint64

	metrics      map[string]struct{}
	metricNames  []string
	charts       []Chart
	chartByTmpl  map[string]int
	chartOrderID []string
}

// New constructs an immutable Program snapshot from precompiled IR items.
func New(version string, revision uint64, metrics []string, charts []Chart) (*Program, error) {
	if version == "" {
		return nil, fmt.Errorf("program: version is required")
	}

	p := &Program{
		version:     version,
		revision:    revision,
		metrics:     make(map[string]struct{}, len(metrics)),
		charts:      make([]Chart, 0, len(charts)),
		chartByTmpl: make(map[string]int, len(charts)),
	}

	for _, name := range metrics {
		if name == "" {
			return nil, fmt.Errorf("program: metric name cannot be empty")
		}
		if _, ok := p.metrics[name]; ok {
			continue
		}
		p.metrics[name] = struct{}{}
		p.metricNames = append(p.metricNames, name)
	}
	slices.Sort(p.metricNames)

	for i, chart := range charts {
		if err := validateChart(chart); err != nil {
			return nil, fmt.Errorf("program: chart[%d]: %w", i, err)
		}
		if _, exists := p.chartByTmpl[chart.TemplateID]; exists {
			return nil, fmt.Errorf("program: duplicate chart template_id %q", chart.TemplateID)
		}

		clone := chart.clone()
		p.chartByTmpl[clone.TemplateID] = len(p.charts)
		p.chartOrderID = append(p.chartOrderID, clone.TemplateID)
		p.charts = append(p.charts, clone)
	}

	return p, nil
}

// Version returns the program schema/version marker.
func (p *Program) Version() string { return p.version }

// Revision returns a deterministic revision/hash value assigned by compiler.
func (p *Program) Revision() uint64 { return p.revision }

// MetricNames returns declared metric names in sorted deterministic order.
func (p *Program) MetricNames() []string {
	return append([]string(nil), p.metricNames...)
}

// HasMetric reports whether a metric name is declared in program scope.
func (p *Program) HasMetric(name string) bool {
	_, ok := p.metrics[name]
	return ok
}

// ChartTemplateIDs returns chart template IDs in insertion order.
func (p *Program) ChartTemplateIDs() []string {
	return append([]string(nil), p.chartOrderID...)
}

// Charts returns a deep copy of compiled charts in insertion order.
func (p *Program) Charts() []Chart {
	out := make([]Chart, 0, len(p.charts))
	for _, chart := range p.charts {
		out = append(out, chart.clone())
	}
	return out
}

// Chart returns one chart by template ID.
func (p *Program) Chart(templateID string) (Chart, bool) {
	idx, ok := p.chartByTmpl[templateID]
	if !ok {
		return Chart{}, false
	}
	return p.charts[idx].clone(), true
}
