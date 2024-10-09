// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

enum netdata_netframework_metrics {
    NETDATA_NETFRAMEWORK_EXCEPTIONS,
    NETDATA_NETFRAMEWORK_INTEROP,
    NETDATA_NETFRAMEWORK_JIT,
    NETDATA_NETFRAMEWORK_LOADING,

    NETDATA_NETFRAMEWORK_END
};

struct net_framework_instances {
    RRDSET *st_clrexception_thrown;
    RRDDIM *rd_clrexception_thrown;

    COUNTER_DATA NETFrameworkCLRExceptionThrown;
    COUNTER_DATA NETFrameworkCLRExceptionFilters;
    COUNTER_DATA NETFrameworkCLRExceptionFinallys;
    COUNTER_DATA NETFrameworkCLRExceptionTotalCatchDepth;
};

static inline void initialize_net_framework_processes_keys(struct net_framework_instances *p) {
    p->NETFrameworkCLRExceptionFilters.key = "# of Filters / sec";
    p->NETFrameworkCLRExceptionFinallys.key = "# of Finallys / sec";
    p->NETFrameworkCLRExceptionThrown.key = "# of Exceps Thrown / sec";
    p->NETFrameworkCLRExceptionTotalCatchDepth.key = "Throw To Catch Depth / sec";
}

void dict_net_framework_processes_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct net_framework_instances *p = value;
    initialize_net_framework_processes_keys(p);
}

static DICTIONARY *processes = NULL;

static void initialize(void) {
    processes = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                               DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct net_framework_instances));

    dictionary_register_insert_callback(processes, dict_net_framework_processes_insert_cb, NULL);
}

static void netdata_framework_clr_exceptions(PERF_DATA_BLOCK *pDataBlock,
                                             PERF_OBJECT_TYPE *pObjectType,
                                             char *object_name,
                                             int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global") == 0)
            continue;
    }
}

static void netdata_framework_clr_interop(PERF_DATA_BLOCK *pDataBlock,
                                          PERF_OBJECT_TYPE *pObjectType,
                                          char *object_name,
                                          int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global") == 0)
            continue;
    }
}

static void netdata_framework_clr_jit(PERF_DATA_BLOCK *pDataBlock,
                                      PERF_OBJECT_TYPE *pObjectType,
                                      char *object_name,
                                      int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global") == 0)
            continue;
    }
}

static void netdata_framework_clr_loading(PERF_DATA_BLOCK *pDataBlock,
                                          PERF_OBJECT_TYPE *pObjectType,
                                          char *object_name,
                                          int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if(strcasecmp(windows_shared_buffer, "_Global") == 0)
            continue;
    }
}

struct netdata_netframework_objects {
    char *object;
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, char *, int);
} netframewrk_obj[NETDATA_NETFRAMEWORK_END] = {
    {
        .fnct = netdata_framework_clr_exceptions,
        .object = ".NET CLR Exceptions"
    },
    {
        .fnct = netdata_framework_clr_interop,
        .object = ".NET CLR Interop"
    },
    {
        .fnct = netdata_framework_clr_jit,
        .object = ".NET CLR Jit"
    },
    {
        .fnct = netdata_framework_clr_loading,
        .object = ".NET CLR Loading"
    }
};

int do_PerflibNetFramework(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    int i;
    for (i = 0; i < NETDATA_NETFRAMEWORK_END; i++) {
        DWORD id = RegistryFindIDByName(netframewrk_obj[i].object);
        if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            continue;

        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if(!pDataBlock) continue;

        PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, netframewrk_obj[i].object);
        if(!pObjectType) continue;

        netframewrk_obj[i].fnct(pDataBlock, pObjectType, netframewrk_obj[i].object, update_every);
    }

    return 0;
}
