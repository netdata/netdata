// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

type queryResult struct {
	rows []map[string]any
	err  error
}

type fakeSession struct {
	results map[string]queryResult
	queries []string
	closed  int
}

func (f *fakeSession) Query(_ context.Context, wql string) ([]map[string]any, error) {
	f.queries = append(f.queries, wql)
	result, ok := f.results[wql]
	if !ok {
		return nil, errors.New("unexpected query")
	}
	return result.rows, result.err
}

func (f *fakeSession) Close() { f.closed++ }

func fixtureRows(records ...map[string]any) queryResult {
	// WMI property names are case-insensitive. The query client lowercases them.
	rows := make([]map[string]any, 0, len(records))
	for _, record := range records {
		row := make(map[string]any, len(record))
		for key, value := range record {
			row[strings.ToLower(key)] = value
		}
		rows = append(rows, row)
	}
	return queryResult{
		rows: rows,
	}
}

func validQueryResults() map[string]queryResult {
	return map[string]queryResult{
		queryCPU: fixtureRows(map[string]any{"PercentProcessorTime": uint64(75_000_000), "Timestamp_Sys100NS": uint64(100_000_000)}),
		queryMemory: fixtureRows(map[string]any{
			"TotalVisibleMemorySize": "8388608",
			"FreePhysicalMemory":     uint64(2097152),
		}),
		queryNetwork: fixtureRows(
			map[string]any{"Name": "Ethernet", "BytesReceivedPersec": uint64(190), "BytesSentPersec": uint64(30)},
			map[string]any{"Name": "Ethernet#1", "BytesReceivedPersec": uint64(0), "BytesSentPersec": "70"},
		),
		queryDisk: fixtureRows(
			map[string]any{"DeviceID": "C:", "Size": uint64(500000000000), "FreeSpace": uint64(420000000000)},
			map[string]any{"DeviceID": "D:", "Size": "4294967296", "FreeSpace": "0"},
		),
	}
}

func configuredCollector() *Collector {
	c := New()
	c.Host = "windows.example.com"
	c.Auth = "ntlm"
	c.Username = "fixture-account"
	c.Password = "fixture-secret"
	return c
}

func TestCollector_Collect(t *testing.T) {
	c := configuredCollector()
	sessions := []*fakeSession{
		{results: validQueryResults()},
		{results: validQueryResults()},
		{results: validQueryResults()},
	}
	// These source readings distinguish absent properties, malformed numbers,
	// valid zero, and entities missing from a successful table response.
	sessions[1].results[queryCPU] = queryResult{
		err: errors.New("fixture-secret"),
	}
	sessions[1].results[queryMemory] = fixtureRows(map[string]any{"FreePhysicalMemory": uint64(0)})
	sessions[1].results[queryNetwork] = fixtureRows(
		map[string]any{"Name": "Ethernet", "BytesReceivedPersec": "0", "BytesSentPersec": "malformed"},
		map[string]any{"Name": "_Total", "BytesReceivedPersec": uint32(30)},
		map[string]any{"BytesSentPersec": uint64(20)},
	)
	// 20 s of processor time later, idle for 15 s of it: 25% busy since the first sample.
	sessions[2].results[queryCPU] = fixtureRows(map[string]any{
		"PercentProcessorTime": uint64(90_000_000),
		"Timestamp_Sys100NS":   uint64(120_000_000),
	})
	sessions[1].results[queryDisk] = fixtureRows(
		map[string]any{"DeviceID": "C:", "Size": uint64(300), "FreeSpace": uint64(300)},
		map[string]any{"DeviceID": "D:", "Size": uint64(500)},
	)

	next := 0
	c.newSession = func(ctx context.Context) (querySession, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		assert.Positive(t, time.Until(deadline))
		assert.LessOrEqual(t, time.Until(deadline), 15*time.Second)
		require.Less(t, next, len(sessions))
		session := sessions[next]
		next++
		return session, nil
	}
	require.NoError(t, c.Init(t.Context()))

	// No CPU value yet: an interval needs a previous sample.
	first, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		"memory_used_bytes":                              6 * 1024 * 1024 * 1024,
		"memory_available_bytes":                         2 * 1024 * 1024 * 1024,
		`network_received_bytes{interface="Ethernet"}`:   190,
		`network_received_bytes{interface="Ethernet#1"}`: 0,
		`network_sent_bytes{interface="Ethernet"}`:       30,
		`network_sent_bytes{interface="Ethernet#1"}`:     70,
		`disk_used_bytes{drive="C:"}`:                    80000000000,
		`disk_free_bytes{drive="C:"}`:                    420000000000,
		`disk_used_bytes{drive="D:"}`:                    4294967296,
		`disk_free_bytes{drive="D:"}`:                    0,
	}, first)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	second, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		"memory_available_bytes":                       0,
		`network_received_bytes{interface="Ethernet"}`: 0,
		`disk_used_bytes{drive="C:"}`:                  0,
		`disk_free_bytes{drive="C:"}`:                  300,
	}, second)

	// The failed CPU query keeps the first sample, so the average spans both intervals.
	recovered, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	first["cpu_busy_percent"] = 25
	assert.Equal(t, first, recovered)

	for _, session := range sessions {
		assert.Equal(t, []string{queryCPU, queryMemory, queryNetwork, queryDisk}, metricQueries(session.queries))
		assert.Equal(t, 1, session.closed)
	}
	c.Cleanup(t.Context())
}

