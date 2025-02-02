// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibProcesses"
#include "../common-contexts/common-contexts.h"

static void initialize(void)
{
    ;
}

static bool do_processes(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "System");
    if (!pObjectType)
        return false;

    static COUNTER_DATA processesRunning = {.key = "Processes"};
    static COUNTER_DATA contextSwitchPerSec = {.key = "Context Switches/sec"};
    static COUNTER_DATA threads = {.key = "Threads"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &processesRunning)) {
        ULONGLONG running = processesRunning.current.Data;
        common_system_processes(running, update_every);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &contextSwitchPerSec)) {
        ULONGLONG contexts = contextSwitchPerSec.current.Data;
        common_system_context_switch(contexts, update_every);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &threads)) {
        ULONGLONG totalThreads = threads.current.Data;
        common_system_threads(totalThreads, update_every);
    }
    return true;
}

int do_PerflibProcesses(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("System");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    do_processes(pDataBlock, update_every);

    return 0;
}
