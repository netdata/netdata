// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func validSample(v uint64) sample {
	return sample{
		value: v,
		valid: true,
	}
}

func TestReadMemory(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		want       observations
		wantUsable bool
	}{
		"WinRM text": {
			rows: fixtureRows(map[string]any{
				"TotalVisibleMemorySize": "4",
				"FreePhysicalMemory":     "1",
				"TotalVirtualMemorySize": "6",
				"FreeVirtualMemory":      "2",
			}),
			want: observations{
				memoryFree: validSample(1024),
				memoryUsed: validSample(3072),
				swapFree:   validSample(2048),
				swapUsed:   validSample(4096),
			},
			wantUsable: true,
		},
		"swap alone": {
			rows: fixtureRows(map[string]any{
				"TotalVirtualMemorySize": uint64(6),
				"FreeVirtualMemory":      uint64(2),
			}),
			want: observations{
				swapFree: validSample(2048),
				swapUsed: validSample(4096),
			},
			wantUsable: true,
		},
		"more available commit than the limit leaves the commit charge out": {
			rows: fixtureRows(map[string]any{
				"TotalVirtualMemorySize": uint64(2),
				"FreeVirtualMemory":      uint64(6),
			}),
			want: observations{
				swapFree: validSample(6144),
			},
			wantUsable: true,
		},
		"more than one instance": {
			rows: fixtureRows(map[string]any{"FreePhysicalMemory": uint64(1)}, map[string]any{"FreePhysicalMemory": uint64(1)}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readMemory(tc.rows.rows))
			assert.Equal(t, tc.want, o)
		})
	}
}

// memoryCounterValues places values by WMI property where readMemoryCounters
// stores them.
func memoryCounterValues(values map[string]sample) [len(memoryCounters)]sample {
	var samples [len(memoryCounters)]sample
	for property, value := range values {
		i := slices.IndexFunc(memoryCounters[:], func(c rawCounter) bool { return c.property == property })
		samples[i] = value
	}
	return samples
}

func TestReadMemoryCounters(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		want       observations
		wantUsable bool
	}{
		"WinRM text, a malformed value and more pages than faults": {
			rows: fixtureRows(map[string]any{
				"AvailableBytes":   "6596587520",
				"CommittedBytes":   "malformed",
				"PageFaultsPersec": "100",
				"PagesPersec":      "120",
			}),
			want: observations{
				memoryCounters: memoryCounterValues(map[string]sample{"AvailableBytes": validSample(6596587520)}),
				pageFaults: pageFaults{
					minor: validSample(0),
					major: validSample(120),
				},
			},
			wantUsable: true,
		},
		"page faults need both counters": {
			rows: fixtureRows(map[string]any{
				"PageFaultsPersec":     uint32(100),
				"SystemCodeTotalBytes": uint64(0),
			}),
			want: observations{
				memoryCounters: memoryCounterValues(map[string]sample{"SystemCodeTotalBytes": validSample(0)}),
			},
			wantUsable: true,
		},
		"page faults alone": {
			rows: fixtureRows(map[string]any{
				"PageFaultsPersec": uint32(100),
				"PagesPersec":      uint32(30),
			}),
			want: observations{
				pageFaults: pageFaults{
					minor: validSample(70),
					major: validSample(30),
				},
			},
			wantUsable: true,
		},
		"no counter": {
			rows: fixtureRows(map[string]any{"PageFaultsPersec": uint32(100)}),
		},
		"more than one instance": {
			rows: fixtureRows(memoryCountersRow(), memoryCountersRow()),
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readMemoryCounters(tc.rows.rows))
			assert.Equal(t, tc.want, o)
		})
	}
}

func TestReadNUMANodes(t *testing.T) {
	for name, tc := range map[string]struct {
		rows       queryResult
		want       []numaNode
		wantUsable bool
	}{
		"nodes; the total is not a node": {
			rows: fixtureRows(
				map[string]any{"Name": "0", "FreeAndZeroPageListMBytes": uint32(4304), "StandbyListMBytes": uint32(0)},
				map[string]any{"Name": "1", "FreeAndZeroPageListMBytes": "malformed", "StandbyListMBytes": "6598"},
				map[string]any{"Name": "_Total", "FreeAndZeroPageListMBytes": uint32(4304), "StandbyListMBytes": uint32(6598)},
			),
			want: []numaNode{
				{
					name:    "0",
					free:    validSample(4304),
					standby: validSample(0),
				},
				{
					name:    "1",
					standby: validSample(6598),
				},
			},
			wantUsable: true,
		},
		"no node reports a value": {
			rows: fixtureRows(map[string]any{"Name": "0"}, map[string]any{"FreeAndZeroPageListMBytes": uint32(1)}),
		},
		"a host without NUMA nodes": {
			wantUsable: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readNUMANodes(tc.rows.rows))
			assert.Equal(t, tc.want, o.numaNodes)
		})
	}
}

func TestReadPagingFiles(t *testing.T) {
	pagingFileRow := func(name string, used, size any) map[string]any {
		return map[string]any{"Name": name, "PercentUsage": used, "PercentUsage_Base": size}
	}
	file := func(path string, usedPages, freePages uint64) pagingFile {
		return pagingFile{
			path: path,
			used: validSample(usedPages * 4096),
			free: validSample(freePages * 4096),
		}
	}
	for name, tc := range map[string]struct {
		rows       queryResult
		want       []pagingFile
		wantUsable bool
	}{
		"names with and without the NT prefix; the total is not a file": {
			rows: fixtureRows(
				pagingFileRow(`\??\C:\pagefile.sys`, uint32(10), uint32(100)),
				pagingFileRow(`D:\pagefile.sys`, "0", "50"),
				pagingFileRow("_Total", uint32(10), uint32(150)),
			),
			want:       []pagingFile{file(`C:\pagefile.sys`, 10, 90), file(`D:\pagefile.sys`, 0, 50)},
			wantUsable: true,
		},
		"a duplicated path keeps its first valid sample": {
			rows: fixtureRows(
				pagingFileRow(`\??\C:\pagefile.sys`, uint32(10), uint32(0)),
				pagingFileRow(`C:\pagefile.sys`, uint32(20), uint32(100)),
				pagingFileRow(`\??\C:\pagefile.sys`, uint32(30), uint32(100)),
			),
			want:       []pagingFile{file(`C:\pagefile.sys`, 20, 80)},
			wantUsable: true,
		},
		"inconsistent samples leave gaps": {
			rows: fixtureRows(
				pagingFileRow(`C:\pagefile.sys`, uint32(101), uint32(100)),
				pagingFileRow(`D:\pagefile.sys`, nil, uint32(100)),
				pagingFileRow(`E:\pagefile.sys`, uint32(0), uint64(math.MaxUint64/4096+1)),
				pagingFileRow(`F:\pagefile.sys`, uint32(100), uint32(100)),
				pagingFileRow(`\??\`, uint32(1), uint32(100)),
			),
			want:       []pagingFile{file(`F:\pagefile.sys`, 100, 0)},
			wantUsable: true,
		},
		"no file gives a sample": {
			rows: fixtureRows(pagingFileRow(`C:\pagefile.sys`, uint32(1), uint32(0))),
		},
		"a host without paging files": {
			rows:       fixtureRows(pagingFileRow("_Total", uint32(0), uint32(0))),
			wantUsable: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var o observations
			assert.Equal(t, tc.wantUsable, o.readPagingFiles(tc.rows.rows))
			assert.Equal(t, tc.want, o.pagingFiles)
		})
	}
}
