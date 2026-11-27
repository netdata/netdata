// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"math"
	"strings"
)

// queryMemory reads the physical memory and the commit limit and available
// commit, in KiB: the values GlobalMemoryStatusEx gives the native Windows
// Agent's system.ram and mem.swap.
const queryMemory = "SELECT TotalVisibleMemorySize, FreePhysicalMemory, TotalVirtualMemorySize, FreeVirtualMemory " +
	"FROM Win32_OperatingSystem"

// memoryCounter is a raw counter of the Memory counter set that feeds one
// dimension of the native Windows Agent's memory charts.
type memoryCounter struct {
	property string
	metric   string
	// total marks a count since the counter started, whose rate Netdata charts.
	total bool
}

var memoryCounters = [...]memoryCounter{
	{property: "AvailableBytes", metric: "memory_available_bytes"},
	{property: "PageReadsPersec", metric: "memory_page_reads", total: true},
	{property: "PageWritesPersec", metric: "memory_page_writes", total: true},
	{property: "PagesInputPersec", metric: "memory_pages_input", total: true},
	{property: "PagesOutputPersec", metric: "memory_pages_output", total: true},
	{property: "CommittedBytes", metric: "memory_committed_bytes"},
	{property: "CommitLimit", metric: "memory_commit_limit_bytes"},
	{property: "CacheBytes", metric: "memory_system_cache_bytes"},
	{property: "CacheBytesPeak", metric: "memory_system_cache_peak_bytes"},
	{property: "SystemCacheResidentBytes", metric: "memory_system_cache_resident_bytes"},
	{property: "FreeAndZeroPageListBytes", metric: "memory_free_and_zero_page_list_bytes"},
	{property: "ModifiedPageListBytes", metric: "memory_modified_page_list_bytes"},
	{property: "StandbyCacheCoreBytes", metric: "memory_standby_cache_core_bytes"},
	{property: "StandbyCacheNormalPriorityBytes", metric: "memory_standby_cache_normal_priority_bytes"},
	{property: "StandbyCacheReserveBytes", metric: "memory_standby_cache_reserve_bytes"},
	{property: "PoolPagedBytes", metric: "memory_pool_paged_bytes"},
	{property: "PoolNonpagedBytes", metric: "memory_pool_nonpaged_bytes"},
	{property: "PoolPagedResidentBytes", metric: "memory_pool_paged_resident_bytes"},
	// Current numbers of outstanding allocations, which the native chart
	// charts as rates.
	{property: "PoolPagedAllocs", metric: "memory_pool_paged_allocations"},
	{property: "PoolNonpagedAllocs", metric: "memory_pool_nonpaged_allocations"},
	{property: "FreeSystemPageTableEntries", metric: "memory_free_system_page_table_entries"},
	{property: "SystemCodeResidentBytes", metric: "memory_system_code_resident_bytes"},
	{property: "SystemCodeTotalBytes", metric: "memory_system_code_total_bytes"},
	{property: "SystemDriverResidentBytes", metric: "memory_system_driver_resident_bytes"},
	{property: "SystemDriverTotalBytes", metric: "memory_system_driver_total_bytes"},
	{property: "CacheFaultsPersec", metric: "memory_cache_faults", total: true},
	{property: "DemandZeroFaultsPersec", metric: "memory_demand_zero_faults", total: true},
	{property: "TransitionFaultsPersec", metric: "memory_transition_faults", total: true},
	{property: "WriteCopiesPersec", metric: "memory_write_copy_faults", total: true},
	{property: "TransitionPagesRePurposedPersec", metric: "memory_transition_pages_repurposed", total: true},
}

// The page fault counters feed mem.pgfaults only, through pageFaults.
const (
	pageFaultsProperty = "PageFaultsPersec"
	pagesProperty      = "PagesPersec"
)

var queryMemoryCounters = func() string {
	properties := []string{pageFaultsProperty, pagesProperty}
	for _, counter := range memoryCounters {
		properties = append(properties, counter.property)
	}
	return "SELECT " + strings.Join(properties, ", ") + " FROM Win32_PerfRawData_PerfOS_Memory"
}()

// queryPagingFiles reads each paging file's pages in use and its size in
// pages, the raw fraction the native Windows Agent reads.
const queryPagingFiles = "SELECT Name, PercentUsage, PercentUsage_Base FROM Win32_PerfRawData_PerfOS_PagingFile"

