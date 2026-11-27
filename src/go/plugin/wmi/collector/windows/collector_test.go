// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// processorRow is a Processor Information instance with raw times, in 100 ns
// units, per mode.
func processorRow[T uint64 | string](name string, interrupt, user, privileged, dpc, idle T) map[string]any {
	return map[string]any{
		"Name":                  name,
		"PercentInterruptTime":  interrupt,
		"PercentUserTime":       user,
		"PercentPrivilegedTime": privileged,
		"PercentDPCTime":        dpc,
		"PercentIdleTime":       idle,
	}
}

// systemRow is a System counter set instance as DCOM decodes it. The host
// booted 86401.2345678 seconds before the sample; times are 100 ns ticks.
func systemRow() map[string]any {
	return map[string]any{
		"Processes":             uint32(69),
		"Threads":               uint32(951),
		"ContextSwitchesPersec": uint32(4294967000),
		"ProcessorQueueLength":  uint32(3),
		"SystemUpTime":          uint64(133000000000000000),
		"Timestamp_Object":      uint64(133000864012345678),
		"Frequency_Object":      uint64(10000000),
	}
}

func validQueryResults() map[string]queryResult {
	return map[string]queryResult{
		// Instances come in no particular order; "1,0" is the first processor
		// of the second NUMA node.
		queryCPU: fixtureRows(
			processorRow[uint64]("_Total", 10, 100, 50, 5, 1000),
			processorRow[uint64]("0,_Total", 10, 100, 50, 5, 1000),
			processorRow[uint64]("1,0", 5, 5, 5, 5, 5),
			processorRow[uint64]("0,1", 0, 0, 0, 0, 0),
			processorRow[uint64]("0,0", 0, 100, 0, 0, 900),
		),
		queryMemory: fixtureRows(map[string]any{
			"TotalVisibleMemorySize": "8388608",
			"FreePhysicalMemory":     uint64(2097152),
		}),
		querySystem: fixtureRows(systemRow()),
		queryNetworkInterfaces: fixtureRows(
			map[string]any{"Name": "Ethernet", "BytesReceivedPersec": uint64(190), "BytesSentPersec": uint64(30)},
			map[string]any{"Name": "Ethernet#1", "BytesReceivedPersec": uint64(0), "BytesSentPersec": "70"},
			map[string]any{"Name": "isatap.example.com", "BytesReceivedPersec": uint64(0), "BytesSentPersec": "0"},
		),
		// The Network Adapter counter set lists the interfaces again, under the
		// same names, plus adapters without a TCP/IP binding.
		queryNetworkAdapters: fixtureRows(
			map[string]any{"Name": "Ethernet", "BytesReceivedPersec": uint64(190), "BytesSentPersec": uint64(30)},
			map[string]any{"Name": "Ethernet#1", "BytesReceivedPersec": uint64(0), "BytesSentPersec": "70"},
			map[string]any{"Name": "isatap.example.com", "BytesReceivedPersec": uint64(0), "BytesSentPersec": "0"},
			map[string]any{"Name": "Team Member", "BytesReceivedPersec": uint64(400), "BytesSentPersec": uint64(5)},
			map[string]any{
				"Name":                "Teredo Tunneling Pseudo-Interface",
				"BytesReceivedPersec": "0",
				"BytesSentPersec":     "0",
			},
		),
		queryDisk: fixtureRows(
			map[string]any{
				"DeviceID":           "C:",
				"Size":               uint64(500000000000),
				"FreeSpace":          uint64(420000000000),
				"FileSystem":         "NTFS",
				"VolumeSerialNumber": "BEF3E296",
			},
			map[string]any{
				"DeviceID":           "D:",
				"Size":               "4294967296",
				"FreeSpace":          "0",
				"VolumeSerialNumber": "0000ABCD",
			},
		),
	}
}

func configuredCollector() *Collector {
	c := New()
	c.Host = "windows.example.com"
	c.Auth = "ntlm"
	c.Username = "fixture-account"
	c.Password = "fixture-secret"
	ping := &fakePinger{
		sample: answeredPing(),
	}
	c.newPinger = ping.factory
	return c
}