func TestCollector_CheckFailure(t *testing.T) {
	const secret = "permission denied: fixture-account fixture-secret"
	for name, tc := range map[string]struct {
		openErr   error
		queryErr  error
		wantError string
	}{
		"session rejected": {
			openErr: errors.New(secret), wantError: "failed to open WMI session",
		},
		"queries rejected": {
			queryErr: errors.New(secret), wantError: "CPU WMI query failed",
		},
		"empty fixed-dimension groups": {
			wantError: "CPU WMI query returned no usable observations",
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			c.Collectors.Network = new(false)
			c.Collectors.Disk = new(false)
			session := &fakeSession{
				results: map[string]queryResult{
					queryCPU:    {err: tc.queryErr},
					queryMemory: {err: tc.queryErr},
				},
			}
			c.newSession = func(_ context.Context) (querySession, error) {
				if tc.openErr != nil {
					return nil, tc.openErr
				}
				return session, nil
			}
			require.NoError(t, c.Init(t.Context()))
			err := c.Check(t.Context())
			require.ErrorContains(t, err, tc.wantError)
			assert.NotContains(t, err.Error(), "fixture-")
			if tc.openErr == nil {
				assert.Equal(t, 1, session.closed)
			}
		})
	}
}

type cancellableSession struct {
	t       *testing.T
	ctx     context.Context
	cancel  context.CancelFunc
	queries []string
	closed  bool
}

func (f *cancellableSession) Query(ctx context.Context, wql string) ([]map[string]any, error) {
	f.queries = append(f.queries, wql)
	assert.Same(f.t, f.ctx, ctx)
	if wql == queryCPU {
		return validQueryResults()[queryCPU].rows, nil
	}
	f.cancel()
	return nil, ctx.Err()
}

func (f *cancellableSession) Close() { f.closed = true }

func TestCollector_CancelDiscardsPartialCycle(t *testing.T) {
	c := configuredCollector()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	session := &cancellableSession{
		t:      t,
		cancel: cancel,
	}
	c.newSession = func(ctx context.Context) (querySession, error) {
		session.ctx = ctx
		return session, nil
	}

	require.NoError(t, c.Init(ctx))
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	assert.ErrorIs(t, c.Collect(ctx), context.Canceled)
	cycle.AbortCycle()

	assert.Equal(t, []string{queryCPU, queryMemory}, session.queries)
	assert.True(t, session.closed)
}

func TestChartTemplateAndMetadata(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, chartTemplateYAML, nil)
}

