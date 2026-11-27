// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

// Two PhysicalDisk samples of one lab host's disk, 2.2 seconds apart, as DCOM
// decoded them: 18 writes of 106496 bytes in total took 67068 ticks of the
// 10 MHz performance counter, no read completed, and the disk was idle for
// 21847786 of 22026218 100 ns units. The read time has wrapped its 32 bits 166
// times since the host started: its queue-length integral is exactly the
// unwrapped time.
func labDiskRows() (first, second map[string]any) {
	first = map[string]any{
		"Name":                       "0 C:",
		"DiskReadBytesPersec":        uint64(463502537216),
		"DiskWriteBytesPersec":       uint64(359558174208),
		"DiskReadsPersec":            uint32(78084033),
		"DiskWritesPersec":           uint32(5556613),
		"SplitIOPerSec":              uint32(1681850),
		"CurrentDiskQueueLength":     uint32(0),
		"PercentDiskTime":            uint64(785393584119),
		"PercentDiskReadTime":        uint64(713408378323),
		"PercentDiskWriteTime":       uint64(71985205796),
		"PercentIdleTime":            uint64(14123705624035),
		"PercentIdleTime_Base":       uint64(134402530954054165),
		"AvgDisksecPerRead":          uint32(443807187),
		"AvgDisksecPerRead_Base":     uint32(78084033),
		"AvgDiskReadQueueLength":     uint64(713408378323),
		"AvgDisksecPerWrite":         uint32(3265729060),
		"AvgDisksecPerWrite_Base":    uint32(5556613),
		"AvgDiskWriteQueueLength":    uint64(71985205796),
		"AvgDisksecPerTransfer":      uint32(3709536247),
		"AvgDisksecPerTransfer_Base": uint32(83640646),
		"AvgDiskQueueLength":         uint64(785393584119),
		"AvgDiskBytesPerRead":        uint64(463502537216),
		"AvgDiskBytesPerRead_Base":   uint32(78084033),
		"AvgDiskBytesPerWrite":       uint64(359558174208),
		"AvgDiskBytesPerWrite_Base":  uint32(5556613),
		"Frequency_PerfTime":         uint64(10000000),
	}
	second = map[string]any{
		"Name":                       "0 C:",
		"DiskReadBytesPersec":        uint64(463502537216),
		"DiskWriteBytesPersec":       uint64(359558280704),
		"DiskReadsPersec":            uint32(78084033),
		"DiskWritesPersec":           uint32(5556631),
		"SplitIOPerSec":              uint32(1681850),
		"CurrentDiskQueueLength":     uint32(0),
		"PercentDiskTime":            uint64(785393651187),
		"PercentDiskReadTime":        uint64(713408378323),
		"PercentDiskWriteTime":       uint64(71985272864),
		"PercentIdleTime":            uint64(14123727471821),
		"PercentIdleTime_Base":       uint64(134402530976080383),
		"AvgDisksecPerRead":          uint32(443807187),
		"AvgDisksecPerRead_Base":     uint32(78084033),
		"AvgDiskReadQueueLength":     uint64(713408378323),
		"AvgDisksecPerWrite":         uint32(3265796128),
		"AvgDisksecPerWrite_Base":    uint32(5556631),
		"AvgDiskWriteQueueLength":    uint64(71985272864),
		"AvgDisksecPerTransfer":      uint32(3709603315),
		"AvgDisksecPerTransfer_Base": uint32(83640664),
		"AvgDiskQueueLength":         uint64(785393651187),
		"AvgDiskBytesPerRead":        uint64(463502537216),
		"AvgDiskBytesPerRead_Base":   uint32(78084033),
		"AvgDiskBytesPerWrite":       uint64(359558280704),
		"AvgDiskBytesPerWrite_Base":  uint32(5556631),
		"Frequency_PerfTime":         uint64(10000000),
	}
	return first, second
}