// withAnsweredPing adds the metrics of answeredPing to a collection's metrics.
func withAnsweredPing(metrics map[string]metrix.SampleValue) map[string]metrix.SampleValue {
	const host = `{host="windows.example.com"}`
	metrics["ping_packet_loss"+host] = 0
	metrics["ping_rtt_min"+host] = 1200
	metrics["ping_rtt_max"+host] = 2500
	metrics["ping_rtt_avg"+host] = 1800
	metrics["ping_rtt_std_dev"+host] = 500
	return metrics
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
	// WinRM text, a 32-bit counter that wrapped, a valid zero and no uptime.
	sessions[1].results[querySystem] = fixtureRows(map[string]any{
		"Processes":             "70",
		"Threads":               "malformed",
		"ContextSwitchesPersec": "200",
		"ProcessorQueueLength":  "0",
	})
	sessions[1].results[queryNetworkInterfaces] = fixtureRows(
		map[string]any{"Name": "Ethernet", "BytesReceivedPersec": "250", "BytesSentPersec": "malformed"},
		map[string]any{"Name": "_Total", "BytesReceivedPersec": uint32(30)},
		map[string]any{"BytesSentPersec": uint64(20)},
	)
	// Ethernet#1 is missing from this poll's interfaces, so its adapter is virtual.
	sessions[1].results[queryNetworkAdapters] = fixtureRows(
		map[string]any{"Name": "Ethernet", "BytesReceivedPersec": "250", "BytesSentPersec": "malformed"},
		map[string]any{"Name": "Ethernet#1", "BytesReceivedPersec": uint64(0), "BytesSentPersec": "80"},
	)
	// Each processor's share of the time all its modes accumulated since the
	// first sample; WinRM reports the numbers as text.
	sessions[2].results[queryCPU] = fixtureRows(
		processorRow[uint64]("_Total", 20, 300, 150, 15, 1680),
		processorRow("0,1", "10", "0", "40", "10", "940"),
		processorRow[uint64]("0,0", 0, 600, 0, 0, 1400),
		// A counter that went back leaves the processor without a value.
		processorRow[uint64]("1,0", 5, 5, 5, 5, 4),
	)
	sessions[1].results[queryDisk] = fixtureRows(
		map[string]any{
			"DeviceID":           "C:",
			"Size":               uint64(300),
			"FreeSpace":          uint64(300),
			"FileSystem":         "NTFS",
			"VolumeSerialNumber": "BEF3E296",
		},
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
	const (
		ethernet  = `{device="Ethernet",interface_type="real"}`
		ethernet1 = `{device="Ethernet#1",interface_type="real"}`
		member    = `{device="Team Member",interface_type="virtual"}`
		driveC    = `{drive_type="fixed",filesystem="NTFS",mount_point="C:",rw_mode="rw",serial_number="0xBEF3E296"}`
		driveD    = `{drive_type="fixed",filesystem="unknown",mount_point="D:",rw_mode="rw",serial_number="0xABCD"}`
	)
	assert.Equal(t, withAnsweredPing(map[string]metrix.SampleValue{
		"memory_used_bytes":                  6 * 1024 * 1024 * 1024,
		"memory_free_bytes":                  2 * 1024 * 1024 * 1024,
		"system_processes":                   69,
		"system_threads":                     951,
		"system_context_switches":            4294967000,
		"system_processor_queue_length":      3,
		"system_uptime_milliseconds":         86401234,
		"network_received_bytes" + ethernet:  190,
		"network_received_bytes" + ethernet1: 0,
		"network_sent_bytes" + ethernet:      30,
		"network_sent_bytes" + ethernet1:     70,
		"network_received_bytes" + member:    400,
		"network_sent_bytes" + member:        5,
		"network_total_received_bytes":       190,
		"network_total_sent_bytes":           100,
		"disk_used_bytes" + driveC:           80000000000,
		"disk_avail_bytes" + driveC:          420000000000,
		"disk_used_bytes" + driveD:           4294967296,
		"disk_avail_bytes" + driveD:          0,
	}), first)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	second, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, withAnsweredPing(map[string]metrix.SampleValue{
		"memory_free_bytes":                 0,
		"system_processes":                  70,
		"system_context_switches":           200,
		"system_processor_queue_length":     0,
		"network_received_bytes" + ethernet: 250,
		"network_total_received_bytes":      250,
		`network_received_bytes{device="Ethernet#1",interface_type="virtual"}`: 0,
		`network_sent_bytes{device="Ethernet#1",interface_type="virtual"}`:     80,
		"disk_used_bytes" + driveC:  0,
		"disk_avail_bytes" + driveC: 300,
	}), second)

	// The failed CPU query keeps the first sample, so the shares span both intervals.
	recovered, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	for metric, value := range map[string]metrix.SampleValue{
		"cpu_total_irq_percent":               1,
		"cpu_total_user_percent":              20,
		"cpu_total_system_percent":            10,
		"cpu_total_dpc_percent":               1,
		"cpu_total_idle_percent":              68,
		`cpu_core_irq_percent{cpu="cpu0"}`:    0,
		`cpu_core_user_percent{cpu="cpu0"}`:   50,
		`cpu_core_system_percent{cpu="cpu0"}`: 0,
		`cpu_core_dpc_percent{cpu="cpu0"}`:    0,
		`cpu_core_idle_percent{cpu="cpu0"}`:   50,
		`cpu_core_irq_percent{cpu="cpu1"}`:    1,
		`cpu_core_user_percent{cpu="cpu1"}`:   0,
		`cpu_core_system_percent{cpu="cpu1"}`: 4,
		`cpu_core_dpc_percent{cpu="cpu1"}`:    1,
		`cpu_core_idle_percent{cpu="cpu1"}`:   94,
	} {
		first[metric] = value
	}
	assert.Equal(t, first, recovered)

	for _, session := range sessions {
		assert.Equal(
			t,
			[]string{queryCPU, queryMemory, querySystem, queryNetworkInterfaces, queryNetworkAdapters, queryDisk},
			metricQueries(session.queries),
		)
		assert.Equal(t, 1, session.closed)
	}
	c.Cleanup(t.Context())
}

