// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibMemory"
#include "../common-contexts/common-contexts.h"

static void initialize(void) {
    ;
}

static bool do_processes(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "System");
    if (!pObjectType)
        return false;

    static COUNTER_DATA processorsRunning = { .key = "Processes" };

    if(perflibGetObjectCounter(pDataBlock, pObjectType, processorsRunning)) {
        ULONGLONG total = pageFaultsPerSec.current.Data;
        ULONGLONG major = pagesPerSec.current.Data;
        ULONGLONG minor = (total > major) ? total - major : 0;
        common_mem_pgfaults(minor, major, update_every);
    }

    return true;
}

int do_PerflibProcesses(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("System");
    if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_processes(pDataBlock, update_every);

    return 0;
}