func TestCollector_PhysicalDiskIntervals(t *testing.T) {
	first, second := labDiskRows()
	session := func(row map[string]any) *fakeSession {
		results := validQueryResults()
		results[queryPhysicalDisks] = fixtureRows(row)
		results[queryDiskDrives] = fixtureRows(map[string]any{
			"Index":    uint32(0),
			"Model":    "QEMU HARDDISK",
			"DeviceID": `\\.\PHYSICALDRIVE0`,
		})
		return &fakeSession{
			results: results,
		}
	}
	sessions := []*fakeSession{session(first), session(second)}
	c := configuredCollector()
	c.newSession = func(context.Context) (querySession, error) {
		s := sessions[0]
		sessions = sessions[1:]
		return s, nil
	}
	require.NoError(t, c.Init(t.Context()))
	// Check's poll primes the disk's sample, as it does the CPU's.
	require.NoError(t, c.Check(t.Context()))

	metrics, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	const disk = `{device="Disk 0",device_id="\\\\.\\PHYSICALDRIVE0",model="QEMU HARDDISK",mount_point="C:"}`
	got := make(map[string]metrix.SampleValue)
	for _, metric := range []string{
		"disk_utilization_percent",
		"disk_read_await_microseconds",
		"disk_write_await_microseconds",
		"disk_service_time_microseconds",
		"disk_read_average_bytes",
		"disk_write_average_bytes",
	} {
		if value, ok := metrics[metric+disk]; ok {
			got[metric] = value
		}
	}
	// The native formulas: utilization 100 - 100*21847786/22026218 in whole
	// percent; write and transfer time 67068 ticks * 1000 / (10^7 * 18)
	// milliseconds, sent in whole microseconds; 106496 bytes / 18 writes; no
	// read completed, so no read time or size.
	assert.Equal(t, map[string]metrix.SampleValue{
		"disk_utilization_percent":       1,
		"disk_read_await_microseconds":   0,
		"disk_write_await_microseconds":  372,
		"disk_service_time_microseconds": 372,
		"disk_read_average_bytes":        0,
		"disk_write_average_bytes":       5916,
	}, got)
	// Times arrive in whole milliseconds, as the native agent charts them.
	assert.Equal(t, metrix.SampleValue(78539365), metrics["disk_busy_milliseconds"+disk])
	assert.Equal(t, metrix.SampleValue(7198527), metrics["disk_write_time_milliseconds"+disk])
	assert.Empty(t, sessions)
}