func TestCollector_NetworkAdapterQuery(t *testing.T) {
	ethernet := map[string]any{"Name": "Ethernet", "BytesReceivedPersec": uint64(190), "BytesSentPersec": uint64(30)}
	for name, tc := range map[string]struct {
		interfaces  queryResult
		adapters    queryResult
		wantQueries []string
		wantMetrics map[string]metrix.SampleValue
	}{
		"a failed adapter query keeps the interfaces": {
			interfaces: fixtureRows(ethernet),
			adapters: queryResult{
				err: errors.New("invalid class"),
			},
			wantQueries: []string{queryNetworkInterfaces, queryNetworkAdapters},
			wantMetrics: withAnsweredPing(map[string]metrix.SampleValue{
				`network_received_bytes{device="Ethernet",interface_type="real"}`: 190,
				`network_sent_bytes{device="Ethernet",interface_type="real"}`:     30,
				"network_total_received_bytes":                                    190,
				"network_total_sent_bytes":                                        30,
			}),
		},
		// Without the interface list, every adapter would pass for virtual.
		"an empty interface list leaves the adapters out": {
			interfaces:  fixtureRows(),
			adapters:    fixtureRows(ethernet),
			wantQueries: []string{queryNetworkInterfaces},
			wantMetrics: withAnsweredPing(map[string]metrix.SampleValue{}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			c.Collectors.CPU = new(false)
			c.Collectors.Memory = new(false)
			c.Collectors.System = new(false)
			c.Collectors.Disk = new(false)
			session := &fakeSession{
				results: map[string]queryResult{
					queryNetworkInterfaces: tc.interfaces,
					queryNetworkAdapters:   tc.adapters,
				},
			}
			c.newSession = func(context.Context) (querySession, error) { return session, nil }
			require.NoError(t, c.Init(t.Context()))

			metrics, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)
			assert.Equal(t, tc.wantMetrics, metrics)
			assert.Equal(t, tc.wantQueries, metricQueries(session.queries))
		})
	}
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
			c.Collectors.System = new(false)
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
	// The ping charts reuse the go.d ping collector's contexts for its alerts,
	// which state their own units (ms, %, up/down), so only targets are checked.
	ping, err := os.ReadFile("../../../../../health/health.d/ping.conf")
	require.NoError(t, err)
	collecttest.AssertHealthAlertsTargetChartTemplate(t, ping, chartTemplateYAML)
}