// queryNUMANodes reads each NUMA node's free and standby memory, in MiB, as
// the native Windows Agent does.
const queryNUMANodes = "SELECT Name, FreeAndZeroPageListMBytes, StandbyListMBytes " +
	"FROM Win32_PerfRawData_PerfOS_NUMANodeMemory"

// pageSize is the memory page size of every Windows architecture (x86, x64,
// ARM64). The native Windows Agent uses the local page size for the same
// counters.
const pageSize = 4096

// pagingFileNTPrefix is the NT object-manager prefix of paging file names.
const pagingFileNTPrefix = `\??\`

// pageFaults splits the page faults as the native Windows Agent does: major
// faults read pages from disk, the others are minor.
type pageFaults struct {
	minor sample
	major sample
}

type pagingFile struct {
	path string
	used sample
	free sample
}

// numaNode holds a NUMA node's memory in MiB.
type numaNode struct {
	name    string
	free    sample
	standby sample
}

// readMemory converts Win32_OperatingSystem's KiB values to bytes. Free
// physical memory is the memory available to processes, as the native Windows
// Agent's free dimension; the swap is the commit charge against the commit
// limit, as the native mem.swap.
func (o *observations) readMemory(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	row := rows[0]
	total := kibToBytes(readUnsigned(row["totalvisiblememorysize"]))
	free := kibToBytes(readUnsigned(row["freephysicalmemory"]))
	o.memoryFree = free
	o.memoryUsed = used(total, free)
	commitLimit := kibToBytes(readUnsigned(row["totalvirtualmemorysize"]))
	commitAvailable := kibToBytes(readUnsigned(row["freevirtualmemory"]))
	o.swapFree = commitAvailable
	o.swapUsed = used(commitLimit, commitAvailable)
	return free.valid || commitAvailable.valid
}

func (o *observations) readMemoryCounters(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	row := rows[0]
	var usable bool
	for i, counter := range memoryCounters {
		o.memoryCounters[i] = readUnsigned(row[strings.ToLower(counter.property)])
		usable = usable || o.memoryCounters[i].valid
	}
	total := readUnsigned(row[strings.ToLower(pageFaultsProperty)])
	major := readUnsigned(row[strings.ToLower(pagesProperty)])
	if total.valid && major.valid {
		o.pageFaults = pageFaults{
			minor: sample{
				value: total.value - min(total.value, major.value),
				valid: true,
			},
			major: major,
		}
		usable = true
	}
	return usable
}

// readPagingFiles keeps each paging file's first valid sample. The counter
// set's _Total is not a file. A file whose sample is missing or inconsistent
// leaves a gap.
func (o *observations) readPagingFiles(rows []map[string]any) bool {
	var files int
	for _, row := range rows {
		name, _ := row["name"].(string)
		path := strings.TrimPrefix(name, pagingFileNTPrefix)
		if path == "" || strings.EqualFold(path, "_Total") {
			continue
		}
		files++
		if o.hasPagingFile(path) {
			continue
		}
		usedPages := readUnsigned(row["percentusage"])
		sizePages := readUnsigned(row["percentusage_base"])
		if !usedPages.valid || !sizePages.valid || sizePages.value == 0 || usedPages.value > sizePages.value ||
			sizePages.value > math.MaxUint64/pageSize {
			continue
		}
		o.pagingFiles = append(o.pagingFiles, pagingFile{
			path: path,
			used: sample{
				value: usedPages.value * pageSize,
				valid: true,
			},
			free: sample{
				value: (sizePages.value - usedPages.value) * pageSize,
				valid: true,
			},
		})
	}
	return files == 0 || len(o.pagingFiles) > 0
}

// readNUMANodes keeps the nodes that report a value. The counter set's _Total
// is not a node.
func (o *observations) readNUMANodes(rows []map[string]any) bool {
	var nodes int
	for _, row := range rows {
		name, _ := row["name"].(string)
		if name == "" || strings.EqualFold(name, "_Total") {
			continue
		}
		nodes++
		node := numaNode{
			name:    name,
			free:    readUnsigned(row["freeandzeropagelistmbytes"]),
			standby: readUnsigned(row["standbylistmbytes"]),
		}
		if node.free.valid || node.standby.valid {
			o.numaNodes = append(o.numaNodes, node)
		}
	}
	return nodes == 0 || len(o.numaNodes) > 0
}

func (o *observations) hasPagingFile(path string) bool {
	for _, file := range o.pagingFiles {
		if file.path == path {
			return true
		}
	}
	return false
}