// TestCollector_DiskIntervalsSpanConsecutivePolls forms disk intervals only
// between polls that both read the disks. Across a failed poll, 2^32 + 1000
// reads of 1 ms each look like 1000 reads to the 32-bit count, while their
// time is unwrapped in full, so the interval would claim reads of 4295 s.
func TestCollector_DiskIntervalsSpanConsecutivePolls(t *testing.T) {
	first, second := labDiskRows()
	// advance adds a delta to each counter, in the counter's own width.
	advance := func(row map[string]any, delta func(property string) uint64) map[string]any {
		next := make(map[string]any, len(row))
		for property, value := range row {
			switch v := value.(type) {
			case uint32:
				next[property] = uint32(uint64(v) + delta(property))
			case uint64:
				next[property] = v + delta(property)
			default:
				next[property] = v
			}
		}
		return next
	}
	const reads = counter32Range + 1000
	readTime := map[string]uint64{
		"DiskReadsPersec":          reads,
		"AvgDisksecPerRead_Base":   reads,
		"AvgDiskBytesPerRead_Base": reads,
		// 1 ms each, in 100 ns units and in ticks of the 10 MHz counter.
		"AvgDisksecPerRead":      reads * 10000,
		"AvgDiskReadQueueLength": reads * 10000,
		"PercentDiskReadTime":    reads * 10000,
		"AvgDiskBytesPerRead":    reads * 4096,
		"DiskReadBytesPersec":    reads * 4096,
	}
	afterGap := advance(first, func(property string) uint64 { return readTime[property] })
	// The lab's own 2.2-second interval follows.
	labDelta := func(property string) uint64 {
		switch v := second[property].(type) {
		case uint32:
			return uint64(v - first[property].(uint32))
		case uint64:
			return v - first[property].(uint64)
		}
		return 0
	}
	next := advance(afterGap, labDelta)

	results := func(rows ...map[string]any) queryResult {
		if len(rows) == 0 {
			return queryResult{
				err: errors.New("invalid class"),
			}
		}
		return fixtureRows(rows...)
	}
	polls := []queryResult{results(first), results(), results(afterGap), results(next)}
	c := configuredCollector()
	c.Collectors = Collectors{
		CPU:     new(false),
		Memory:  new(false),
		System:  new(false),
		Network: new(false),
		SMB:     new(false),
		Time:    new(false),
	}
	c.newSession = func(context.Context) (querySession, error) {
		session := &fakeSession{
			results: map[string]queryResult{
				queryDisk:          fixtureRows(),
				queryPhysicalDisks: polls[0],
				queryDiskDrives:    fixtureRows(),
			},
		}
		polls = polls[1:]
		return session, nil
	}
	require.NoError(t, c.Init(t.Context()))
	require.NoError(t, c.Check(t.Context()))

	const disk = `{device="Disk 0",mount_point="C:"}`
	intervalValues := func(metrics map[string]metrix.SampleValue) map[string]metrix.SampleValue {
		got := make(map[string]metrix.SampleValue)
		for _, metric := range []string{
			"disk_utilization_percent",
			"disk_read_await_microseconds",
			"disk_write_await_microseconds",
			"disk_service_time_microseconds",
			"disk_read_average_bytes",
			"disk_write_average_bytes",
		} {
			if value, ok := metrics[metric+disk]; ok {
				got[metric] = value
			}
		}
		return got
	}
	for i, want := range []map[string]metrix.SampleValue{
		// The physical disk query fails; the drive space query succeeds.
		{},
		// No interval across the gap.
		{},
		// The lab interval, as in TestCollector_PhysicalDiskIntervals.
		{
			"disk_utilization_percent":       1,
			"disk_read_await_microseconds":   0,
			"disk_write_await_microseconds":  372,
			"disk_service_time_microseconds": 372,
			"disk_read_average_bytes":        0,
			"disk_write_average_bytes":       5916,
		},
	} {
		metrics, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		assert.Equal(t, want, intervalValues(metrics), "collection %d", i)
	}
	assert.Empty(t, polls)
}

// stalledDriveSession answers every query except the disk drive lookup, which
// waits until its context ends.
type stalledDriveSession struct{ fakeSession }

func (s *stalledDriveSession) Query(ctx context.Context, wql string) ([]map[string]any, error) {
	if wql == queryDiskDrives {
		s.queries = append(s.queries, wql)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.fakeSession.Query(ctx, wql)
}

// TestCollector_DiskDriveLookupBudget keeps a disk drive lookup that does not
// answer from discarding the poll: the samples are published without the
// labels, and the next poll looks the disk up again.
func TestCollector_DiskDriveLookupBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "manual"
		var sessions []*stalledDriveSession
		c.newSession = func(context.Context) (querySession, error) {
			session := &stalledDriveSession{
				fakeSession: fakeSession{
					results: validQueryResults(),
				},
			}
			sessions = append(sessions, session)
			return session, nil
		}
		require.NoError(t, c.Init(t.Context()))
		for poll := range 2 {
			metrics, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err, "poll %d", poll)
			assert.Equal(t, metrix.SampleValue(218802), metrics[`disk_reads{device="Disk 0",mount_point="C:"}`])
			assert.Equal(t, metrix.SampleValue(3), metrics["system_processor_queue_length"])
			assert.Contains(t, sessions[poll].queries, queryDiskDrives, "poll %d", poll)
		}
	})
}

