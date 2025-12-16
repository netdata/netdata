// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "PerflibObjects"
#include "../common-contexts/common-contexts.h"

static void initialize(void)
{
    ;
}

static bool do_objects(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Objects");
    if (!pObjectType)
        return false;

    static COUNTER_DATA semaphores = {.key = "Semaphores"};
    static COUNTER_DATA mutexes = {.key = "Mutexes"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &semaphores)) {
        ULONGLONG sem = (ULONGLONG)semaphores.current.Data;
        common_semaphore_ipc(sem, (NETDATA_DOUBLE)WINDOWS_MAX_KERNEL_OBJECT, _COMMON_PLUGIN_MODULE_NAME, update_every);
    }

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &mutexes)) {
        static RRDSET *st = NULL;
        static RRDDIM *rd = NULL;
        if (unlikely(!st)) {
            st = rrdset_create_localhost("system",
                                       "ipc_mutexes",
                                       NULL,
                                       "ipc",
                                       "system.ipc_mutexes",
                                       "IPC Mutexes",
                                       "mutexes",
                                       _COMMON_PLUGIN_NAME,
                                       _COMMON_PLUGIN_MODULE_NAME,
                                       NETDATA_CHART_PRIO_SYSTEM_IPC_OBJECTS,
                                       update_every,
                                       RRDSET_TYPE_AREA);
            rd = rrddim_add(st, "mutexes", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        rrddim_set_by_pointer(st, rd, (collected_number)mutexes.current.Data);
        rrdset_done(st);
    }

    return true;
}

int do_PerflibObjects(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Objects");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    if (!do_objects(pDataBlock, update_every))
        return -1;

    return 0;
}
