// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/relabel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputSeriesStructuralCount(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  int
	}{
		"scalar including skipped info":           {"# TYPE app_info gauge\napp_info 1\n# TYPE app_value gauge\napp_value NaN\n", 2},
		"histogram includes supplied infinity":    {"# TYPE app_hist histogram\napp_hist_bucket{le=\"1\"} 1\napp_hist_bucket{le=\"+Inf\"} 2\napp_hist_sum 3\napp_hist_count 2\n", 4},
		"incomplete histogram reserves count sum": {"# TYPE app_hist histogram\napp_hist_bucket{le=\"1\"} 1\n", 3},
		"summary":                               {"# TYPE app_sum summary\napp_sum{quantile=\"0.5\"} 1\napp_sum{quantile=\"0.9\"} 2\napp_sum_sum 3\napp_sum_count 2\n", 4},
		"incomplete summary reserves count sum": {"# TYPE app_sum summary\napp_sum{quantile=\"0.5\"} 1\n", 3},
	} {
		t.Run(name, func(t *testing.T) {
			families := scrape(t, tc.input)
			require.Equal(t, tc.want, countInputSeries(families))
			c := New()
			c.MaxInputSeries = tc.want
			require.NoError(t, c.admitInput(families))
			c.MaxInputSeries--
			require.ErrorContains(t, c.admitInput(families), fmt.Sprintf("%d input series", tc.want))
			c.MaxInputSeries = 0
			require.NoError(t, c.admitInput(families))
		})
	}
}

func TestCollectorInputAdmissionGrowthAndRecovery(t *testing.T) {
	input := "# TYPE app_value gauge\napp_value 1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(input)) }))
	defer srv.Close()
	c := New()
	var logs bytes.Buffer
	c.Logger = logger.NewWithWriter(&logs)
	c.URL = srv.URL
	c.Profiles.Mode = profilesModeNone
	c.MaxInputSeries = 1
	ctx := context.Background()
	require.NoError(t, c.Init(ctx))
	defer c.Cleanup(ctx)
	require.NoError(t, c.Check(ctx))
	cc := cycle(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(ctx))
	require.NoError(t, cc.CommitCycleSuccess())
	writerCycle := c.writer.cycle
	handle := c.writer.handles["app_value"]
	for round := 0; round < 2; round++ {
		input = "# TYPE app_value gauge\napp_value 99\n# TYPE app_new gauge\napp_new 7\n"
		for attempt := 0; attempt < 2; attempt++ {
			cc.BeginCycle()
			err := c.Collect(ctx)
			require.ErrorContains(t, err, "2 input series exceeds max_input_series limit 1")
			assert.NotContains(t, err.Error(), srv.URL)
			assert.Equal(t, writerCycle, c.writer.cycle)
			assert.Len(t, c.writer.handles, 1)
			assert.Same(t, handle, c.writer.handles["app_value"])
			cc.AbortCycle()
		}
		assert.InDelta(t, 1, value(t, c.store.Read(metrix.ReadRaw()), "app_value", nil), 1e-9)
		input = "" // A failed scrape must not report admission recovery.
		cc.BeginCycle()
		require.Error(t, c.Collect(ctx))
		cc.AbortCycle()
		assert.Equal(t, round, strings.Count(logs.String(), "input admission resumed"))
		input = "# TYPE app_value gauge\napp_value 1\n"
		for attempt := 0; attempt < 2; attempt++ {
			cc.BeginCycle()
			require.NoError(t, c.Collect(ctx))
			require.NoError(t, cc.CommitCycleSuccess())
		}
		writerCycle = c.writer.cycle
		assert.Equal(t, round+1, strings.Count(logs.String(), "input admission resumed"))
	}
	assert.NotContains(t, logs.String(), srv.URL)
}

func TestCollectorInputAdmissionAfterSelectorAndJobRelabel(t *testing.T) {
	input := "# TYPE app_keep gauge\napp_keep 1\n# TYPE app_drop gauge\napp_drop 2\n# TYPE other gauge\nother 3\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(input)) }))
	defer srv.Close()
	c := New()
	c.URL = srv.URL
	c.Profiles.Mode = profilesModeNone
	c.MaxInputSeries = 1
	c.Selector = selector.Expr{
		Allow: []string{"app_*"},
	}
	c.Relabeling = []relabel.Block{{Match: "app_drop", MetricRelabelConfigs: []relabel.Config{{Action: relabel.Drop, SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp("app_drop")}}}}
	ctx := context.Background()
	require.NoError(t, c.Init(ctx))
	defer c.Cleanup(ctx)
	require.NoError(t, c.Check(ctx))
	cc := cycle(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(ctx))
	require.NoError(t, cc.CommitCycleSuccess())
	assert.InDelta(t, 1, value(t, c.store.Read(metrix.ReadRaw()), "app_keep", nil), 1e-9)
}

func TestCollectorLimitConfiguration(t *testing.T) {
	for _, tc := range []struct {
		input, output int
		bad           string
	}{
		{10000, 1000, ""}, {0, 0, ""}, {-1, 1000, "max_input_series"}, {10000, -1, "max_output_series_per_context"},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.input, tc.output), func(t *testing.T) {
			c := New()
			c.URL = "http://127.0.0.1:9090/metrics"
			c.MaxInputSeries = tc.input
			c.MaxOutputSeriesPerContext = tc.output
			err := c.Init(context.Background())
			if tc.bad != "" {
				require.ErrorContains(t, err, tc.bad)
				return
			}
			require.NoError(t, err)
			defer c.Cleanup(context.Background())
			require.NotNil(t, c.EnginePolicy().MaxOutputSeriesPerContext)
			assert.Equal(t, tc.output, *c.EnginePolicy().MaxOutputSeriesPerContext)
		})
	}
}

func TestCollectorCheckAdmissionCountsWriterIneligibleDistributions(t *testing.T) {
	input := "# TYPE app_keep gauge\napp_keep 1\n# TYPE app_summary summary\napp_summary{quantile=\"0.5\"} 2\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(input)) }))
	defer srv.Close()
	c := New()
	var logs bytes.Buffer
	c.Logger = logger.NewWithWriter(&logs)
	c.URL = srv.URL
	c.MaxInputSeries = 3
	c.Profiles.Mode = profilesModeNone
	ctx := context.Background()
	require.NoError(t, c.Init(ctx))
	defer c.Cleanup(ctx)
	require.ErrorContains(t, c.Check(ctx), "4 input series exceeds max_input_series limit 3")
	assert.Nil(t, c.runtime)
	assert.Empty(t, c.writer.handles)
	// Admission now fits, but startup still fails the existing writable check.
	input = "# TYPE app_summary summary\napp_summary{quantile=\"0.5\"} 2\n"
	require.ErrorContains(t, c.Check(ctx), "no usable metrics")
	assert.NotContains(t, logs.String(), "input admission resumed")
	input = "# TYPE app_keep gauge\napp_keep 1\n"
	require.NoError(t, c.Check(ctx))
	assert.Equal(t, 1, strings.Count(logs.String(), "input admission resumed"))
	require.NoError(t, c.Check(ctx))
	assert.Equal(t, 1, strings.Count(logs.String(), "input admission resumed"))
}
