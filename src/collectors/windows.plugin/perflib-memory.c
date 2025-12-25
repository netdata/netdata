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

struct system_pool {
    RRDSET *pool;
    RRDDIM *rd_paged;
    RRDDIM *rd_nonpaged;

    RRDSET *freeSystemPageTableEntries;
    RRDDIM *rd_free_system_page_table_entries;

    COUNTER_DATA pagedData;
    COUNTER_DATA nonPagedData;
    COUNTER_DATA pageTableEntries;
};

struct swap localSwap = {0};
struct system_pool localPool = {0};

void initialize_swap_keys(struct swap *p)
{
    // SWAP Operations
    p->pageReadsTotal.key = "Page Reads/sec";
    p->pageWritesTotal.key = "Page Writes/sec";

    // Swap Pages
    p->pageInputTotal.key = "Pages Input/sec";
    p->pageOutputTotal.key = "Pages Output/sec";
}

void initialize_pool_keys(struct system_pool *p)
{
    p->pagedData.key = "Pool Paged Bytes";
    p->nonPagedData.key = "Pool Nonpaged Bytes";
    p->pageTableEntries.key = "Free System Page Table Entries";
}

static void initialize(void)
{
    initialize_swap_keys(&localSwap);
    initialize_pool_keys(&localPool);
}

static void do_memory_swap(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    perflibGetObjectCounter(pDataBlock, pObjectType, &localSwap.pageReadsTotal);
    perflibGetObjectCounter(pDataBlock, pObjectType, &localSwap.pageWritesTotal);
    perflibGetObjectCounter(pDataBlock, pObjectType, &localSwap.pageInputTotal);
    perflibGetObjectCounter(pDataBlock, pObjectType, &localSwap.pageOutputTotal);

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
            NETDATA_CHART_PRIO_MEM_SWAPIO,
            update_every,
            RRDSET_TYPE_STACKED);

        localSwap.rd_op_read = rrddim_add(localSwap.operations, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        localSwap.rd_op_write = rrddim_add(localSwap.operations, "write", NULL, 1, -1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        localSwap.operations, localSwap.rd_op_read, (collected_number)localSwap.pageReadsTotal.current.Data);

    rrddim_set_by_pointer(
        localSwap.operations, localSwap.rd_op_write, (collected_number)localSwap.pageWritesTotal.current.Data);
    rrdset_done(localSwap.operations);

    if (!localSwap.pages) {
        localSwap.pages = rrdset_create_localhost(
            "mem",
            "swap_pages",
            NULL,
            "swap",
            "mem.swap_pages_io",
            "Swap Pages",
            "pages/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibMemory",
            NETDATA_CHART_PRIO_MEM_SWAP_PAGES,
            update_every,
            RRDSET_TYPE_STACKED);

        localSwap.rd_page_read = rrddim_add(localSwap.pages, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        localSwap.rd_page_write = rrddim_add(localSwap.pages, "write", NULL, 1, -1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        localSwap.pages, localSwap.rd_page_read, (collected_number)localSwap.pageInputTotal.current.Data);

    rrddim_set_by_pointer(
        localSwap.pages, localSwap.rd_page_write, (collected_number)localSwap.pageOutputTotal.current.Data);
    rrdset_done(localSwap.pages);
}

static void do_memory_system_pool(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.nonPagedData);
    perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.pagedData);

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

    rrddim_set_by_pointer(localPool.pool, localPool.rd_paged, (collected_number)localPool.pagedData.current.Data);

    rrddim_set_by_pointer(localPool.pool, localPool.rd_nonpaged, (collected_number)localPool.nonPagedData.current.Data);
    rrdset_done(localPool.pool);
}

static void do_memory_page_table_entries(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    perflibGetObjectCounter(pDataBlock, pObjectType, &localPool.pageTableEntries);

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

    do_memory_system_pool(pDataBlock, pObjectType, update_every);
    do_memory_page_table_entries(pDataBlock, pObjectType, update_every);

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

    do_memory(pDataBlock, update_every);

    return 0;
}
