// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibProcesses"
#include "../common-contexts/common-contexts.h"

static void do_processor_queue(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static RRDSET *st_queue = NULL;
    static RRDDIM *rd_queue = NULL;
    static COUNTER_DATA processorQueue = {.key = "Processor Queue Length"};
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &processorQueue))
        return;

    if (!st_queue) {
        st_queue = rrdset_create_localhost(
            "system",
            "processor_queue",
            NULL,
            "system",
            "system.processor_queue_length",
            "The number of threads in the processor queue.",
            "threads",
            _COMMON_PLUGIN_NAME,
            _COMMON_PLUGIN_MODULE_NAME,
            NETDATA_CHART_PRIO_SYSTEM_THREAD_QUEUE,
            update_every,
            RRDSET_TYPE_LINE);

        rd_queue = rrddim_add(st_queue, "threads", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_queue, rd_queue, (collected_number)processorQueue.current.Data);

    rrdset_done(st_queue);
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

    do_processor_queue(pDataBlock, pObjectType, update_every);
    return true;
}

int do_PerflibProcesses(int update_every, usec_t dt __maybe_unused)
{
    DWORD id = RegistryFindIDByName("System");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    do_processes(pDataBlock, update_every);

    return 0;
}
