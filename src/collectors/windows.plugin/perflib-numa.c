// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct netdata_numa {
    RRDSET *st_standby;
    RRDDIM *rd_standby;

    RRDSET *st_available;
    RRDDIM *rd_available;

    RRDSET *st_free_zero;
    RRDDIM *rd_free_zero;

    COUNTER_DATA standby;
    COUNTER_DATA available;
    COUNTER_DATA free_zero;
};

static DICTIONARY *numa_dict = NULL;

static void
numa_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct netdata_numa *d = value;

    d->standby.key = "";
    d->available.key = "";
    d->free_zero.key = "";
}

static void initialize(void)
{
    numa_dict = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct netdata_numa));

    dictionary_register_insert_callback(numa_dict, numa_insert_cb, NULL);
}

static bool do_numa(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "NUMA Node Memory");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct netdata_numa *nn = dictionary_set(dict, windows_shared_buffer, NULL, sizeof(*d));
        if (!nn)
            continue;
    }

    return true;
}

int do_PerflibNUMA(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("NUMA Node Memory");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    do_numa(pDataBlock, update_every);

    return 0;
}
