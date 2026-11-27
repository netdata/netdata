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
// booted 86401.2345678 seconds before the sample, taken at
// 2022-06-19 04:26:41.2345678 UTC by its clock; times are 100 ns ticks.
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

// timeServiceRow is the Windows Time Service counter set's row as DCOM decodes
// it: the offset as uint64, the round trip and the source count as uint32,
// both times in microseconds. Every value is distinct.
func timeServiceRow() map[string]any {
	return map[string]any{
		"ComputedTimeOffset":       uint64(48271),
		"NTPRoundtripDelay":        uint32(56702),
		"NTPClientTimeSourceCount": uint32(2),
	}
}

// terminalServicesRow is the Terminal Services counter set's row as DCOM
// decodes it: every counter is uint32. Every value is distinct.
func terminalServicesRow() map[string]any {
	return map[string]any{
		"ActiveSessions":   uint32(4),
		"InactiveSessions": uint32(7),
	}
}

// smbServerRow is the Server counter set's row as DCOM decodes it: byte totals
// as uint64, the other counters as uint32. Every value is distinct.
func smbServerRow() map[string]any {
	return map[string]any{
		"BytesReceivedPersec":      uint64(3288867940),
		"BytesTransmittedPersec":   uint64(119347308),
		"ServerSessions":           uint32(3),
		"SessionsLoggedOff":        uint32(8),
		"SessionsTimedOut":         uint32(11),
		"SessionsErroredOut":       uint32(2),
		"SessionsForcedOff":        uint32(1),
		"LogonPersec":              uint32(18),
		"ErrorsLogon":              uint32(4),
		"ErrorsAccessPermissions":  uint32(5),
		"ErrorsGrantedAccess":      uint32(6),
		"FilesOpen":                uint32(7),
		"FilesOpenedTotal":         uint32(20764),
		"ErrorsSystem":             uint32(9),
		"WorkItemShortages":        uint32(10),
		"BlockingRequestsRejected": uint32(12),
		"PoolPagedFailures":        uint32(13),
		"PoolNonpagedFailures":     uint32(14),
	}
}

// smbShareRow is an SMB Server Shares instance as DCOM decodes it: the read
// and write requests as uint32, the other counters as uint64. Every value is
// distinct.
func smbShareRow(name string) map[string]any {
	return map[string]any{
		"Name":                   name,
		"CurrentOpenFileCount":   uint64(15),
		"TreeConnectCount":       uint64(16),
		"ReadRequestsPersec":     uint32(394),
		"WriteRequestsPersec":    uint32(3072),
		"MetadataRequestsPersec": uint64(366),
		"FilesOpenedPersec":      uint64(57),
		"ReceivedBytesPersec":    uint64(3221619890),
		"SentBytesPersec":        uint64(332094),
	}
}

// memoryCountersRow is a Memory counter set instance as DCOM decodes it: byte
// counts as uint64, per-second counters and counts as uint32. Every value is
// distinct; Pages/sec is the sum of the pages read and written.
func memoryCountersRow() map[string]any {
	return map[string]any{
		"PageFaultsPersec":                uint32(575466442),
		"PagesPersec":                     uint32(2304136),
		"AvailableBytes":                  uint64(10437402624),
		"PageReadsPersec":                 uint32(291996),
		"PageWritesPersec":                uint32(5),
		"PagesInputPersec":                uint32(2304130),
		"PagesOutputPersec":               uint32(6),
		"CommittedBytes":                  uint64(2267668480),
		"CommitLimit":                     uint64(14808518656),
		"CacheBytes":                      uint64(85794816),
		"CacheBytesPeak":                  uint64(235356160),
		"SystemCacheResidentBytes":        uint64(81920),
		"FreeAndZeroPageListBytes":        uint64(4500127744),
		"ModifiedPageListBytes":           uint64(191234048),
		"StandbyCacheCoreBytes":           uint64(4096),
		"StandbyCacheNormalPriorityBytes": uint64(2746179584),
		"StandbyCacheReserveBytes":        uint64(3191095296),
		"PoolPagedBytes":                  uint64(346816512),
		"PoolNonpagedBytes":               uint64(87597056),
		"PoolPagedResidentBytes":          uint64(335323136),
		"PoolPagedAllocs":                 uint32(423244),
		"PoolNonpagedAllocs":              uint32(221739),
		"FreeSystemPageTableEntries":      uint32(12302931),
		"SystemCodeResidentBytes":         uint64(8192),
		"SystemCodeTotalBytes":            uint64(12288),
		"SystemDriverResidentBytes":       uint64(29765632),
		"SystemDriverTotalBytes":          uint64(16011264),
		"CacheFaultsPersec":               uint32(49373616),
		"DemandZeroFaultsPersec":          uint32(199225414),
		"TransitionFaultsPersec":          uint32(348256032),
		"WriteCopiesPersec":               uint32(2372610),
		"TransitionPagesRePurposedPersec": uint32(3),
	}
}

