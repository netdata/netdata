// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibMemory"
#include "../common-contexts/common-contexts.h"

struct swap {
    RRDSET *operations;
    RRDDIM *rd_op_read;
    RRDDIM *rd_op_write;

    RRDSET *pages;
    RRDDIM *rd_page_read;
    RRDDIM *rd_page_write;

    COUNTER_DATA pageReadsTotal;
    COUNTER_DATA pageWritesTotal;
    COUNTER_DATA pageInputTotal;
    COUNTER_DATA pageOutputTotal;
};

struct commit {
    RRDSET *chart;
    RRDDIM *rd_committed;
    RRDDIM *rd_limit;

    COUNTER_DATA committedBytes;
    COUNTER_DATA commitLimit;
};

struct system_cache {
    RRDSET *chart;
    RRDDIM *rd_cache;
    RRDDIM *rd_peak;
    RRDDIM *rd_resident;

    COUNTER_DATA cacheBytes;
    COUNTER_DATA cacheBytesPeak;
    COUNTER_DATA residentBytes;
};

struct page_lists {
    RRDSET *chart;
    RRDDIM *rd_free;
    RRDDIM *rd_modified;
    RRDDIM *rd_standby_core;
    RRDDIM *rd_standby_normal;
    RRDDIM *rd_standby_reserve;

    COUNTER_DATA freeAndZeroPageListBytes;
    COUNTER_DATA modifiedPageListBytes;
    COUNTER_DATA standbyCacheCoreBytes;
    COUNTER_DATA standbyCacheNormalPriorityBytes;
    COUNTER_DATA standbyCacheReserveBytes;
};

struct system_pool {
    RRDSET *pool;
    RRDDIM *rd_paged;
    RRDDIM *rd_nonpaged;

    RRDSET *paged;
    RRDDIM *rd_paged_resident;

    RRDSET *allocs;
    RRDDIM *rd_allocs_paged;
    RRDDIM *rd_allocs_nonpaged;

    RRDSET *freeSystemPageTableEntries;
    RRDDIM *rd_free_system_page_table_entries;

    COUNTER_DATA pagedBytes;
    COUNTER_DATA pagedResidentBytes;
    COUNTER_DATA pagedAllocs;
    COUNTER_DATA nonPagedBytes;
    COUNTER_DATA nonPagedAllocs;
    COUNTER_DATA pageTableEntries;
};

struct system_code {
    RRDSET *chart;
    RRDDIM *rd_resident;
    RRDDIM *rd_total;

    COUNTER_DATA residentBytes;
    COUNTER_DATA totalBytes;
};

struct system_drivers {
    RRDSET *chart;
    RRDDIM *rd_resident;
    RRDDIM *rd_total;

    COUNTER_DATA residentBytes;
    COUNTER_DATA totalBytes;
};

struct page_faults {
    RRDSET *breakdown;
    RRDDIM *rd_cache;
    RRDDIM *rd_demand_zero;
    RRDDIM *rd_transition;
    RRDDIM *rd_write_copies;

    RRDSET *transition_repurposed;
    RRDDIM *rd_repurposed;

    COUNTER_DATA cacheFaultsPerSec;
    COUNTER_DATA demandZeroFaultsPerSec;
    COUNTER_DATA transitionFaultsPerSec;
    COUNTER_DATA transitionPagesRePurposedPerSec;
    COUNTER_DATA writeCopiesPerSec;
};

static struct swap localSwap = {0};
static struct commit localCommit = {0};
static struct system_cache localCache = {0};
static struct page_lists localPageLists = {0};
static struct system_pool localPool = {0};
static struct system_code localCode = {0};
static struct system_drivers localDrivers = {0};
static struct page_faults localFaults = {0};

static void initialize_swap_keys(struct swap *p)
{
    p->pageReadsTotal.key = "Page Reads/sec";
    p->pageWritesTotal.key = "Page Writes/sec";
    p->pageInputTotal.key = "Pages Input/sec";
    p->pageOutputTotal.key = "Pages Output/sec";
}

static void initialize_commit_keys(struct commit *p)
{
    p->committedBytes.key = "Committed Bytes";
    p->commitLimit.key = "Commit Limit";
}

static void initialize_cache_keys(struct system_cache *p)
{
    p->cacheBytes.key = "Cache Bytes";
    p->cacheBytesPeak.key = "Cache Bytes Peak";
    p->residentBytes.key = "System Cache Resident Bytes";
}

