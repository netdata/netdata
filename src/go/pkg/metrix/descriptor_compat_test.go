// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestDescriptor builds an instrumentDescriptor directly (bypassing registration)
// so the compatibility helpers can be exercised with same-name incompatible pairs
// that registration itself would never produce.
func newTestDescriptor(name string, kind metricKind, mode metricMode, mutate ...func(*instrumentDescriptor)) *instrumentDescriptor {
	d := &instrumentDescriptor{
		name:      name,
		kind:      kind,
		mode:      mode,
		freshness: defaultFreshness(mode),
	}
	for _, m := range mutate {
		m(d)
	}
	return d
}

// descWithMeta builds a scalar descriptor carrying family metadata and the
// option-set bits that record which of those fields were explicitly declared.
func descWithMeta(meta MetricMeta, set metricMetaSet) *instrumentDescriptor {
	return &instrumentDescriptor{name: "m", kind: kindGauge, mode: modeSnapshot, meta: meta, metaSet: set}
}

// TestDescriptorSeriesAuthorityCompat pins the series-identity contract: two
// descriptors for the same name may back the same series only when their kind,
// mode, freshness, window, and type schema agree. Each case isolates one
// dimension so the first-mismatch error is deterministic.
func TestDescriptorSeriesAuthorityCompat(t *testing.T) {
	tests := map[string]struct {
		existing *instrumentDescriptor
		incoming *instrumentDescriptor
		wantErr  string // "" = compatible; otherwise required error substring
	}{
		"identical snapshot gauge is compatible": {
			existing: newTestDescriptor("m", kindGauge, modeSnapshot),
			incoming: newTestDescriptor("m", kindGauge, modeSnapshot),
		},
		"kind mismatch": {
			existing: newTestDescriptor("m", kindGauge, modeSnapshot),
			incoming: newTestDescriptor("m", kindCounter, modeSnapshot),
			wantErr:  "instrument kind mismatch for m",
		},
		"mode mismatch": {
			existing: newTestDescriptor("m", kindGauge, modeSnapshot),
			incoming: newTestDescriptor("m", kindGauge, modeStateful),
			wantErr:  "instrument mode mismatch for m",
		},
		"freshness mismatch (kind+mode equal)": {
			existing: newTestDescriptor("m", kindCounter, modeStateful, func(d *instrumentDescriptor) { d.freshness = FreshnessCommitted }),
			incoming: newTestDescriptor("m", kindCounter, modeStateful, func(d *instrumentDescriptor) { d.freshness = FreshnessCycle }),
			wantErr:  "instrument freshness mismatch for m",
		},
		"window mismatch (kind+mode+freshness equal)": {
			existing: newTestDescriptor("m", kindCounter, modeStateful, func(d *instrumentDescriptor) { d.freshness = FreshnessCommitted; d.window = WindowCumulative }),
			incoming: newTestDescriptor("m", kindCounter, modeStateful, func(d *instrumentDescriptor) { d.freshness = FreshnessCommitted; d.window = WindowCycle }),
			wantErr:  "instrument window mismatch for m",
		},
		"histogram bounds mismatch": {
			existing: newTestDescriptor("m", kindHistogram, modeSnapshot, func(d *instrumentDescriptor) { d.histogram = &histogramSchema{bounds: []float64{1, 2}} }),
			incoming: newTestDescriptor("m", kindHistogram, modeSnapshot, func(d *instrumentDescriptor) { d.histogram = &histogramSchema{bounds: []float64{1, 3}} }),
			wantErr:  "histogram schema mismatch for m",
		},
		"histogram nil-bounds snapshot is a wildcard (adopts existing)": {
			existing: newTestDescriptor("m", kindHistogram, modeSnapshot, func(d *instrumentDescriptor) { d.histogram = &histogramSchema{bounds: []float64{1, 2}} }),
			incoming: newTestDescriptor("m", kindHistogram, modeSnapshot, func(d *instrumentDescriptor) { d.histogram = nil }),
		},
		"histogram non-nil incoming against nil-bounds existing is a mismatch (wildcard is only for nil incoming)": {
			// incoming carries explicit bounds; existing has nil -> equalHistogramSchema(nil, x) is false,
			// and the wildcard only fires for a nil INCOMING, so this must be a mismatch.
			existing: newTestDescriptor("m", kindHistogram, modeSnapshot, func(d *instrumentDescriptor) { d.histogram = nil }),
			incoming: newTestDescriptor("m", kindHistogram, modeSnapshot, func(d *instrumentDescriptor) { d.histogram = &histogramSchema{bounds: []float64{1, 2}} }),
			wantErr:  "histogram schema mismatch for m",
		},
		"summary quantiles mismatch": {
			existing: newTestDescriptor("m", kindSummary, modeStateful, func(d *instrumentDescriptor) {
				d.freshness = FreshnessCommitted
				d.summary = &summarySchema{quantiles: []float64{0.5}}
			}),
			incoming: newTestDescriptor("m", kindSummary, modeStateful, func(d *instrumentDescriptor) {
				d.freshness = FreshnessCommitted
				d.summary = &summarySchema{quantiles: []float64{0.9}}
			}),
			wantErr: "summary schema mismatch for m",
		},
		"stateset states mismatch": {
			existing: newTestDescriptor("m", kindStateSet, modeSnapshot, func(d *instrumentDescriptor) { d.stateSet = &stateSetSchema{states: []string{"a"}} }),
			incoming: newTestDescriptor("m", kindStateSet, modeSnapshot, func(d *instrumentDescriptor) { d.stateSet = &stateSetSchema{states: []string{"b"}} }),
			wantErr:  "stateset schema mismatch for m",
		},
		"measureset fields mismatch": {
			existing: newTestDescriptor("m", kindMeasureSet, modeSnapshot, func(d *instrumentDescriptor) {
				d.measureSet = &measureSetSchema{fields: []MeasureFieldSpec{{Name: "a"}}}
			}),
			incoming: newTestDescriptor("m", kindMeasureSet, modeSnapshot, func(d *instrumentDescriptor) {
				d.measureSet = &measureSetSchema{fields: []MeasureFieldSpec{{Name: "b"}}}
			}),
			wantErr: "measureset schema mismatch for m",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := descriptorSeriesAuthorityCompat(tc.existing, tc.incoming)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			// Exact match: step 1 must preserve the full registration error strings verbatim.
			require.EqualError(t, err, "metrix: "+tc.wantErr)
		})
	}
}