// physicalDiskRow is a PhysicalDisk instance as DCOM decodes it: byte counts,
// time integrals and the idle time with its timestamp as uint64, operation
// counts, average timers and their bases as uint32. Each average timer is the
// low 32 bits of its queue-length integral, as Windows reports it.
func physicalDiskRow(name string) map[string]any {
	return map[string]any{
		"Name":                       name,
		"DiskReadBytesPersec":        uint64(8555499008),
		"DiskWriteBytesPersec":       uint64(43681006080),
		"DiskReadsPersec":            uint32(218802),
		"DiskWritesPersec":           uint32(2805931),
		"SplitIOPerSec":              uint32(160491),
		"CurrentDiskQueueLength":     uint32(2),
		"PercentDiskTime":            uint64(111251464098),
		"PercentDiskReadTime":        uint64(6537594026),
		"PercentDiskWriteTime":       uint64(104713870072),
		"PercentIdleTime":            uint64(40333877810720),
		"PercentIdleTime_Base":       uint64(134402530853236929),
		"AvgDisksecPerRead":          uint32(2242626730),
		"AvgDisksecPerRead_Base":     uint32(218802),
		"AvgDiskReadQueueLength":     uint64(6537594026),
		"AvgDisksecPerWrite":         uint32(1634654968),
		"AvgDisksecPerWrite_Base":    uint32(2805931),
		"AvgDiskWriteQueueLength":    uint64(104713870072),
		"AvgDisksecPerTransfer":      uint32(3877281698),
		"AvgDisksecPerTransfer_Base": uint32(3024733),
		"AvgDiskQueueLength":         uint64(111251464098),
		"AvgDiskBytesPerRead":        uint64(8555499008),
		"AvgDiskBytesPerRead_Base":   uint32(218802),
		"AvgDiskBytesPerWrite":       uint64(43681006080),
		"AvgDiskBytesPerWrite_Base":  uint32(2805931),
		"Frequency_PerfTime":         uint64(10000000),
	}
}

// Label sets of the physical disks of validQueryResults.
const (
	disk0Labels = `{device="Disk 0",device_id="\\\\.\\PHYSICALDRIVE0",model="QEMU HARDDISK",mount_point="C:"}`
	disk1Labels = `{device="Disk 1",device_id="\\\\.\\PHYSICALDRIVE1",model="Virtual Disk"}`
)

// withDiskCounters adds the raw counter metrics of the physical disks and the
// disk totals of validQueryResults to a collection's metrics; times arrive in
// whole milliseconds.
func withDiskCounters(metrics map[string]metrix.SampleValue) map[string]metrix.SampleValue {
	for _, labels := range []string{disk0Labels, disk1Labels} {
		for metric, value := range map[string]metrix.SampleValue{
			"disk_read_bytes":              8555499008,
			"disk_written_bytes":           43681006080,
			"disk_reads":                   218802,
			"disk_writes":                  2805931,
			"disk_split_io":                160491,
			"disk_queue_length":            2,
			"disk_busy_milliseconds":       11125146,
			"disk_read_time_milliseconds":  653759,
			"disk_write_time_milliseconds": 10471387,
		} {
			metrics[metric+labels] = value
		}
	}
	metrics["disk_total_read_bytes"] = 2 * 8555499008
	metrics["disk_total_written_bytes"] = 2 * 43681006080
	return metrics
}

// withProtocolCounters adds the TCP and UDP metrics of validQueryResults to a
// collection's metrics: TCP segments per IP version, every TCP counter for
// both versions summed, and every UDP counter per version.
func withProtocolCounters(metrics map[string]metrix.SampleValue) map[string]metrix.SampleValue {
	for metric, value := range map[string]metrix.SampleValue{
		"ipv4_tcp_segments_received":     1130775,
		"ipv4_tcp_segments_sent":         1406820,
		"ipv6_tcp_segments_received":     1572,
		"ipv6_tcp_segments_sent":         1599,
		"tcp_segments_received":          1130775 + 1572,
		"tcp_segments_sent":              1406820 + 1599,
		"tcp_established_connections":    6,
		"tcp_active_opens":               5194 + 76,
		"tcp_passive_opens":              3548 + 49,
		"tcp_established_resets":         168,
		"tcp_failed_connection_attempts": 208 + 74,
		"tcp_retransmitted_segments":     6689 + 32,
		"ipv4_udp_datagrams_received":    187524,
		"ipv4_udp_datagrams_sent":        203100,
		"ipv4_udp_receive_errors":        0,
		"ipv4_udp_no_port_datagrams":     86178,
		"ipv6_udp_datagrams_received":    1745,
		"ipv6_udp_datagrams_sent":        1411,
		"ipv6_udp_receive_errors":        0,
		"ipv6_udp_no_port_datagrams":     822,
	} {
		metrics[metric] = value
	}
	return metrics
}

// Label sets of the shares of validQueryResults.
const (
	shareC   = `{share="\\\\*\\C$"}`
	shareIPC = `{share="\\\\*\\IPC$"}`
)