static void initialize_page_list_keys(struct page_lists *p)
{
    p->freeAndZeroPageListBytes.key = "Free & Zero Page List Bytes";
    p->modifiedPageListBytes.key = "Modified Page List Bytes";
    p->standbyCacheCoreBytes.key = "Standby Cache Core Bytes";
    p->standbyCacheNormalPriorityBytes.key = "Standby Cache Normal Priority Bytes";
    p->standbyCacheReserveBytes.key = "Standby Cache Reserve Bytes";
}

static void initialize_pool_keys(struct system_pool *p)
{
    p->pagedBytes.key = "Pool Paged Bytes";
    p->pagedResidentBytes.key = "Pool Paged Resident Bytes";
    p->pagedAllocs.key = "Pool Paged Allocs";
    p->nonPagedBytes.key = "Pool Nonpaged Bytes";
    p->nonPagedAllocs.key = "Pool Nonpaged Allocs";
    p->pageTableEntries.key = "Free System Page Table Entries";
}

static void initialize_system_code_keys(struct system_code *p)
{
    p->residentBytes.key = "System Code Resident Bytes";
    p->totalBytes.key = "System Code Total Bytes";
}

static void initialize_system_driver_keys(struct system_drivers *p)
{
    p->residentBytes.key = "System Driver Resident Bytes";
    p->totalBytes.key = "System Driver Total Bytes";
}

static void initialize_fault_keys(struct page_faults *p)
{
    p->cacheFaultsPerSec.key = "Cache Faults/sec";
    p->demandZeroFaultsPerSec.key = "Demand Zero Faults/sec";
    p->transitionFaultsPerSec.key = "Transition Faults/sec";
    p->transitionPagesRePurposedPerSec.key = "Transition Pages RePurposed/sec";
    p->writeCopiesPerSec.key = "Write Copies/sec";
}

static void initialize(void)
{
    initialize_swap_keys(&localSwap);
    initialize_commit_keys(&localCommit);
    initialize_cache_keys(&localCache);
    initialize_page_list_keys(&localPageLists);
    initialize_pool_keys(&localPool);
    initialize_system_code_keys(&localCode);
    initialize_system_driver_keys(&localDrivers);
    initialize_fault_keys(&localFaults);
}

