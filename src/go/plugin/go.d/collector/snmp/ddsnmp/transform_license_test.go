// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runTransform compiles a transform body and applies it to the given
// metric. It mirrors the minimal "execute the template against {Metric: m}"
// contract used by ddsnmpcollector.applyTransform, without crossing package
// boundaries just for the test.
func runTransform(t *testing.T, body string, m *Metric) {
	t.Helper()
	require.NoError(t, executeTransform(body, m))
}

func executeTransform(body string, m *Metric) error {
	tmpl, err := compileTransform(body)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	return tmpl.Execute(&buf, struct{ Metric *Metric }{Metric: m})
}

func TestSetTagTransform(t *testing.T) {
	tests := map[string]struct {
		metric Metric
		body   string
		want   string
	}{
		"stamps value on existing tags map": {
			metric: Metric{Value: 42, Tags: map[string]string{}},
			body:   `{{- setTag .Metric "custom_kind" "expiry_timestamp" -}}`,
			want:   "expiry_timestamp",
		},
		"allocates tags when nil": {
			metric: Metric{Value: 1},
			body:   `{{- setTag .Metric "custom_kind" "state_severity" -}}`,
			want:   "state_severity",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m := tc.metric
			runTransform(t, tc.body, &m)

			require.NotNil(t, m.Tags)
			assert.Equal(t, tc.want, m.Tags["custom_kind"])
			assert.EqualValues(t, tc.metric.Value, m.Value)
		})
	}
}

func TestIsTextDateNoValue(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want bool
	}{
		"empty":           {raw: "", want: true},
		"zero":            {raw: "0", want: true},
		"negative one":    {raw: "-1", want: true},
		"never":           {raw: "never", want: true},
		"perpetual":       {raw: "perpetual", want: true},
		"permanent":       {raw: "permanent", want: true},
		"n/a":             {raw: "n/a", want: true},
		"na":              {raw: "na", want: true},
		"none":            {raw: "none", want: true},
		"unlimited":       {raw: "unlimited", want: true},
		"uint32 max":      {raw: "4294967295", want: true},
		"one":             {raw: "1", want: false},
		"unix timestamp":  {raw: "1798675200", want: false},
		"text date":       {raw: "2026-12-31", want: false},
		"invalid nonzero": {raw: "not-a-date", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsTextDateNoValue(tc.raw))
		})
	}
}
