// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibMemory"
#include "../common-contexts/common-contexts.h"

static void initialize(void) {
    ;
}

static bool do_memory(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Memory");
    if (!pObjectType)
        return false;

    static COUNTER_DATA pagesPerSec = { .key = "Pages/sec" };
    static COUNTER_DATA pageFaultsPerSec = { .key = "Page Faults/sec" };

    if(perflibGetObjectCounter(pDataBlock, pObjectType, &pageFaultsPerSec) &&
        perflibGetObjectCounter(pDataBlock, pObjectType, &pagesPerSec)) {
        ULONGLONG total = pageFaultsPerSec.current.Data;
        ULONGLONG major = pagesPerSec.current.Data;
        ULONGLONG minor = (total > major) ? total - major : 0;
        common_mem_pgfaults(minor, major, update_every);
    }

    static COUNTER_DATA availableBytes = { .key = "Available Bytes" };
    static COUNTER_DATA availableKBytes = { .key = "Available KBytes" };
    static COUNTER_DATA availableMBytes = { .key = "Available MBytes" };
    ULONGLONG available_bytes = 0;

    if(perflibGetObjectCounter(pDataBlock, pObjectType, &availableBytes))
        available_bytes = availableBytes.current.Data;
    else if(perflibGetObjectCounter(pDataBlock, pObjectType, &availableKBytes))
        available_bytes = availableKBytes.current.Data * 1024;
    else if(perflibGetObjectCounter(pDataBlock, pObjectType, &availableMBytes))
        available_bytes = availableMBytes.current.Data * 1024 * 1024;

    common_mem_available(available_bytes, update_every);

    return true;
}

int do_PerflibMemory(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Memory");
    if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_memory(pDataBlock, update_every);

    return 0;
}
