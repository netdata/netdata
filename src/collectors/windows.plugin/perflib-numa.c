// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct netdata_numa {
    RRDSET *st_numa;
    RRDDIM *rd_standby;
    RRDDIM *rd_free_zero;

    COUNTER_DATA standby;
    COUNTER_DATA free_zero;
};

static DICTIONARY *numa_dict = NULL;

static void numa_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct netdata_numa *d = value;

    d->standby.key = "Standby List MBytes";
    d->free_zero.key = "Free & Zero Page List MBytes";
}

static void initialize(void)
{
    numa_dict = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct netdata_numa));

    dictionary_register_insert_callback(numa_dict, numa_insert_cb, NULL);
}

static void netdata_numa_chart(struct netdata_numa *nn, int update_every)
{
    if (unlikely(!nn->st_numa)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "numa_node_%s_mem_usage", windows_shared_buffer);

        nn->st_numa = rrdset_create_localhost(
            "numa_node_mem_usage",
            id,
            NULL,
            "numa",
            "mem.numa_node_mem_usage",
            "NUMA Node Memory Usage",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibNUMA",
            NETDATA_CHART_PRIO_MEM_NUMA_NODES_MEMINFO,
            update_every,
            RRDSET_TYPE_LINE);

        rrdlabels_add(nn->st_numa->rrdlabels, "node", windows_shared_buffer, RRDLABEL_SRC_AUTO);

        nn->rd_free_zero = rrddim_add(nn->st_numa, "free", NULL, MEGA_FACTOR, 1, RRD_ALGORITHM_ABSOLUTE);

        nn->rd_standby = rrddim_add(nn->st_numa, "standby", NULL, MEGA_FACTOR, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(nn->st_numa, nn->rd_free_zero, (collected_number)nn->free_zero.current.Data);

    rrddim_set_by_pointer(nn->st_numa, nn->rd_standby, (collected_number)nn->standby.current.Data);

    rrdset_done(nn->st_numa);
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

        struct netdata_numa *nn = dictionary_set(numa_dict, windows_shared_buffer, NULL, sizeof(*nn));

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &nn->standby);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &nn->free_zero);
        netdata_numa_chart(nn, update_every);
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
