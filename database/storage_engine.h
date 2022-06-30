// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGEENGINEAPI_H
#define NETDATA_STORAGEENGINEAPI_H

#include "rrd.h"

typedef struct storage_engine STORAGE_ENGINE;

// ------------------------------------------------------------------------
// function pointers for all APIs provided by a storge engine
typedef struct storage_engine_api {
    void *(*init)(RRDDIM *rd, void *instance, int type);
    void (*free)(void *);
    struct rrddim_collect_ops collect_ops;
    struct rrddim_query_ops query_ops;
} STORAGE_ENGINE_API;

struct storage_engine {
    RRD_MEMORY_MODE id;
    const char* name;
    STORAGE_ENGINE_API api;
};

extern STORAGE_ENGINE* storage_engine_get(RRD_MEMORY_MODE mmode);
extern STORAGE_ENGINE* storage_engine_find(const char* name);

// Iterator over existing engines
extern STORAGE_ENGINE* storage_engine_foreach_init();
extern STORAGE_ENGINE* storage_engine_foreach_next(STORAGE_ENGINE* it);

#endif