// withSMBCounters adds the metrics of smbServerRow and of the shares of
// validQueryResults to a collection's metrics.
func withSMBCounters(metrics map[string]metrix.SampleValue) map[string]metrix.SampleValue {
	for metric, value := range map[string]metrix.SampleValue{
		"smb_server_received_bytes":              3288867940,
		"smb_server_sent_bytes":                  119347308,
		"smb_server_sessions":                    3,
		"smb_server_sessions_logged_off":         8,
		"smb_server_sessions_timed_out":          11,
		"smb_server_sessions_errored_out":        2,
		"smb_server_sessions_forced_off":         1,
		"smb_server_logons":                      18,
		"smb_server_logon_errors":                4,
		"smb_server_access_permission_errors":    5,
		"smb_server_granted_access_errors":       6,
		"smb_server_open_files":                  7,
		"smb_server_files_opened":                20764,
		"smb_server_system_errors":               9,
		"smb_server_work_item_shortages":         10,
		"smb_server_blocking_requests_rejected":  12,
		"smb_server_paged_pool_failures":         13,
		"smb_server_nonpaged_pool_failures":      14,
		"smb_share_open_files" + shareC:          15,
		"smb_share_tree_connects" + shareC:       16,
		"smb_share_read_requests" + shareC:       394,
		"smb_share_write_requests" + shareC:      3072,
		"smb_share_metadata_requests" + shareC:   366,
		"smb_share_files_opened" + shareC:        57,
		"smb_share_received_bytes" + shareC:      3221619890,
		"smb_share_sent_bytes" + shareC:          332094,
		"smb_share_open_files" + shareIPC:        0,
		"smb_share_tree_connects" + shareIPC:     2,
		"smb_share_read_requests" + shareIPC:     0,
		"smb_share_write_requests" + shareIPC:    0,
		"smb_share_metadata_requests" + shareIPC: 13,
		"smb_share_files_opened" + shareIPC:      0,
		"smb_share_received_bytes" + shareIPC:    1494,
		"smb_share_sent_bytes" + shareIPC:        1392,
	} {
		metrics[metric] = value
	}
	return metrics
}

// withTimeServiceCounters adds the metrics of timeServiceRow to a collection's
// metrics.
func withTimeServiceCounters(metrics map[string]metrix.SampleValue) map[string]metrix.SampleValue {
	metrics["time_service_offset_microseconds"] = 48271
	metrics["time_service_ntp_roundtrip_delay_microseconds"] = 56702
	metrics["time_service_ntp_sources"] = 2
	return metrics
}

// withTerminalServicesCounters adds the metrics of terminalServicesRow to a
// collection's metrics.
func withTerminalServicesCounters(metrics map[string]metrix.SampleValue) map[string]metrix.SampleValue {
	metrics["terminal_services_active_sessions"] = 4
	metrics["terminal_services_inactive_sessions"] = 7
	return metrics
}

