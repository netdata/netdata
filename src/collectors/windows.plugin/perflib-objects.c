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

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &semaphores)) {
        ULONGLONG sem = (ULONGLONG)semaphores.current.Data;
        common_semaphore_ipc(sem, (NETDATA_DOUBLE)WINDOWS_MAX_KERNEL_OBJECT, _COMMON_PLUGIN_MODULE_NAME, update_every);
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

    if (do_objects(pDataBlock, update_every))
        return -1;

    return 0;
}
