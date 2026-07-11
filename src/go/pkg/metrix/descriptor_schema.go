// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"math"
	"strings"
)

func buildHistogramSchema(cfg instrumentConfig, mode metricMode) (*histogramSchema, error) {
	bounds, err := normalizeHistogramBounds(cfg.histogramBounds)
	if err != nil {
		return nil, err
	}
	if mode == modeStateful && len(bounds) == 0 {
		return nil, fmt.Errorf("%w for stateful histogram", errHistogramBounds)
	}
	if len(bounds) == 0 {
		return nil, nil
	}
	return &histogramSchema{bounds: bounds}, nil
}

func buildSummarySchema(cfg instrumentConfig) (*summarySchema, error) {
	if cfg.summaryReservoirSet && cfg.summaryReservoir <= 0 {
		return nil, fmt.Errorf("metrix: summary reservoir size must be > 0")
	}

	qs, err := normalizeSummaryQuantiles(cfg.summaryQuantile)
	if err != nil {
		return nil, err
	}

	if len(qs) == 0 {
		return nil, nil
	}

	size := defaultSummaryReservoirSize
	if cfg.summaryReservoirSet {
		size = cfg.summaryReservoir
	}

	return &summarySchema{
		quantiles:     qs,
		reservoirSize: size,
	}, nil
}

func buildStateSetSchema(cfg instrumentConfig) (*stateSetSchema, error) {
	if len(cfg.states) == 0 {
		return nil, fmt.Errorf("metrix: stateset requires WithStateSetStates")
	}

	mode := ModeBitSet
	if cfg.stateSetMode != nil {
		mode = *cfg.stateSetMode
	}

	seen := make(map[string]struct{}, len(cfg.states))
	states := make([]string, 0, len(cfg.states))
	for _, st := range cfg.states {
		if st == "" {
			return nil, fmt.Errorf("metrix: stateset state cannot be empty")
		}
		if _, ok := seen[st]; ok {
			return nil, fmt.Errorf("metrix: duplicate stateset state %q", st)
		}
		seen[st] = struct{}{}
		states = append(states, st)
	}

	return &stateSetSchema{
		mode:   mode,
		states: states,
		index:  seen,
	}, nil
}

func equalStateSetSchema(a, b *stateSetSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.mode != b.mode || len(a.states) != len(b.states) {
		return false
	}
	for i := range a.states {
		if a.states[i] != b.states[i] {
			return false
		}
	}
	return true
}

func buildMeasureSetSchema(cfg instrumentConfig) (*measureSetSchema, error) {
	if len(cfg.measureSetFields) == 0 {
		return nil, fmt.Errorf("metrix: measureset requires WithMeasureSetFields")
	}
	if cfg.measureSetSemantics == nil {
		return nil, fmt.Errorf("metrix: measureset semantics are missing")
	}

	fields := make([]MeasureFieldSpec, 0, len(cfg.measureSetFields))
	index := make(map[string]int, len(cfg.measureSetFields))
	for i, field := range cfg.measureSetFields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			return nil, fmt.Errorf("metrix: measureset field name cannot be empty")
		}
		if _, ok := index[name]; ok {
			return nil, fmt.Errorf("metrix: duplicate measureset field %q", name)
		}
		field.Name = name
		fields = append(fields, field)
		index[name] = i
	}

	return &measureSetSchema{
		semantics: *cfg.measureSetSemantics,
		fields:    fields,
		index:     index,
	}, nil
}

func equalMeasureSetSchema(a, b *measureSetSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.semantics != b.semantics || len(a.fields) != len(b.fields) {
		return false
	}
	for i := range a.fields {
		if a.fields[i].Name != b.fields[i].Name || a.fields[i].Float != b.fields[i].Float {
			return false
		}
	}
	return true
}

func equalHistogramSchema(a, b *histogramSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	return equalHistogramBounds(a.bounds, b.bounds)
}

func equalSummarySchema(a, b *summarySchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.reservoirSize != b.reservoirSize || len(a.quantiles) != len(b.quantiles) {
		return false
	}
	for i := range a.quantiles {
		if a.quantiles[i] != b.quantiles[i] {
			return false
		}
	}
	return true
}

func equalHistogramBounds(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeHistogramBounds(in []float64) ([]float64, error) {
	if len(in) == 0 {
		return nil, nil
	}

	bounds := append([]float64(nil), in...)
	out := make([]float64, 0, len(bounds))
	prev := math.Inf(-1)
	for i, b := range bounds {
		if math.IsNaN(b) || math.IsInf(b, -1) {
			return nil, fmt.Errorf("%w: invalid upper bound", errHistogramPoint)
		}
		if math.IsInf(b, +1) {
			if i != len(bounds)-1 {
				return nil, fmt.Errorf("%w: +Inf bucket must be last", errHistogramPoint)
			}
			break // +Inf is implicit.
		}
		if b <= prev {
			return nil, fmt.Errorf("%w: bounds must be strictly increasing", errHistogramPoint)
		}
		out = append(out, b)
		prev = b
	}
	return out, nil
}

func normalizeSummaryQuantiles(in []float64) ([]float64, error) {
	if len(in) == 0 {
		return nil, nil
	}

	qs := append([]float64(nil), in...)
	prev := -1.0
	for _, q := range qs {
		if math.IsNaN(q) || q < 0 || q > 1 {
			return nil, fmt.Errorf("metrix: invalid summary quantile %v", q)
		}
		if q <= prev {
			return nil, fmt.Errorf("metrix: summary quantiles must be strictly increasing")
		}
		prev = q
	}
	return qs, nil
}

func isWindowAllowed(kind metricKind, mode metricMode) bool {
	return mode == modeStateful && (kind == kindHistogram || kind == kindSummary)
}