// withMemoryCounters adds the metrics of memoryCountersRow and of the paging
// files and NUMA nodes of validQueryResults to a collection's metrics.
func withMemoryCounters(metrics map[string]metrix.SampleValue) map[string]metrix.SampleValue {
	for metric, value := range map[string]metrix.SampleValue{
		"memory_page_faults_minor":                    573162306,
		"memory_page_faults_major":                    2304136,
		"memory_available_bytes":                      10437402624,
		"memory_page_reads":                           291996,
		"memory_page_writes":                          5,
		"memory_pages_input":                          2304130,
		"memory_pages_output":                         6,
		"memory_committed_bytes":                      2267668480,
		"memory_commit_limit_bytes":                   14808518656,
		"memory_system_cache_bytes":                   85794816,
		"memory_system_cache_peak_bytes":              235356160,
		"memory_system_cache_resident_bytes":          81920,
		"memory_free_and_zero_page_list_bytes":        4500127744,
		"memory_modified_page_list_bytes":             191234048,
		"memory_standby_cache_core_bytes":             4096,
		"memory_standby_cache_normal_priority_bytes":  2746179584,
		"memory_standby_cache_reserve_bytes":          3191095296,
		"memory_pool_paged_bytes":                     346816512,
		"memory_pool_nonpaged_bytes":                  87597056,
		"memory_pool_paged_resident_bytes":            335323136,
		"memory_pool_paged_allocations":               423244,
		"memory_pool_nonpaged_allocations":            221739,
		"memory_free_system_page_table_entries":       12302931,
		"memory_system_code_resident_bytes":           8192,
		"memory_system_code_total_bytes":              12288,
		"memory_system_driver_resident_bytes":         29765632,
		"memory_system_driver_total_bytes":            16011264,
		"memory_cache_faults":                         49373616,
		"memory_demand_zero_faults":                   199225414,
		"memory_transition_faults":                    348256032,
		"memory_write_copy_faults":                    2372610,
		"memory_transition_pages_repurposed":          3,
		`numa_node_free_and_zero_mebibytes{node="0"}`: 4304,
		`numa_node_standby_mebibytes{node="0"}`:       5662,
		`numa_node_free_and_zero_mebibytes{node="1"}`: 1157,
		`numa_node_standby_mebibytes{node="1"}`:       6598,
		// 88040 of 475136 and 1024 of 262144 pages of 4 KiB in use.
		`pagefile_used_bytes{pagefile="C:\\pagefile.sys"}`: 360611840,
		`pagefile_free_bytes{pagefile="C:\\pagefile.sys"}`: 1585545216,
		`pagefile_used_bytes{pagefile="D:\\pagefile.sys"}`: 4194304,
		`pagefile_free_bytes{pagefile="D:\\pagefile.sys"}`: 1069547520,
	} {
		metrics[metric] = value
	}
	return metrics
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
			"TotalVirtualMemorySize": uint64(12582912),
			"FreeVirtualMemory":      "9437184",
		}),
		queryMemoryCounters: fixtureRows(memoryCountersRow()),
		// Paging files carry the NT prefix; WinRM reports the numbers as text.
		queryPagingFiles: fixtureRows(
			map[string]any{
				"Name":              `\??\C:\pagefile.sys`,
				"PercentUsage":      uint32(88040),
				"PercentUsage_Base": uint32(475136),
			},
			map[string]any{"Name": `\??\D:\pagefile.sys`, "PercentUsage": "1024", "PercentUsage_Base": "262144"},
			map[string]any{"Name": "_Total", "PercentUsage": uint32(89064), "PercentUsage_Base": uint32(737280)},
		),
		queryNUMANodes: fixtureRows(
			map[string]any{"Name": "0", "FreeAndZeroPageListMBytes": uint32(4304), "StandbyListMBytes": uint32(5662)},
			map[string]any{"Name": "1", "FreeAndZeroPageListMBytes": "1157", "StandbyListMBytes": "6598"},
			map[string]any{
				"Name":                      "_Total",
				"FreeAndZeroPageListMBytes": uint32(5461),
				"StandbyListMBytes":         uint32(12260),
			},
		),
		querySystem:           fixtureRows(systemRow()),
		queryTerminalServices: fixtureRows(terminalServicesRow()),
		queryTimeService:      fixtureRows(timeServiceRow()),
		queryNetworkInterfaces: fixtureRows(
			map[string]any{"Name": "Ethernet", "BytesReceivedPersec": uint64(190), "BytesSentPersec": uint64(30)},
			map[string]any{"Name": "Ethernet#1", "BytesReceivedPersec": uint64(0), "BytesSentPersec": "70"},
			map[string]any{"Name": "isatap.example.com", "BytesReceivedPersec": uint64(0), "BytesSentPersec": "0"},
		),
		// The TCP and UDP counter sets have one row per IP version; WinRM
		// reports the numbers as text.
		queryTCPv4: fixtureRows(map[string]any{
			"SegmentsReceivedPersec":      uint32(1130775),
			"SegmentsSentPersec":          uint32(1406820),
			"ConnectionsEstablished":      uint32(6),
			"ConnectionsActive":           uint32(5194),
			"ConnectionsPassive":          uint32(3548),
			"ConnectionsReset":            uint32(168),
			"ConnectionFailures":          uint32(208),
			"SegmentsRetransmittedPersec": uint32(6689),
		}),
		queryTCPv6: fixtureRows(map[string]any{
			"SegmentsReceivedPersec":      uint32(1572),
			"SegmentsSentPersec":          uint32(1599),
			"ConnectionsEstablished":      uint32(0),
			"ConnectionsActive":           uint32(76),
			"ConnectionsPassive":          uint32(49),
			"ConnectionsReset":            uint32(0),
			"ConnectionFailures":          uint32(74),
			"SegmentsRetransmittedPersec": uint32(32),
		}),
		queryUDPv4: fixtureRows(map[string]any{
			"DatagramsReceivedPersec": uint32(187524),
			"DatagramsSentPersec":     uint32(203100),
			"DatagramsReceivedErrors": uint32(0),
			"DatagramsNoPortPersec":   uint32(86178),
		}),
		queryUDPv6: fixtureRows(map[string]any{
			"DatagramsReceivedPersec": "1745",
			"DatagramsSentPersec":     "1411",
			"DatagramsReceivedErrors": "0",
			"DatagramsNoPortPersec":   "822",
		}),
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
		// Disk 1 has no volume with a drive letter.
		queryPhysicalDisks: fixtureRows(
			physicalDiskRow("0 C:"),
			physicalDiskRow("1"),
			map[string]any{
				"Name":                 "_Total",
				"DiskReadBytesPersec":  uint64(2 * 8555499008),
				"DiskWriteBytesPersec": uint64(2 * 43681006080),
			},
		),
		queryDiskDrives: fixtureRows(
			map[string]any{"Index": uint32(1), "Model": "Virtual Disk", "DeviceID": `\\.\PHYSICALDRIVE1`},
			map[string]any{"Index": uint32(0), "Model": "QEMU HARDDISK", "DeviceID": `\\.\PHYSICALDRIVE0`},
		),
		querySMBServer: fixtureRows(smbServerRow()),
		// Shares under the counter set's instance names; WinRM reports the
		// numbers as text. _Total is not a share.
		querySMBShares: fixtureRows(
			smbShareRow(`\\*\C$`),
			map[string]any{
				"Name":                   `\\*\IPC$`,
				"CurrentOpenFileCount":   "0",
				"TreeConnectCount":       "2",
				"ReadRequestsPersec":     "0",
				"WriteRequestsPersec":    "0",
				"MetadataRequestsPersec": "13",
				"FilesOpenedPersec":      "0",
				"ReceivedBytesPersec":    "1494",
				"SentBytesPersec":        "1392",
			},
			map[string]any{"Name": "_Total", "CurrentOpenFileCount": uint64(15), "TreeConnectCount": uint64(18)},
		),
	}
}

