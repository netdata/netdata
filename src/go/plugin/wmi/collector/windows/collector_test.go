// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

const (
	cpuProjection     = "SELECT PercentProcessorTime FROM Win32_PerfFormattedData_PerfOS_Processor WHERE Name = '_Total'"
	memoryProjection  = "SELECT TotalVisibleMemorySize, FreePhysicalMemory FROM Win32_OperatingSystem"
	networkProjection = "SELECT Name, BytesReceivedPerSec, BytesSentPerSec FROM Win32_PerfFormattedData_Tcpip_NetworkInterface"
	diskProjection    = "SELECT DeviceID, Size, FreeSpace FROM Win32_LogicalDisk WHERE DriveType = 3"
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
		cpuProjection: fixtureRows(map[string]any{"PercentProcessorTime": uint32(0)}),
		memoryProjection: fixtureRows(map[string]any{
			"TotalVisibleMemorySize": "8388608",
			"FreePhysicalMemory":     uint64(2097152),
		}),
		networkProjection: fixtureRows(
			map[string]any{"Name": "Ethernet", "BytesReceivedPerSec": uint32(190), "BytesSentPerSec": uint64(30)},
			map[string]any{"Name": "Ethernet#1", "BytesReceivedPerSec": uint64(0), "BytesSentPerSec": int32(70)},
		),
		diskProjection: fixtureRows(
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
	sessions[1].results[cpuProjection] = queryResult{
		err: errors.New("fixture-secret"),
	}
	sessions[1].results[memoryProjection] = fixtureRows(map[string]any{"FreePhysicalMemory": uint64(0)})
	sessions[1].results[networkProjection] = fixtureRows(
		map[string]any{"Name": "Ethernet", "BytesReceivedPerSec": "0", "BytesSentPerSec": "malformed"},
		map[string]any{"Name": "_Total", "BytesReceivedPerSec": uint32(30)},
		map[string]any{"BytesSentPerSec": uint64(20)},
	)
	sessions[1].results[diskProjection] = fixtureRows(
		map[string]any{"DeviceID": "C:", "Size": uint64(300), "FreeSpace": uint64(300)},
		map[string]any{"DeviceID": "D:", "Size": uint64(500)},
	)

	next := 0
	c.open = func(ctx context.Context) (queryClient, error) {
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

	first, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		"cpu_busy_percent":       0,
		"memory_used_bytes":      6 * 1024 * 1024 * 1024,
		"memory_available_bytes": 2 * 1024 * 1024 * 1024,
		`network_received_bytes_per_second{interface="Ethernet"}`:   190,
		`network_received_bytes_per_second{interface="Ethernet#1"}`: 0,
		`network_sent_bytes_per_second{interface="Ethernet"}`:       30,
		`network_sent_bytes_per_second{interface="Ethernet#1"}`:     70,
		`disk_used_bytes{drive="C:"}`:                               80000000000,
		`disk_free_bytes{drive="C:"}`:                               420000000000,
		`disk_used_bytes{drive="D:"}`:                               4294967296,
		`disk_free_bytes{drive="D:"}`:                               0,
	}, first)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	second, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		"memory_available_bytes":                                  0,
		`network_received_bytes_per_second{interface="Ethernet"}`: 0,
		`disk_used_bytes{drive="C:"}`:                             0,
		`disk_free_bytes{drive="C:"}`:                             300,
	}, second)

	recovered, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, first, recovered)

	for _, session := range sessions {
		assert.Equal(t, []string{cpuProjection, memoryProjection, networkProjection, diskProjection}, metricQueries(session.queries))
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
					cpuProjection:    {err: tc.queryErr},
					memoryProjection: {err: tc.queryErr},
				},
			}
			c.open = func(_ context.Context) (queryClient, error) {
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
	if wql == cpuProjection {
		return fixtureRows(map[string]any{"PercentProcessorTime": uint32(12)}).rows, nil
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
	c.open = func(ctx context.Context) (queryClient, error) {
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

	assert.Equal(t, []string{cpuProjection, memoryProjection}, session.queries)
	assert.True(t, session.closed)
}

func TestChartTemplateAndMetadata(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	metadata, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	collecttest.AssertMetadataDocumentsChartTemplate(t, metadata, chartTemplateYAML, nil)
}