static void do_memory_swap_operations_chart(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    collected_number page_reads = 0;
    collected_number page_writes = 0;
    bool has_page_reads = perflibGetObjectCounter(pDataBlock, pObjectType, &localSwap.pageReadsTotal);
    bool has_page_writes = perflibGetObjectCounter(pDataBlock, pObjectType, &localSwap.pageWritesTotal);

    if (!has_page_reads && !has_page_writes)
        return;

    if (has_page_reads)
        page_reads = (collected_number)localSwap.pageReadsTotal.current.Data;

    if (has_page_writes)
        page_writes = (collected_number)localSwap.pageWritesTotal.current.Data;

    if (!localSwap.operations) {
        localSwap.operations = rrdset_create_localhost(
            "mem",
            "swap_operations",
            NULL,
            "swap",
            "mem.swap_iops",
            "Swap Operations",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SWAP_CALLS,
            update_every,
            RRDSET_TYPE_STACKED);

        localSwap.rd_op_read = rrddim_add(localSwap.operations, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        localSwap.rd_op_write = rrddim_add(localSwap.operations, "write", NULL, 1, -1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(localSwap.operations, localSwap.rd_op_read, page_reads);
    rrddim_set_by_pointer(localSwap.operations, localSwap.rd_op_write, page_writes);
    rrdset_done(localSwap.operations);
}

static void do_memory_swap_pages_chart(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    collected_number page_input = 0;
    collected_number page_output = 0;
    bool has_page_input = perflibGetObjectCounter(pDataBlock, pObjectType, &localSwap.pageInputTotal);
    bool has_page_output = perflibGetObjectCounter(pDataBlock, pObjectType, &localSwap.pageOutputTotal);

    if (!has_page_input && !has_page_output)
        return;

    if (has_page_input)
        page_input = (collected_number)localSwap.pageInputTotal.current.Data;

    if (has_page_output)
        page_output = (collected_number)localSwap.pageOutputTotal.current.Data;

    if (!localSwap.pages) {
        localSwap.pages = rrdset_create_localhost(
            "mem",
            "swapio",
            NULL,
            "swap",
            "mem.swapio",
            "Swap I/O",
            "KiB/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SWAPIO,
            update_every,
            RRDSET_TYPE_AREA);

        // divide by 1024 to convert bytes to KiB/s
        collected_number page_size = (collected_number)os_get_system_page_size();
        localSwap.rd_page_read = rrddim_add(localSwap.pages, "in",  NULL,  page_size, 1024, RRD_ALGORITHM_INCREMENTAL);
        localSwap.rd_page_write = rrddim_add(localSwap.pages, "out", NULL, -page_size, 1024, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(localSwap.pages, localSwap.rd_page_read, page_input);
    rrddim_set_by_pointer(localSwap.pages, localSwap.rd_page_write, page_output);
    rrdset_done(localSwap.pages);
}

static void do_memory_swap(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    do_memory_swap_operations_chart(pDataBlock, pObjectType, update_every);
    do_memory_swap_pages_chart(pDataBlock, pObjectType, update_every);
}

static void do_memory_commit(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    collected_number committed = 0;
    collected_number limit = 0;
    bool has_data = false;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localCommit.committedBytes)) {
        committed = (collected_number)localCommit.committedBytes.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localCommit.commitLimit)) {
        limit = (collected_number)localCommit.commitLimit.current.Data;
        has_data = true;
    }

    if (!has_data)
        return;

    if (!localCommit.chart) {
        localCommit.chart = rrdset_create_localhost(
            "mem",
            "committed",
            NULL,
            "mem",
            "mem.committed",
            "Committed Memory",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_COMMITTED,
            update_every,
            RRDSET_TYPE_LINE);

        localCommit.rd_committed = rrddim_add(localCommit.chart, "committed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localCommit.rd_limit = rrddim_add(localCommit.chart, "limit", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(localCommit.chart, localCommit.rd_committed, committed);
    rrddim_set_by_pointer(localCommit.chart, localCommit.rd_limit, limit);
    rrdset_done(localCommit.chart);
}

static void do_memory_cache(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    collected_number cache = 0;
    collected_number peak = 0;
    collected_number resident = 0;
    bool has_data = false;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localCache.cacheBytes)) {
        cache = (collected_number)localCache.cacheBytes.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localCache.cacheBytesPeak)) {
        peak = (collected_number)localCache.cacheBytesPeak.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localCache.residentBytes)) {
        resident = (collected_number)localCache.residentBytes.current.Data;
        has_data = true;
    }

    if (!has_data)
        return;

    if (!localCache.chart) {
        localCache.chart = rrdset_create_localhost(
            "mem",
            "system_cache",
            NULL,
            "mem",
            "mem.system_cache",
            "System Cache Memory",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_CACHE,
            update_every,
            RRDSET_TYPE_LINE);

        localCache.rd_cache = rrddim_add(localCache.chart, "cache", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localCache.rd_peak = rrddim_add(localCache.chart, "peak", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localCache.rd_resident = rrddim_add(localCache.chart, "resident", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(localCache.chart, localCache.rd_cache, cache);
    rrddim_set_by_pointer(localCache.chart, localCache.rd_peak, peak);
    rrddim_set_by_pointer(localCache.chart, localCache.rd_resident, resident);
    rrdset_done(localCache.chart);
}

static void do_memory_page_lists(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    collected_number free_bytes = 0;
    collected_number modified = 0;
    collected_number standby_core = 0;
    collected_number standby_normal = 0;
    collected_number standby_reserve = 0;
    bool has_data = false;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localPageLists.freeAndZeroPageListBytes)) {
        free_bytes = (collected_number)localPageLists.freeAndZeroPageListBytes.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localPageLists.modifiedPageListBytes)) {
        modified = (collected_number)localPageLists.modifiedPageListBytes.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localPageLists.standbyCacheCoreBytes)) {
        standby_core = (collected_number)localPageLists.standbyCacheCoreBytes.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localPageLists.standbyCacheNormalPriorityBytes)) {
        standby_normal = (collected_number)localPageLists.standbyCacheNormalPriorityBytes.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localPageLists.standbyCacheReserveBytes)) {
        standby_reserve = (collected_number)localPageLists.standbyCacheReserveBytes.current.Data;
        has_data = true;
    }

    if (!has_data)
        return;

    if (!localPageLists.chart) {
        localPageLists.chart = rrdset_create_localhost(
            "mem",
            "page_lists",
            NULL,
            "mem",
            "mem.page_lists",
            "Memory Page Lists",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_PAGE_LISTS,
            update_every,
            RRDSET_TYPE_STACKED);

        localPageLists.rd_free = rrddim_add(localPageLists.chart, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localPageLists.rd_modified =
            rrddim_add(localPageLists.chart, "modified", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localPageLists.rd_standby_core =
            rrddim_add(localPageLists.chart, "standby_core", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localPageLists.rd_standby_normal =
            rrddim_add(localPageLists.chart, "standby_normal", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localPageLists.rd_standby_reserve =
            rrddim_add(localPageLists.chart, "standby_reserve", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(localPageLists.chart, localPageLists.rd_free, free_bytes);
    rrddim_set_by_pointer(localPageLists.chart, localPageLists.rd_modified, modified);
    rrddim_set_by_pointer(localPageLists.chart, localPageLists.rd_standby_core, standby_core);
    rrddim_set_by_pointer(localPageLists.chart, localPageLists.rd_standby_normal, standby_normal);
    rrddim_set_by_pointer(localPageLists.chart, localPageLists.rd_standby_reserve, standby_reserve);
    rrdset_done(localPageLists.chart);
}

static void do_memory_system_pool_size_chart(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    collected_number paged = 0;
    collected_number nonpaged = 0;
    bool has_paged = perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.pagedBytes);
    bool has_nonpaged = perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.nonPagedBytes);

    if (!has_paged && !has_nonpaged)
        return;

    if (has_paged)
        paged = (collected_number)localPool.pagedBytes.current.Data;

    if (has_nonpaged)
        nonpaged = (collected_number)localPool.nonPagedBytes.current.Data;

    if (!localPool.pool) {
        localPool.pool = rrdset_create_localhost(
            "mem",
            "system_pool",
            NULL,
            "mem",
            "mem.system_pool_size",
            "System Memory Pool",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_POOL,
            update_every,
            RRDSET_TYPE_STACKED);

        localPool.rd_paged = rrddim_add(localPool.pool, "paged", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localPool.rd_nonpaged = rrddim_add(localPool.pool, "non-paged", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(localPool.pool, localPool.rd_paged, paged);
    rrddim_set_by_pointer(localPool.pool, localPool.rd_nonpaged, nonpaged);
    rrdset_done(localPool.pool);
}

static void do_memory_system_pool_paged_chart(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.pagedResidentBytes))
        return;

    if (!localPool.paged) {
        localPool.paged = rrdset_create_localhost(
            "mem",
            "system_pool_paged",
            NULL,
            "mem",
            "mem.system_pool_paged",
            "Paged System Memory Pool",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_POOL_PAGED,
            update_every,
            RRDSET_TYPE_LINE);

        localPool.rd_paged_resident =
            rrddim_add(localPool.paged, "resident", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        localPool.paged,
        localPool.rd_paged_resident,
        (collected_number)localPool.pagedResidentBytes.current.Data);
    rrdset_done(localPool.paged);
}

static void do_memory_system_pool_allocs_chart(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    collected_number paged_allocs = 0;
    collected_number nonpaged_allocs = 0;
    bool has_paged_allocs = perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.pagedAllocs);
    bool has_nonpaged_allocs = perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.nonPagedAllocs);

    if (!has_paged_allocs && !has_nonpaged_allocs)
        return;

    if (has_paged_allocs)
        paged_allocs = (collected_number)localPool.pagedAllocs.current.Data;

    if (has_nonpaged_allocs)
        nonpaged_allocs = (collected_number)localPool.nonPagedAllocs.current.Data;

    if (!localPool.allocs) {
        localPool.allocs = rrdset_create_localhost(
            "mem",
            "system_pool_allocs",
            NULL,
            "mem",
            "mem.system_pool_allocs",
            "System Pool Allocations",
            "allocations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_POOL_ALLOCS,
            update_every,
            RRDSET_TYPE_LINE);

        localPool.rd_allocs_paged = rrddim_add(localPool.allocs, "paged", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        localPool.rd_allocs_nonpaged =
            rrddim_add(localPool.allocs, "non-paged", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(localPool.allocs, localPool.rd_allocs_paged, paged_allocs);
    rrddim_set_by_pointer(localPool.allocs, localPool.rd_allocs_nonpaged, nonpaged_allocs);
    rrdset_done(localPool.allocs);
}

static void do_memory_system_pool(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    do_memory_system_pool_size_chart(pDataBlock, pObjectType, update_every);
    do_memory_system_pool_paged_chart(pDataBlock, pObjectType, update_every);
    do_memory_system_pool_allocs_chart(pDataBlock, pObjectType, update_every);
}

static void do_memory_page_table_entries(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.pageTableEntries))
        return;

    if (!localPool.freeSystemPageTableEntries) {
        localPool.freeSystemPageTableEntries = rrdset_create_localhost(
            "mem",
            "free_system_page_table_entries",
            NULL,
            "mem",
            "mem.system_page_table_entries",
            "Unused page table entries.",
            "pages",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_FREE_SYSTEM_PAGE,
            update_every,
            RRDSET_TYPE_LINE);

        localPool.rd_free_system_page_table_entries =
            rrddim_add(localPool.freeSystemPageTableEntries, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        localPool.freeSystemPageTableEntries,
        localPool.rd_free_system_page_table_entries,
        (collected_number)localPool.pageTableEntries.current.Data);
    rrdset_done(localPool.freeSystemPageTableEntries);
}

static void do_memory_system_code(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    collected_number resident = 0;
    collected_number total = 0;
    bool has_data = false;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localCode.residentBytes)) {
        resident = (collected_number)localCode.residentBytes.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localCode.totalBytes)) {
        total = (collected_number)localCode.totalBytes.current.Data;
        has_data = true;
    }

    if (!has_data)
        return;

    if (!localCode.chart) {
        localCode.chart = rrdset_create_localhost(
            "mem",
            "system_code",
            NULL,
            "mem",
            "mem.system_code",
            "System Code Memory",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_CODE,
            update_every,
            RRDSET_TYPE_LINE);

        localCode.rd_resident = rrddim_add(localCode.chart, "resident", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localCode.rd_total = rrddim_add(localCode.chart, "total", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(localCode.chart, localCode.rd_resident, resident);
    rrddim_set_by_pointer(localCode.chart, localCode.rd_total, total);
    rrdset_done(localCode.chart);
}

static void do_memory_system_drivers(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    collected_number resident = 0;
    collected_number total = 0;
    bool has_data = false;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localDrivers.residentBytes)) {
        resident = (collected_number)localDrivers.residentBytes.current.Data;
        has_data = true;
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &localDrivers.totalBytes)) {
        total = (collected_number)localDrivers.totalBytes.current.Data;
        has_data = true;
    }

    if (!has_data)
        return;

    if (!localDrivers.chart) {
        localDrivers.chart = rrdset_create_localhost(
            "mem",
            "system_drivers",
            NULL,
            "mem",
            "mem.system_drivers",
            "System Driver Memory",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_DRIVERS,
            update_every,
            RRDSET_TYPE_LINE);

        localDrivers.rd_resident =
            rrddim_add(localDrivers.chart, "resident", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        localDrivers.rd_total = rrddim_add(localDrivers.chart, "total", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(localDrivers.chart, localDrivers.rd_resident, resident);
    rrddim_set_by_pointer(localDrivers.chart, localDrivers.rd_total, total);
    rrdset_done(localDrivers.chart);
}

static void do_memory_faults_breakdown_chart(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    collected_number cache = 0;
    collected_number demand_zero = 0;
    collected_number transition = 0;
    collected_number write_copies = 0;
    bool has_cache = perflibGetObjectCounter(pDataBlock, pObjectType, &localFaults.cacheFaultsPerSec);
    bool has_demand_zero = perflibGetObjectCounter(pDataBlock, pObjectType, &localFaults.demandZeroFaultsPerSec);
    bool has_transition = perflibGetObjectCounter(pDataBlock, pObjectType, &localFaults.transitionFaultsPerSec);
    bool has_write_copies = perflibGetObjectCounter(pDataBlock, pObjectType, &localFaults.writeCopiesPerSec);

    if (!has_cache && !has_demand_zero && !has_transition && !has_write_copies)
        return;

    if (has_cache)
        cache = (collected_number)localFaults.cacheFaultsPerSec.current.Data;

    if (has_demand_zero)
        demand_zero = (collected_number)localFaults.demandZeroFaultsPerSec.current.Data;

    if (has_transition)
        transition = (collected_number)localFaults.transitionFaultsPerSec.current.Data;

    if (has_write_copies)
        write_copies = (collected_number)localFaults.writeCopiesPerSec.current.Data;

    if (!localFaults.breakdown) {
        localFaults.breakdown = rrdset_create_localhost(
            "mem",
            "page_faults_breakdown",
            NULL,
            "page faults",
            "mem.page_faults_breakdown",
            "Memory Page Fault Breakdown",
            "faults/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_PGFAULTS_DETAIL,
            update_every,
            RRDSET_TYPE_LINE);

        localFaults.rd_cache =
            rrddim_add(localFaults.breakdown, "cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        localFaults.rd_demand_zero =
            rrddim_add(localFaults.breakdown, "demand_zero", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        localFaults.rd_transition =
            rrddim_add(localFaults.breakdown, "transition", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        localFaults.rd_write_copies =
            rrddim_add(localFaults.breakdown, "write_copies", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(localFaults.breakdown, localFaults.rd_cache, cache);
    rrddim_set_by_pointer(localFaults.breakdown, localFaults.rd_demand_zero, demand_zero);
    rrddim_set_by_pointer(localFaults.breakdown, localFaults.rd_transition, transition);
    rrddim_set_by_pointer(localFaults.breakdown, localFaults.rd_write_copies, write_copies);
    rrdset_done(localFaults.breakdown);
}

static void do_memory_transition_repurposed_chart(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    collected_number repurposed = 0;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &localFaults.transitionPagesRePurposedPerSec))
        return;

    repurposed = (collected_number)localFaults.transitionPagesRePurposedPerSec.current.Data;

    if (!localFaults.transition_repurposed) {
        localFaults.transition_repurposed = rrdset_create_localhost(
            "mem",
            "transition_repurposed",
            NULL,
            "page faults",
            "mem.transition_repurposed",
            "Transition Pages Repurposed",
            "pages/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SYSTEM_TRANSITION,
            update_every,
            RRDSET_TYPE_LINE);

        localFaults.rd_repurposed = rrddim_add(
            localFaults.transition_repurposed, "repurposed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(localFaults.transition_repurposed, localFaults.rd_repurposed, repurposed);
    rrdset_done(localFaults.transition_repurposed);
}

static void do_memory_faults(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    do_memory_faults_breakdown_chart(pDataBlock, pObjectType, update_every);
    do_memory_transition_repurposed_chart(pDataBlock, pObjectType, update_every);
}

static bool do_memory(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Memory");
    if (!pObjectType)
        return false;

    static COUNTER_DATA pagesPerSec = {.key = "Pages/sec"};
    static COUNTER_DATA pageFaultsPerSec = {.key = "Page Faults/sec"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &pageFaultsPerSec) &&
        perflibGetObjectCounter(pDataBlock, pObjectType, &pagesPerSec)) {
        ULONGLONG total = pageFaultsPerSec.current.Data;
        ULONGLONG major = pagesPerSec.current.Data;
        ULONGLONG minor = (total > major) ? total - major : 0;
        common_mem_pgfaults(minor, major, update_every);
    }

    static COUNTER_DATA availableBytes = {.key = "Available Bytes"};
    static COUNTER_DATA availableKBytes = {.key = "Available KBytes"};
    static COUNTER_DATA availableMBytes = {.key = "Available MBytes"};
    ULONGLONG available_bytes = 0;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &availableBytes))
        available_bytes = availableBytes.current.Data;
    else if (perflibGetObjectCounter(pDataBlock, pObjectType, &availableKBytes))
        available_bytes = availableKBytes.current.Data * 1024;
    else if (perflibGetObjectCounter(pDataBlock, pObjectType, &availableMBytes))
        available_bytes = availableMBytes.current.Data * 1024 * 1024;

    common_mem_available(available_bytes, update_every);

    do_memory_swap(pDataBlock, pObjectType, update_every);
    do_memory_commit(pDataBlock, pObjectType, update_every);
    do_memory_cache(pDataBlock, pObjectType, update_every);
    do_memory_page_lists(pDataBlock, pObjectType, update_every);
    do_memory_system_pool(pDataBlock, pObjectType, update_every);
    do_memory_page_table_entries(pDataBlock, pObjectType, update_every);
    do_memory_system_code(pDataBlock, pObjectType, update_every);
    do_memory_system_drivers(pDataBlock, pObjectType, update_every);
    do_memory_faults(pDataBlock, pObjectType, update_every);

    return true;
}

int do_PerflibMemory(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Memory");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    if (!do_memory(pDataBlock, update_every))
        return -1;

    return 0;
}