// TestDescriptorDeclarationCompat pins the metadata contract: a field conflicts
// only when the incoming registration EXPLICITLY set it to a value differing
// from the first (preserve-first). Fields the incoming did not set are ignored,
// even if their values differ.
func TestDescriptorDeclarationCompat(t *testing.T) {
	base := MetricMeta{Description: "A", ChartFamily: "F", ChartPriority: 100, Unit: "u", Float: true}
	allSet := metricMetaSet{description: true, chartFamily: true, chartPriority: true, unit: true, float: true}

	tests := map[string]struct {
		existing *instrumentDescriptor
		incoming *instrumentDescriptor
		wantErr  string
	}{
		"incoming sets nothing keeps existing (preserve-first)": {
			existing: descWithMeta(base, metricMetaSet{}),
			incoming: descWithMeta(MetricMeta{Description: "different", ChartFamily: "other", ChartPriority: 999, Unit: "x", Float: false}, metricMetaSet{}),
		},
		"incoming re-declares identical values": {
			existing: descWithMeta(base, allSet),
			incoming: descWithMeta(base, allSet),
		},
		"description conflict": {
			existing: descWithMeta(base, metricMetaSet{}),
			incoming: descWithMeta(MetricMeta{Description: "B"}, metricMetaSet{description: true}),
			wantErr:  "metric description mismatch for m",
		},
		"description differs but unset is ignored": {
			existing: descWithMeta(base, metricMetaSet{}),
			incoming: descWithMeta(MetricMeta{Description: "B"}, metricMetaSet{}),
		},
		"chart family conflict": {
			existing: descWithMeta(base, metricMetaSet{}),
			incoming: descWithMeta(MetricMeta{ChartFamily: "G"}, metricMetaSet{chartFamily: true}),
			wantErr:  "metric chart family mismatch for m",
		},
		"chart priority conflict": {
			existing: descWithMeta(base, metricMetaSet{}),
			incoming: descWithMeta(MetricMeta{ChartPriority: 200}, metricMetaSet{chartPriority: true}),
			wantErr:  "metric chart priority mismatch for m",
		},
		"unit conflict": {
			existing: descWithMeta(base, metricMetaSet{}),
			incoming: descWithMeta(MetricMeta{Unit: "v"}, metricMetaSet{unit: true}),
			wantErr:  "metric unit mismatch for m",
		},
		"float conflict": {
			existing: descWithMeta(base, metricMetaSet{}),
			incoming: descWithMeta(MetricMeta{Float: false}, metricMetaSet{float: true}),
			wantErr:  "metric float mismatch for m",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := descriptorDeclarationCompat(tc.existing, tc.incoming)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			// Exact match: step 1 must preserve the full registration error strings verbatim.
			require.EqualError(t, err, "metrix: "+tc.wantErr)
		})
	}
}