func TestAverageMicroseconds(t *testing.T) {
	const tenMHz = 10000000
	timer := func(ticks, operations, queueTime uint64) averageTimer {
		return averageTimer{
			ticks: sample{
				value: ticks,
				valid: true,
			},
			operations: sample{
				value: operations,
				valid: true,
			},
			queueTime: sample{
				value: queueTime,
				valid: true,
			},
		}
	}
	frequency := func(hz uint64) sample {
		return sample{
			value: hz,
			valid: true,
		}
	}
	valid := func(microseconds uint64) sample {
		return sample{
			value: microseconds,
			valid: true,
		}
	}
	for name, tc := range map[string]struct {
		previous, current averageTimer
		frequency         sample
		want              sample
	}{
		// 100 operations of 2 ms: 2,000,000 ticks at 10 MHz.
		"operations completed": {
			previous:  timer(1000, 10, 1000),
			current:   timer(2001000, 110, 2001000),
			frequency: frequency(tenMHz),
			want:      valid(2000),
		},
		// The 32-bit time wraps; its 64-bit integral does not.
		"time wrapped once": {
			previous:  timer(4294000000, 10, 5*counter32Range+4294000000),
			current:   timer(1032704, 110, 6*counter32Range+1032704),
			frequency: frequency(tenMHz),
			want:      valid(2000),
		},
		// 20,000 operations of 1 ms in 30 s add 6 * 10^9 ticks, more than 2^32.
		"time wrapped more than once in the interval": {
			previous:  timer(0, 0, 0),
			current:   timer(6000000000%counter32Range, 600000, 6000000000),
			frequency: frequency(tenMHz),
			want:      valid(1000),
		},
		// 6,000,000 operations of 1 ms at 3,579,545 Hz: a 100 ns queue unit is
		// not a tick, so the integral is converted before the wraps are counted.
		"other counter frequency": {
			previous:  timer(0, 0, 0),
			current:   timer(21477270000%counter32Range, 6000000, 60000000000),
			frequency: frequency(3579545),
			want:      valid(1000),
		},
		"operation count wrapped": {
			previous:  timer(0, counter32Range-50, 0),
			current:   timer(2000000, 50, 2000000),
			frequency: frequency(tenMHz),
			want:      valid(2000),
		},
		"no operation completed": {
			previous:  timer(1000, 10, 1000),
			current:   timer(1000, 10, 50000000),
			frequency: frequency(tenMHz),
			want:      valid(0),
		},
		"time without operations": {
			previous:  timer(1000, 10, 1000),
			current:   timer(5000, 10, 5000),
			frequency: frequency(tenMHz),
		},
		// The host restarted after nearly 2^32 operations: their count looks
		// like a small increase, the integral shows the restart.
		"counters restarted": {
			previous:  timer(4000000000, counter32Range-1000, 90000000000),
			current:   timer(2000, 100, 2000),
			frequency: frequency(tenMHz),
		},
		"operation count reset": {
			previous:  timer(1000, 1000000000, 1000),
			current:   timer(2001000, 100, 2001000),
			frequency: frequency(tenMHz),
		},
		"no frequency": {
			previous:  timer(1000, 10, 1000),
			current:   timer(2001000, 110, 2001000),
			frequency: frequency(0),
		},
		"missing value": {
			previous: averageTimer{
				ticks:      valid(1000),
				operations: valid(10),
			},
			current:   timer(2001000, 110, 2001000),
			frequency: frequency(tenMHz),
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, averageMicroseconds(tc.previous, tc.current, tc.frequency))
		})
	}
}

func TestAveragePerOperation(t *testing.T) {
	bulk := func(total, operations uint64) averageBulk {
		return averageBulk{
			total: sample{
				value: total,
				valid: true,
			},
			operations: sample{
				value: operations,
				valid: true,
			},
		}
	}
	for name, tc := range map[string]struct {
		previous, current averageBulk
		want              sample
	}{
		"whole bytes per operation": {
			previous: bulk(1000, 10),
			current:  bulk(1000+3*4097, 13),
			want: sample{
				value: 4097,
				valid: true,
			},
		},
		"operation count wrapped": {
			previous: bulk(0, counter32Range-1),
			current:  bulk(8192, 1),
			want: sample{
				value: 4096,
				valid: true,
			},
		},
		"no operation": {
			previous: bulk(1000, 10),
			current:  bulk(1000, 10),
			want: sample{
				value: 0,
				valid: true,
			},
		},
		"bytes without operations": {
			previous: bulk(1000, 10),
			current:  bulk(2000, 10),
		},
		"total went back": {
			previous: bulk(2000, 10),
			current:  bulk(1000, 20),
		},
		"missing value": {
			previous: averageBulk{
				total: sample{
					value: 1000,
					valid: true,
				},
			},
			current: bulk(2000, 20),
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, averagePerOperation(tc.previous, tc.current))
		})
	}
}

