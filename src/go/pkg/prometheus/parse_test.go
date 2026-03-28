package prometheus

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

func TestPromTextParser_parseToSeries(t *testing.T) {
	var p promTextParser

	txt := []byte(`
# HELP test_gauge Test Gauge
# TYPE test_gauge gauge
test_gauge{label="b"} 1
# TYPE test_counter_total counter
test_counter_total{label="a"} 2
# TYPE test_summary summary
test_summary{label="c",quantile="0.5"} 3
test_summary_sum{label="c"} 4
test_summary_count{label="c"} 5
# TYPE test_histogram histogram
test_histogram_bucket{label="d",le="0.5"} 6
test_histogram_sum{label="d"} 7
test_histogram_count{label="d"} 8
`)

	got, err := p.parseToSeries(txt)
	require.NoError(t, err)

	want := Series{
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_counter_total"},
				{Name: "label", Value: "a"},
			},
			Value: 2,
		},
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_gauge"},
				{Name: "label", Value: "b"},
			},
			Value: 1,
		},
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_histogram_bucket"},
				{Name: "label", Value: "d"},
				{Name: "le", Value: "0.5"},
			},
			Value: 6,
		},
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_histogram_count"},
				{Name: "label", Value: "d"},
			},
			Value: 8,
		},
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_histogram_sum"},
				{Name: "label", Value: "d"},
			},
			Value: 7,
		},
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_summary"},
				{Name: "label", Value: "c"},
				{Name: "quantile", Value: "0.5"},
			},
			Value: 3,
		},
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_summary_count"},
				{Name: "label", Value: "c"},
			},
			Value: 5,
		},
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_summary_sum"},
				{Name: "label", Value: "c"},
			},
			Value: 4,
		},
	}

	assert.Equal(t, want, got)
}

func TestPromTextParser_parseToSeriesWithSelector(t *testing.T) {
	sr, err := selector.Parse(`test_metric{label="value2"}`)
	require.NoError(t, err)

	p := promTextParser{sr: sr}

	txt := []byte(`
test_metric{label="value1"} 1
test_metric{label="value2"} 2
test_other{label="value2"} 3
`)

	got, err := p.parseToSeries(txt)
	require.NoError(t, err)

	want := Series{
		{
			Labels: labels.Labels{
				{Name: "__name__", Value: "test_metric"},
				{Name: "label", Value: "value2"},
			},
			Value: 2,
		},
	}

	assert.Equal(t, want, got)
}

func TestPromTextParser_parseToSeries_failsOnInvalidSeriesValue(t *testing.T) {
	var p promTextParser

	txt := []byte(`
test_metric{label="ok"} 1
test_metric{label="bad"} ERROR - FAILED TO CONVERT TO STRING
`)

	_, err := p.parseToSeries(txt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse prometheus metrics")
}
