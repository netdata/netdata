// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGEENGINEAPI_H
#define NETDATA_STORAGEENGINEAPI_H

#include "rrd.h"

typedef struct storage_engine STORAGE_ENGINE;

// ------------------------------------------------------------------------
// function pointers for all APIs provided by a storge engine
typedef struct storage_engine_api {
    // metric management
    STORAGE_METRIC_HANDLE *(*metric_get)(STORAGE_INSTANCE *instance, uuid_t *uuid, STORAGE_METRICS_GROUP *smg);
    STORAGE_METRIC_HANDLE *(*metric_get_or_create)(RRDDIM *rd, STORAGE_INSTANCE *instance, STORAGE_METRICS_GROUP *smg);
    void (*metric_release)(STORAGE_METRIC_HANDLE *);

    // metrics groups management
    STORAGE_METRICS_GROUP *(*group_get)(STORAGE_INSTANCE *db_instance, uuid_t *uuid);
    void (*group_release)(STORAGE_INSTANCE *db_instance, STORAGE_METRICS_GROUP *smg);

    // operations
    struct rrddim_collect_ops collect_ops;
    struct rrddim_query_ops query_ops;
} STORAGE_ENGINE_API;

struct storage_engine {
    RRD_MEMORY_MODE id;
    const char* name;
    STORAGE_ENGINE_API api;
};

STORAGE_ENGINE* storage_engine_get(RRD_MEMORY_MODE mmode);
STORAGE_ENGINE* storage_engine_find(const char* name);

// Iterator over existing engines
STORAGE_ENGINE* storage_engine_foreach_init();
STORAGE_ENGINE* storage_engine_foreach_next(STORAGE_ENGINE* it);

#endif