func TestUtilizationPercent(t *testing.T) {
	idle := func(idle, stamp uint64) [2]sample {
		return [2]sample{{value: idle, valid: true}, {value: stamp, valid: true}}
	}
	for name, tc := range map[string]struct {
		previous, current [2]sample
		want              sample
	}{
		// 99.19% idle is 99 in whole percent, so 1% busy.
		"partly idle": {
			previous: idle(14123705624035, 134402530954054165),
			current:  idle(14123727471821, 134402530976080383),
			want: sample{
				value: 1,
				valid: true,
			},
		},
		"busy": {
			previous: idle(500, 1000),
			current:  idle(500, 2000),
			want: sample{
				value: 100,
				valid: true,
			},
		},
		// Idle time can run slightly ahead of its timestamp.
		"idle beyond the interval": {
			previous: idle(500, 1000),
			current:  idle(1600, 2000),
			want: sample{
				value: 0,
				valid: true,
			},
		},
		"same timestamp": {
			previous: idle(500, 1000),
			current:  idle(500, 1000),
		},
		"idle time went back": {
			previous: idle(500, 1000),
			current:  idle(400, 2000),
		},
		"no previous timestamp": {
			previous: idle(0, 0),
			current:  idle(400, 2000),
		},
		"missing value": {
			previous: [2]sample{{value: 500, valid: true}},
			current:  idle(600, 2000),
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, utilizationPercent(tc.previous, tc.current))
		})
	}
}

func TestReadPhysicalDisks(t *testing.T) {
	first, _ := labDiskRows()
	for name, tc := range map[string]struct {
		rows        queryResult
		wantDevices []string
		wantMounts  []string
		wantTotal   [2]sample
		wantUsable  bool
	}{
		// WinRM reports numbers as text.
		"disks and total": {
			rows: fixtureRows(
				first,
				map[string]any{"Name": "1", "DiskReadsPersec": "7"},
				map[string]any{"Name": "2 E: F:", "DiskReadsPersec": uint32(3)},
				map[string]any{"Name": "_Total", "DiskReadBytesPersec": "4096", "DiskWriteBytesPersec": uint64(0)},
			),
			wantDevices: []string{"Disk 0", "Disk 1", "Disk 2"},
			wantMounts:  []string{"C:", "", "E: F:"},
			wantTotal:   [2]sample{{value: 4096, valid: true}, {value: 0, valid: true}},
			wantUsable:  true,
		},
		"name that is not a disk number": {
			rows:        fixtureRows(map[string]any{"Name": "HarddiskVolume3 G:", "DiskReadsPersec": uint32(3)}),
			wantDevices: []string{"HarddiskVolume3"},
			wantMounts:  []string{"G:"},
			wantUsable:  true,
		},
		"duplicate disk": {
			rows: fixtureRows(
				map[string]any{"Name": "0 C:", "DiskReadsPersec": uint32(3)},
				map[string]any{"Name": "0 D:", "DiskReadsPersec": uint32(4)},
			),
			wantDevices: []string{"Disk 0"},
			wantMounts:  []string{"C:"},
			wantUsable:  true,
		},
		"instances without values": {
			rows: fixtureRows(
				map[string]any{"Name": "0 C:", "DiskReadsPersec": "malformed"},
				map[string]any{"Name": "", "DiskReadsPersec": uint32(3)},
			),
		},
		"no disks": {
			rows:       fixtureRows(),
			wantUsable: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readPhysicalDisks(tc.rows.rows))
			var devices, mounts []string
			for _, disk := range o.physicalDisks {
				devices = append(devices, disk.device)
				mounts = append(mounts, disk.mountPoint)
			}
			assert.Equal(t, tc.wantDevices, devices)
			assert.Equal(t, tc.wantMounts, mounts)
			assert.Equal(t, tc.wantTotal, [2]sample{o.diskTotalRead, o.diskTotalWritten})
		})
	}
}

