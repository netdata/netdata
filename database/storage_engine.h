// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGEENGINEAPI_H
#define NETDATA_STORAGEENGINEAPI_H

#include "rrd.h"

typedef struct storage_engine STORAGE_ENGINE;
typedef struct storage_engine_instance STORAGE_ENGINE_INSTANCE;

// ------------------------------------------------------------------------
// function pointers that handle storage engine instance creation and destruction
struct storage_engine_ops {
    STORAGE_ENGINE_INSTANCE*(*create)(STORAGE_ENGINE* engine, RRDHOST *host);
    void(*exit)(STORAGE_ENGINE_INSTANCE*);
    void(*destroy)(STORAGE_ENGINE_INSTANCE*);
};

// ------------------------------------------------------------------------
// function pointers that handle RRDSET creation, destruction and query
struct rrdset_ops {
    RRDSET*(*create)(const char *id, const char *fullid, const char *filename, long entries, int update_every);
    void(*destroy)(RRDSET *rd);
};

// ------------------------------------------------------------------------
// function pointers that handle RRDDIM creation and destruction
struct rrddim_ops {
    RRDDIM*(*create)(RRDSET *st, const char *id, const char *filename, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm);
    void(*init)(RRDDIM* rd);
    void(*destroy)(RRDDIM *rd);
};

// ------------------------------------------------------------------------
// function pointers for all APIs provided by a storge engine
typedef struct storage_engine_api {
    struct storage_engine_ops engine_ops;
    struct rrdset_ops set_ops;
    struct rrddim_ops dim_ops;
    struct rrddim_collect_ops collect_ops;
    struct rrddim_query_ops query_ops;
} STORAGE_ENGINE_API;

struct storage_engine {
    RRD_MEMORY_MODE id;
    const char* name;
    STORAGE_ENGINE_API api;
    STORAGE_ENGINE_INSTANCE* context;
};

// Abstract structure to be extended by implementations
struct storage_engine_instance {
    STORAGE_ENGINE* engine;
};

extern STORAGE_ENGINE* storage_engine_get(RRD_MEMORY_MODE mmode);
extern STORAGE_ENGINE* storage_engine_find(const char* name);

// Iterator over existing engines
extern STORAGE_ENGINE* storage_engine_foreach_init();
extern STORAGE_ENGINE* storage_engine_foreach_next(STORAGE_ENGINE* it);

// ------------------------------------------------------------------------
// Retreive or create a storage engine instance for host
STORAGE_ENGINE_INSTANCE* storage_engine_new(STORAGE_ENGINE* engine, RRDHOST *host);
void storage_engine_delete(STORAGE_ENGINE_INSTANCE* engine);

#endif