// fixturePollerTime is the poller's clock in the fixture collections: 1.5 s
// behind the host's clock in systemRow. It stands still, so every query takes
// no time.
var fixturePollerTime = time.Date(2022, 6, 19, 4, 26, 39, 734567800, time.UTC)

func configuredCollector() *Collector {
	c := New()
	c.now = func() time.Time { return fixturePollerTime }
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
	// WinRM text, a malformed value, more pages than faults and a valid zero.
	sessions[1].results[queryMemoryCounters] = fixtureRows(map[string]any{
		"AvailableBytes":   "6596587520",
		"CommittedBytes":   "malformed",
		"PageFaultsPersec": "100",
		"PagesPersec":      "120",
		"PoolPagedAllocs":  uint32(0),
	})
	sessions[1].results[queryPagingFiles] = queryResult{
		err: errors.New("invalid class"),
	}
	sessions[1].results[queryNUMANodes] = fixtureRows(map[string]any{
		"Name":                      "0",
		"FreeAndZeroPageListMBytes": "malformed",
		"StandbyListMBytes":         "0",
	})
	// WinRM text, a 32-bit counter that wrapped, a valid zero and no uptime.
	sessions[1].results[querySystem] = fixtureRows(map[string]any{
		"Processes":             "70",
		"Threads":               "malformed",
		"ContextSwitchesPersec": "200",
		"ProcessorQueueLength":  "0",
	})
	// WinRM text, a valid zero and a missing counter.
	sessions[1].results[queryTerminalServices] = fixtureRows(map[string]any{"ActiveSessions": "0"})
	// WinRM text, a valid zero, a malformed value and a missing counter.
	sessions[1].results[queryTimeService] = fixtureRows(map[string]any{
		"ComputedTimeOffset":       "0",
		"NTPClientTimeSourceCount": "malformed",
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
	// Without the IPv6 counters there are no sums for both versions; a reply
	// that is not one row leaves the UDPv4 charts without values.
	sessions[1].results[queryTCPv4] = fixtureRows(map[string]any{
		"SegmentsReceivedPersec": "1130900",
		"SegmentsSentPersec":     "malformed",
		"ConnectionsEstablished": "0",
	})
	sessions[1].results[queryTCPv6] = queryResult{
		err: errors.New("invalid class"),
	}
	sessions[1].results[queryUDPv4] = fixtureRows(
		map[string]any{"DatagramsReceivedPersec": uint32(1)},
		map[string]any{"DatagramsReceivedPersec": uint32(2)},
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
	sessions[1].results[queryPhysicalDisks] = queryResult{
		err: errors.New("invalid class"),
	}
	// WinRM text, a malformed value and missing counters; a share without a
	// usable value and an unnamed instance are left out.
	sessions[1].results[querySMBServer] = fixtureRows(map[string]any{
		"BytesReceivedPersec": "3288900000",
		"ServerSessions":      "0",
		"ErrorsLogon":         "malformed",
	})
	sessions[1].results[querySMBShares] = fixtureRows(
		map[string]any{"Name": `\\*\C$`, "CurrentOpenFileCount": "0", "ReadRequestsPersec": "malformed",
			"SentBytesPersec": "332500"},
		map[string]any{"Name": `\\*\ADMIN$`, "TreeConnectCount": "malformed"},
		map[string]any{"TreeConnectCount": uint64(1)},
		map[string]any{"Name": "_Total", "TreeConnectCount": uint64(1)},
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
	assert.Equal(
		t,
		withTerminalServicesCounters(withTimeServiceCounters(withSMBCounters(
			withProtocolCounters(withDiskCounters(withMemoryCounters(withAnsweredPing(map[string]metrix.SampleValue{
				"memory_used_bytes":                  6 * 1024 * 1024 * 1024,
				"memory_free_bytes":                  2 * 1024 * 1024 * 1024,
				"memory_swap_used_bytes":             3 * 1024 * 1024 * 1024,
				"memory_swap_free_bytes":             9 * 1024 * 1024 * 1024,
				"system_processes":                   69,
				"system_threads":                     951,
				"system_context_switches":            4294967000,
				"system_processor_queue_length":      3,
				"system_uptime_milliseconds":         86401234,
				"system_clock_offset_microseconds":   1500000,
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
			})))),
		))),
		first,
	)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	second, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, withAnsweredPing(map[string]metrix.SampleValue{
		"memory_free_bytes":                     0,
		"memory_available_bytes":                6596587520,
		"memory_page_faults_minor":              0,
		"memory_page_faults_major":              120,
		"memory_pool_paged_allocations":         0,
		`numa_node_standby_mebibytes{node="0"}`: 0,
		"system_processes":                      70,
		"system_context_switches":               200,
		"system_processor_queue_length":         0,
		"time_service_offset_microseconds":      0,
		"network_received_bytes" + ethernet:     250,
		"network_total_received_bytes":          250,
		`network_received_bytes{device="Ethernet#1",interface_type="virtual"}`: 0,
		`network_sent_bytes{device="Ethernet#1",interface_type="virtual"}`:     80,
		"ipv4_tcp_segments_received":                                           1130900,
		"ipv6_udp_datagrams_received":                                          1745,
		"ipv6_udp_datagrams_sent":                                              1411,
		"ipv6_udp_receive_errors":                                              0,
		"ipv6_udp_no_port_datagrams":                                           822,
		"disk_used_bytes" + driveC:                                             0,
		"disk_avail_bytes" + driveC:                                            300,
		"smb_server_received_bytes":                                            3288900000,
		"smb_server_sessions":                                                  0,
		"smb_share_open_files" + shareC:                                        0,
		"smb_share_sent_bytes" + shareC:                                        332500,
		"terminal_services_active_sessions":                                    0,
	}), second)

	// The failed CPU query keeps the first sample, so the shares span both
	// intervals. The failed physical disk query leaves the disks without a
	// previous sample, so they have no interval values yet.
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

	// The disks' model and device ID are read once, when they first appear.
	for i, session := range sessions {
		want := []string{
			queryCPU,
			queryMemory,
			queryMemoryCounters,
			queryPagingFiles,
			queryNUMANodes,
			querySystem,
			queryTerminalServices,
			queryTimeService,
			queryNetworkInterfaces,
			queryNetworkAdapters,
			queryTCPv4,
			queryTCPv6,
			queryUDPv4,
			queryUDPv6,
			queryDisk,
			queryPhysicalDisks,
			querySMBServer,
			querySMBShares,
		}
		if i == 0 {
			want = append(want, queryDiskDrives)
		}
		assert.Equal(t, want, metricQueries(session.queries))
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
			wantQueries: []string{queryNetworkInterfaces, queryNetworkAdapters, queryTCPv4, queryTCPv6, queryUDPv4, queryUDPv6},
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
			wantQueries: []string{queryNetworkInterfaces, queryTCPv4, queryTCPv6, queryUDPv4, queryUDPv6},
			wantMetrics: withAnsweredPing(map[string]metrix.SampleValue{}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			c.Collectors.CPU = new(false)
			c.Collectors.Memory = new(false)
			c.Collectors.System = new(false)
			c.Collectors.Disk = new(false)
			c.Collectors.SMB = new(false)
			c.Collectors.Time = new(false)
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
			c.Collectors.SMB = new(false)
			c.Collectors.Time = new(false)
			session := &fakeSession{
				results: map[string]queryResult{
					queryCPU:            {err: tc.queryErr},
					queryMemory:         {err: tc.queryErr},
					queryMemoryCounters: {err: tc.queryErr},
					// A paging file without a size and a node without values give no sample.
					queryPagingFiles: {
						rows: fixtureRows(map[string]any{"Name": `\??\C:\pagefile.sys`}).rows,
						err:  tc.queryErr,
					},
					queryNUMANodes: {
						rows: fixtureRows(map[string]any{"Name": "0"}).rows,
						err:  tc.queryErr,
					},
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
	// WMI's own alerts on the host's charts. The clock offset alert converts
	// the chart's milliseconds to seconds, so the documentation check covers
	// the time sync alert only.
	alerts, err := os.ReadFile("../../../../../health/health.d/wmi.plugin.conf")
	require.NoError(t, err)
	collecttest.AssertHealthAlertsTargetChartTemplateWith(t, alerts, chartTemplateYAML, collecttest.HealthAlertsCheck{
		ContextPrefix: "wmi.",
	})
	collecttest.AssertHealthAlertsMatchMetadataWith(t, alerts, metadata, collecttest.HealthAlertsCheck{
		ContextPrefix: "wmi.clock_sync_",
		ChartTemplate: chartTemplateYAML,
	})
}

func metricQueries(queries []string) []string {
	var metrics []string
	for _, query := range queries {
		switch query {
		case queryCPU,
			queryMemory,
			queryMemoryCounters,
			queryPagingFiles,
			queryNUMANodes,
			querySystem,
			queryTerminalServices,
			queryTimeService,
			queryNetworkInterfaces,
			queryNetworkAdapters,
			queryTCPv4,
			queryTCPv6,
			queryUDPv4,
			queryUDPv6,
			queryDisk,
			queryPhysicalDisks,
			queryDiskDrives,
			querySMBServer,
			querySMBShares:
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
				queryMemoryCounters:    lost,
				queryPagingFiles:       lost,
				queryNUMANodes:         lost,
				querySystem:            lost,
				queryTerminalServices:  lost,
				queryTimeService:       lost,
				queryNetworkInterfaces: lost,
				queryTCPv4:             lost,
				queryTCPv6:             lost,
				queryUDPv4:             lost,
				queryUDPv6:             lost,
				queryDisk:              lost,
				queryPhysicalDisks:     lost,
				querySMBServer:         lost,
				querySMBShares:         lost,
			},
			wantMetrics: withAnsweredPing(map[string]metrix.SampleValue{
				"memory_used_bytes":      6 * 1024 * 1024 * 1024,
				"memory_free_bytes":      2 * 1024 * 1024 * 1024,
				"memory_swap_used_bytes": 3 * 1024 * 1024 * 1024,
				"memory_swap_free_bytes": 9 * 1024 * 1024 * 1024,
			}),
		},
		"after a refused query, report the query failures": {
			results: map[string]queryResult{
				queryCPU:               {err: errors.New("WS-Management fault")},
				queryMemory:            lost,
				queryMemoryCounters:    lost,
				queryPagingFiles:       lost,
				queryNUMANodes:         lost,
				querySystem:            lost,
				queryTerminalServices:  lost,
				queryTimeService:       lost,
				queryNetworkInterfaces: lost,
				queryTCPv4:             lost,
				queryTCPv6:             lost,
				queryUDPv4:             lost,
				queryUDPv6:             lost,
				queryDisk:              lost,
				queryPhysicalDisks:     lost,
				querySMBServer:         lost,
				querySMBShares:         lost,
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
				[]string{
					queryCPU,
					queryMemory,
					queryMemoryCounters,
					queryPagingFiles,
					queryNUMANodes,
					querySystem,
					queryTerminalServices,
					queryTimeService,
					queryNetworkInterfaces,
					queryTCPv4,
					queryTCPv6,
					queryUDPv4,
					queryUDPv6,
					queryDisk,
					queryPhysicalDisks,
					querySMBServer,
					querySMBShares,
				},
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

// TestCollector_SumsBothIPVersions checks the TCP values for both IP versions.
// A sum needs both versions from the same poll. It wraps at 32 bits, like the
// counters it adds, so the Agent unwraps it as it unwraps each counter: its
// incremental algorithm (src/database/rrdset-collection.c) takes a drop for a
// 32-bit wrap only while every value it collected fits in 32 bits, and counts
// the interval as 0 otherwise.
func TestCollector_SumsBothIPVersions(t *testing.T) {
	received := func(segments uint32) queryResult {
		return fixtureRows(map[string]any{"SegmentsReceivedPersec": segments})
	}
	failed := queryResult{
		err: errors.New("invalid class"),
	}
	for name, tc := range map[string]struct {
		tcpv4 queryResult
		tcpv6 queryResult
		want  map[string]metrix.SampleValue
	}{
		"both versions": {
			tcpv4: received(1000),
			tcpv6: received(20),
			want: map[string]metrix.SampleValue{
				"ipv4_tcp_segments_received": 1000,
				"ipv6_tcp_segments_received": 20,
				"tcp_segments_received":      1020,
			},
		},
		"without IPv6": {
			tcpv4: received(1000),
			tcpv6: failed,
			want: map[string]metrix.SampleValue{
				"ipv4_tcp_segments_received": 1000,
			},
		},
		"without IPv4": {
			tcpv4: failed,
			tcpv6: received(20),
			want: map[string]metrix.SampleValue{
				"ipv6_tcp_segments_received": 20,
			},
		},
		"a sum past 32 bits wraps": {
			tcpv4: received(math.MaxUint32 - 9),
			tcpv6: received(30),
			want: map[string]metrix.SampleValue{
				"ipv4_tcp_segments_received": math.MaxUint32 - 9,
				"ipv6_tcp_segments_received": 30,
				"tcp_segments_received":      20,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			c.Collectors.CPU = new(false)
			c.Collectors.Memory = new(false)
			c.Collectors.System = new(false)
			c.Collectors.Disk = new(false)
			c.Collectors.SMB = new(false)
			c.Collectors.Time = new(false)
			session := &fakeSession{
				results: map[string]queryResult{
					queryNetworkInterfaces: fixtureRows(),
					queryTCPv4:             tc.tcpv4,
					queryTCPv6:             tc.tcpv6,
					queryUDPv4:             failed,
					queryUDPv6:             failed,
				},
			}
			c.newSession = func(context.Context) (querySession, error) { return session, nil }
			require.NoError(t, c.Init(t.Context()))

			metrics, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)
			assert.Equal(t, withAnsweredPing(tc.want), metrics)
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
	// The host's clock in the WinRM row reads 2022-06-18 04:26:40.005 UTC; the
	// poller sends the query 1 s earlier by its clock.
	sent := time.Date(2022, 6, 18, 4, 26, 39, 5000000, time.UTC)
	offset := func(d time.Duration) clockOffset {
		return clockOffset{
			value: d,
			valid: true,
		}
	}
	for name, tc := range map[string]struct {
		rows       queryResult
		timing     queryTiming
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
			// The host read its clock 1 s after the query was sent by the
			// poller's clock; the poller compares it with the middle of the
			// 400 ms query.
			timing: queryTiming{
				sent:     sent,
				duration: 400 * time.Millisecond,
			},
			want: systemCounters{
				processes:       valid(138),
				threads:         valid(1328),
				contextSwitches: valid(746215952),
				processorQueue:  valid(2),
				uptime:          valid(5),
				clockOffset:     offset(800 * time.Millisecond),
			},
			wantUsable: true,
		},
		"a host clock behind the poller's": {
			rows: fixtureRows(map[string]any{
				"Timestamp_Object": uint64(133000000000050000),
				"Frequency_Object": uint64(10000000),
			}),
			timing: queryTiming{
				sent: sent.Add(3 * time.Second),
			},
			want: systemCounters{
				clockOffset: offset(-2 * time.Second),
			},
			wantUsable: true,
		},
		"a query that took the bound still gives a clock offset": {
			rows: fixtureRows(map[string]any{
				"Timestamp_Object": uint64(133000000000050000),
				"Frequency_Object": uint64(10000000),
			}),
			timing: queryTiming{
				sent:     sent,
				duration: time.Second,
			},
			want: systemCounters{
				clockOffset: offset(500 * time.Millisecond),
			},
			wantUsable: true,
		},
		"a slower query leaves the clock offset out": {
			rows: fixtureRows(map[string]any{
				"Processes":        uint32(69),
				"Timestamp_Object": uint64(133000000000050000),
				"Frequency_Object": uint64(10000000),
			}),
			timing: queryTiming{
				sent:     sent,
				duration: time.Second + time.Nanosecond,
			},
			want: systemCounters{
				processes: valid(69),
			},
			wantUsable: true,
		},
		"a host clock before 1970 gives a clock offset": {
			// 1969-12-31 23:59:59 UTC.
			rows: fixtureRows(map[string]any{
				"Timestamp_Object": uint64(116444735990000000),
				"Frequency_Object": uint64(10000000),
			}),
			timing: queryTiming{
				sent: time.Date(1970, 1, 1, 0, 0, 1, 0, time.UTC),
			},
			want: systemCounters{
				clockOffset: offset(-2 * time.Second),
			},
			wantUsable: true,
		},
		"a host clock centuries off leaves the clock offset out": {
			// 1601-01-01 UTC, beyond the range of a duration from the poller's clock.
			rows: fixtureRows(map[string]any{
				"Timestamp_Object": uint64(0),
				"Frequency_Object": uint64(10000000),
			}),
			timing: queryTiming{
				sent: sent,
			},
		},
		"a timestamp Windows would reject leaves the clock offset out": {
			rows: fixtureRows(map[string]any{
				"Timestamp_Object": uint64(math.MaxInt64) + 1,
				"Frequency_Object": uint64(10000000),
			}),
			timing: queryTiming{
				sent: sent,
			},
		},
		"a boot time after the sample leaves uptime out": {
			rows: fixtureRows(map[string]any{
				"Processes":        uint32(69),
				"SystemUpTime":     uint64(133000000000000001),
				"Timestamp_Object": uint64(133000000000000000),
				"Frequency_Object": uint64(10000000),
			}),
			timing: queryTiming{
				sent: sent.Add(-5 * time.Millisecond),
			},
			want: systemCounters{
				processes:   valid(69),
				clockOffset: offset(time.Second),
			},
			wantUsable: true,
		},
		"a zero frequency leaves uptime and the clock offset out": {
			rows: fixtureRows(map[string]any{
				"SystemUpTime":     uint64(0),
				"Timestamp_Object": uint64(10000000),
				"Frequency_Object": uint64(0),
			}),
			timing: queryTiming{
				sent: sent,
			},
		},
		"another frequency leaves the clock offset out": {
			rows: fixtureRows(map[string]any{
				"SystemUpTime":     uint64(0),
				"Timestamp_Object": uint64(133000000000050000),
				"Frequency_Object": uint64(1000),
			}),
			timing: queryTiming{
				sent: sent,
			},
			want: systemCounters{
				uptime: valid(133000000000050000),
			},
			wantUsable: true,
		},
		"an uptime beyond the millisecond range is left out": {
			rows: fixtureRows(map[string]any{
				"SystemUpTime":     uint64(0),
				"Timestamp_Object": uint64(math.MaxUint64),
				"Frequency_Object": uint64(1),
			}),
			timing: queryTiming{
				sent: sent,
			},
		},
		"more than one instance": {
			rows: fixtureRows(systemRow(), systemRow()),
			timing: queryTiming{
				sent: sent,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readSystem(tc.rows.rows, tc.timing))
			assert.Equal(t, tc.want, o.system)
		})
	}
}