func metricQueries(queries []string) []string {
	var metrics []string
	for _, query := range queries {
		switch query {
		case queryCPU, queryMemory, querySystem, queryNetworkInterfaces, queryNetworkAdapters, queryDisk:
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
				queryCPU:               validQueryResults()[queryCPU],
				queryMemory:            validQueryResults()[queryMemory],
				querySystem:            lost,
				queryNetworkInterfaces: lost,
				queryDisk:              lost,
			},
			wantMetrics: withAnsweredPing(map[string]metrix.SampleValue{
				"memory_used_bytes": 6 * 1024 * 1024 * 1024,
				"memory_free_bytes": 2 * 1024 * 1024 * 1024,
			}),
		},
		"after a refused query, report the query failures": {
			results: map[string]queryResult{
				queryCPU:               {err: errors.New("WS-Management fault")},
				queryMemory:            lost,
				querySystem:            lost,
				queryNetworkInterfaces: lost,
				queryDisk:              lost,
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
			// A lost interface query leaves no interface list, so the adapter query is skipped.
			assert.Equal(
				t,
				[]string{queryCPU, queryMemory, querySystem, queryNetworkInterfaces, queryDisk},
				session.queries,
			)
		})
	}
}

func TestReadDisks(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		want       []diskSpace
		wantUsable bool
	}{
		"labels as the native Windows Agent writes them": {
			rows: fixtureRows(
				map[string]any{
					"DeviceID":           "C:",
					"Size":               uint64(500),
					"FreeSpace":          uint64(200),
					"FileSystem":         "ReFS",
					"VolumeSerialNumber": "00c0ffee",
					"Access":             uint16(0),
				},
				map[string]any{"DeviceID": "D:", "Size": "100", "FreeSpace": "100"},
			),
			want: []diskSpace{
				{
					mountPoint:   "C:",
					fileSystem:   "ReFS",
					serialNumber: "0xC0FFEE",
					avail: sample{
						value: 200,
						valid: true,
					},
					used: sample{
						value: 300,
						valid: true,
					},
				},
				{
					mountPoint:   "D:",
					fileSystem:   "unknown",
					serialNumber: "0x0",
					avail: sample{
						value: 100,
						valid: true,
					},
					used: sample{
						value: 0,
						valid: true,
					},
				},
			},
			wantUsable: true,
		},
		"read-only drives are left out": {
			rows: fixtureRows(
				map[string]any{"DeviceID": "E:", "Size": uint64(100), "FreeSpace": uint64(0), "Access": "1"},
			),
			wantUsable: true,
		},
		"no drive reports free space": {
			rows: fixtureRows(map[string]any{"DeviceID": "C:", "Size": uint64(100)}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readDisks(tc.rows.rows))
			assert.Equal(t, tc.want, o.drives)
		})
	}
}

func TestReadSystem(t *testing.T) {
	valid := func(v uint64) sample {
		return sample{
			value: v,
			valid: true,
		}
	}
	for name, tc := range map[string]struct {
		rows       queryResult
		want       systemCounters
		wantUsable bool
	}{
		"WinRM text": {
			rows: fixtureRows(map[string]any{
				"Processes":             "138",
				"Threads":               "1328",
				"ContextSwitchesPersec": "746215952",
				"ProcessorQueueLength":  "2",
				"SystemUpTime":          "133000000000000000",
				"Timestamp_Object":      "133000000000050000",
				"Frequency_Object":      "10000000",
			}),
			want: systemCounters{
				processes:       valid(138),
				threads:         valid(1328),
				contextSwitches: valid(746215952),
				processorQueue:  valid(2),
				uptime:          valid(5),
			},
			wantUsable: true,
		},
		"a boot time after the sample leaves uptime out": {
			rows: fixtureRows(map[string]any{
				"Processes":        uint32(69),
				"SystemUpTime":     uint64(133000000000000001),
				"Timestamp_Object": uint64(133000000000000000),
				"Frequency_Object": uint64(10000000),
			}),
			want: systemCounters{
				processes: valid(69),
			},
			wantUsable: true,
		},
		"a zero frequency leaves uptime out": {
			rows: fixtureRows(map[string]any{
				"SystemUpTime":     uint64(0),
				"Timestamp_Object": uint64(10000000),
				"Frequency_Object": uint64(0),
			}),
		},
		"an uptime beyond the millisecond range is left out": {
			rows: fixtureRows(map[string]any{
				"SystemUpTime":     uint64(0),
				"Timestamp_Object": uint64(math.MaxUint64),
				"Frequency_Object": uint64(1),
			}),
		},
		"more than one instance": {
			rows: fixtureRows(systemRow(), systemRow()),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readSystem(tc.rows.rows))
			assert.Equal(t, tc.want, o.system)
		})
	}
}