// TestCollector_DiskDriveLookup reads a disk's model and device ID when the
// disk first appears, retries after a failed lookup, and looks up again a disk
// that disappeared and came back.
func TestCollector_DiskDriveLookup(t *testing.T) {
	disk0 := map[string]any{"Name": "0 C:", "DiskReadsPersec": uint32(3)}
	disk1 := map[string]any{"Name": "1", "DiskReadsPersec": uint32(4)}
	drives := fixtureRows(
		map[string]any{"Index": uint32(0), "Model": "Disk Model A", "DeviceID": `\\.\PHYSICALDRIVE0`},
		map[string]any{"Index": "5", "Model": "Unrelated", "DeviceID": `\\.\PHYSICALDRIVE5`},
	)
	polls := []struct {
		disks      queryResult
		drives     queryResult
		wantLookup bool
		wantReads  map[string]metrix.SampleValue
	}{
		{
			disks: fixtureRows(disk0),
			drives: queryResult{
				err: errors.New("refused"),
			},
			wantLookup: true,
			wantReads:  map[string]metrix.SampleValue{`{device="Disk 0",mount_point="C:"}`: 3},
		},
		{
			disks:      fixtureRows(disk0),
			drives:     drives,
			wantLookup: true,
			wantReads: map[string]metrix.SampleValue{
				`{device="Disk 0",device_id="\\\\.\\PHYSICALDRIVE0",model="Disk Model A",mount_point="C:"}`: 3,
			},
		},
		// Disk 1 is not among the disk drives: it is looked up once.
		{
			disks:      fixtureRows(disk0, disk1),
			drives:     drives,
			wantLookup: true,
			wantReads: map[string]metrix.SampleValue{
				`{device="Disk 0",device_id="\\\\.\\PHYSICALDRIVE0",model="Disk Model A",mount_point="C:"}`: 3,
				`{device="Disk 1"}`: 4,
			},
		},
		{
			disks: fixtureRows(disk1),
			wantReads: map[string]metrix.SampleValue{
				`{device="Disk 1"}`: 4,
			},
		},
		{
			disks:      fixtureRows(disk0, disk1),
			drives:     drives,
			wantLookup: true,
			wantReads: map[string]metrix.SampleValue{
				`{device="Disk 0",device_id="\\\\.\\PHYSICALDRIVE0",model="Disk Model A",mount_point="C:"}`: 3,
				`{device="Disk 1"}`: 4,
			},
		},
	}
	c := configuredCollector()
	c.Collectors = Collectors{
		CPU:     new(false),
		Memory:  new(false),
		System:  new(false),
		Network: new(false),
		SMB:     new(false),
		Time:    new(false),
	}
	var current *fakeSession
	c.newSession = func(context.Context) (querySession, error) { return current, nil }
	require.NoError(t, c.Init(t.Context()))
	for i, poll := range polls {
		current = &fakeSession{
			results: map[string]queryResult{
				queryDisk:          fixtureRows(),
				queryPhysicalDisks: poll.disks,
				queryDiskDrives:    poll.drives,
			},
		}
		metrics, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		reads := make(map[string]metrix.SampleValue)
		for series, value := range metrics {
			if labels, ok := strings.CutPrefix(series, "disk_reads{"); ok {
				reads["{"+labels] = value
			}
		}
		assert.Equal(t, poll.wantReads, reads, "poll %d", i)
		wantQueries := []string{queryDisk, queryPhysicalDisks}
		if poll.wantLookup {
			wantQueries = append(wantQueries, queryDiskDrives)
		}
		assert.Equal(t, wantQueries, metricQueries(current.queries), "poll %d", i)
	}
}
