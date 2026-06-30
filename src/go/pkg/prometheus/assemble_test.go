// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssemble_roundTripMatchesScrape proves the new ScrapeSamples+Assemble seam is
// equivalent to the direct Scrape path (parseToMetricFamilies) when no relabeling is
// applied — the parity the collector's fast path and relabel executor rely on. It
// exercises a gauge, a counter with multiple series, a histogram, and a summary.
func TestAssemble_roundTripMatchesScrape(t *testing.T) {
	text := []byte(`# HELP app_req Total requests.
# TYPE app_req counter
app_req{method="get"} 5
app_req{method="post"} 2
# HELP app_lat Request latency.
# TYPE app_lat histogram
app_lat_bucket{le="0.1"} 1
app_lat_bucket{le="1"} 3
app_lat_bucket{le="+Inf"} 4
app_lat_sum 0.9
app_lat_count 4
# HELP app_q Query duration.
# TYPE app_q summary
app_q{quantile="0.5"} 0.2
app_q{quantile="0.9"} 0.4
app_q_sum 1.1
app_q_count 7
# HELP app_temp Temperature.
# TYPE app_temp gauge
app_temp 42
`)

	var direct promTextParser
	want, err := direct.parseToMetricFamilies(text)
	require.NoError(t, err)

	var viaSamples promTextParser
	batch, err := viaSamples.parseToSamples(text)
	require.NoError(t, err)
	got, err := Assemble(batch)
	require.NoError(t, err)

	assert.Equal(t, want, got)
}