func TestMetadataDocumentsChartLabels(t *testing.T) {
	charts, err := collecttest.ChartTemplateCharts(chartTemplateYAML)
	require.NoError(t, err)
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	var doc struct {
		Modules []struct {
			Metrics struct {
				Scopes []struct {
					Labels []struct {
						Name string `yaml:"name"`
					} `yaml:"labels"`
					Metrics []struct {
						Name string `yaml:"name"`
					} `yaml:"metrics"`
				} `yaml:"scopes"`
			} `yaml:"metrics"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Len(t, doc.Modules, 1)

	for _, scope := range doc.Modules[0].Metrics.Scopes {
		var documented []string
		for _, label := range scope.Labels {
			documented = append(documented, label.Name)
		}
		for _, metric := range scope.Metrics {
			chart, ok := charts[metric.Name]
			require.True(t, ok, metric.Name)
			var attached []string
			if chart.Instances != nil {
				attached = chart.Instances.ByLabels
			}
			assert.ElementsMatch(t, attached, documented, "labels of %s", metric.Name)
		}
	}
}

func metricQueries(queries []string) []string {
	var metrics []string
	for _, query := range queries {
		switch query {
		case queryCPU, queryMemory, queryNetwork, queryDisk:
			metrics = append(metrics, query)
		}
	}
	return metrics
}

type sessionFailure struct{ error }

func (sessionFailure) SessionFailed() bool { return true }

func TestCollector_SessionFailureStopsThePoll(t *testing.T) {
	c := configuredCollector()
	c.Transport = transportWinRM
	session := &fakeSession{
		results: validQueryResults(),
	}
	session.results[queryCPU] = queryResult{
		err: sessionFailure{errors.New("fixture-secret rejected")},
	}
	c.newSession = func(context.Context) (querySession, error) { return session, nil }
	require.NoError(t, c.Init(t.Context()))

	err := c.Check(t.Context())
	require.EqualError(t, err, "failed to open WMI session")
	assert.Equal(t, []string{queryCPU}, session.queries)
	assert.Equal(t, 1, session.closed)
}

type stalledSession struct{ fakeSession }

func (s *stalledSession) Query(ctx context.Context, wql string) ([]map[string]any, error) {
	s.queries = append(s.queries, wql)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCollector_TimeoutNamesTheLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		session := &stalledSession{}
		c.newSession = func(context.Context) (querySession, error) { return session, nil }
		require.NoError(t, c.Init(t.Context()))

		err := c.Check(t.Context())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.EqualError(t, err, "WMI collection did not finish within the timeout of 15s")
		assert.Equal(t, []string{queryCPU}, session.queries)
	})
}

func TestCollector_SessionLostMidPoll(t *testing.T) {
	lost := queryResult{
		err: sessionFailure{errors.New("connection reset")},
	}
	for name, tc := range map[string]struct {
		results     map[string]queryResult
		wantErr     string
		wantMetrics map[string]metrix.SampleValue
	}{
		"after a successful group, keep its samples and skip the inventory": {
			results: map[string]queryResult{
				queryCPU:     validQueryResults()[queryCPU],
				queryMemory:  validQueryResults()[queryMemory],
				queryNetwork: lost,
				queryDisk:    lost,
			},
			wantMetrics: map[string]metrix.SampleValue{
				"memory_used_bytes":      6 * 1024 * 1024 * 1024,
				"memory_available_bytes": 2 * 1024 * 1024 * 1024,
			},
		},
		"after a refused query, report the query failures": {
			results: map[string]queryResult{
				queryCPU:     {err: errors.New("WS-Management fault")},
				queryMemory:  lost,
				queryNetwork: lost,
				queryDisk:    lost,
			},
			wantErr: "no enabled WMI query group succeeded",
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			session := &fakeSession{
				results: tc.results,
			}
			c.newSession = func(context.Context) (querySession, error) { return session, nil }
			require.NoError(t, c.Init(t.Context()))

			metrics, err := collecttest.CollectScalarSeries(c)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantMetrics, metrics)
			}
			assert.Equal(t, []string{queryCPU, queryMemory, queryNetwork, queryDisk}, session.queries)
		})
	}
}

func TestBusyPercent(t *testing.T) {
	sample := func(idle, timestamp uint64) cpuTimes {
		return cpuTimes{
			idle:      idle,
			timestamp: timestamp,
			valid:     true,
		}
	}
	for name, tc := range map[string]struct {
		prev, cur cpuTimes
		want      percent
	}{
		"no previous sample": {cur: sample(10, 100), want: percent{}},
		"busy share of interval": {prev: sample(10, 100), cur: sample(70, 180), want: percent{
			value: 25,
			valid: true,
		}},
		"fully idle": {prev: sample(10, 100), cur: sample(90, 180), want: percent{
			value: 0,
			valid: true,
		}},
		"idle above elapsed": {prev: sample(10, 100), cur: sample(95, 180), want: percent{
			value: 0,
			valid: true,
		}},
		"same timestamp":         {prev: sample(10, 100), cur: sample(10, 100), want: percent{}},
		"timestamp reset":        {prev: sample(10, 100), cur: sample(5, 50), want: percent{}},
		"idle counter went back": {prev: sample(50, 100), cur: sample(40, 180), want: percent{}},
		"invalid current sample": {prev: sample(10, 100), cur: cpuTimes{}, want: percent{}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, busyPercent(tc.prev, tc.cur))
		})
	}
}
