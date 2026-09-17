// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "storage-engine.h"
#include "ram/rrddim_mem.h"
#ifdef ENABLE_DBENGINE
#include "database/engine/include/dbengine/rrdengineapi.h"

// The vtable hands over the RRDDIM because the RAM backend keeps a reference to it;
// dbengine only ever needs the dimension's uuid.
static STORAGE_METRIC_HANDLE *dbengine_metric_get_or_create(RRDDIM *rd, STORAGE_INSTANCE *si) {
    return rrdeng_metric_get_or_create_by_id(si, rd->uuid);
}
#endif

// The three in-memory modes share one implementation. A macro, not a static const
// object: a static initializer must be a constant expression on every compiler we build with.
#define RRDDIM_STORAGE_ENGINE_API {                                         \
    .metric_get_by_id = rrddim_metric_get_by_id,                            \
    .metric_get_by_uuid = rrddim_metric_get_by_uuid,                        \
    .metric_get_or_create = rrddim_metric_get_or_create,                    \
    .metric_dup = rrddim_metric_dup,                                        \
    .metric_release = rrddim_metric_release,                                \
    .metric_retention_by_id = rrddim_metric_retention_by_id,                \
    .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,            \
    .metric_retention_delete_by_id = rrddim_retention_delete_by_id,         \
}

static STORAGE_ENGINE engines[] = {
    {
        .id = RRD_DB_MODE_NONE,
        .name = RRD_DB_MODE_NONE_NAME,
        .seb = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = RRDDIM_STORAGE_ENGINE_API,
    },
    {
        .id = RRD_DB_MODE_RAM,
        .name = RRD_DB_MODE_RAM_NAME,
        .seb = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = RRDDIM_STORAGE_ENGINE_API,
    },
    {
        .id = RRD_DB_MODE_ALLOC,
        .name = RRD_DB_MODE_ALLOC_NAME,
        .seb = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = RRDDIM_STORAGE_ENGINE_API,
    },
#ifdef ENABLE_DBENGINE
    {
        .id = RRD_DB_MODE_DBENGINE,
        .name = RRD_DB_MODE_DBENGINE_NAME,
        .seb = STORAGE_ENGINE_BACKEND_DBENGINE,
        .api = {
            .metric_get_by_id = rrdeng_metric_get_by_id,
            .metric_get_by_uuid = rrdeng_metric_get_by_uuid,
            .metric_get_or_create = dbengine_metric_get_or_create,
            .metric_dup = rrdeng_metric_dup,
            .metric_release = rrdeng_metric_release,
            .metric_retention_by_id = rrdeng_metric_retention_by_id,
            .metric_retention_by_uuid = rrdeng_metric_retention_by_uuid,
            .metric_retention_delete_by_id = rrdeng_metric_retention_delete_by_id,
        }
    },
#endif
    { .id = RRD_DB_MODE_NONE, .name = NULL }
};

STORAGE_ENGINE* storage_engine_find(const char* name)
{
    for (STORAGE_ENGINE* it = engines; it->name; it++) {
        if (strcmp(it->name, name) == 0)
            return it;
    }
    return NULL;
}

STORAGE_ENGINE* storage_engine_get(RRD_DB_MODE mmode)
{
    for (STORAGE_ENGINE* it = engines; it->name; it++) {
        if (it->id == mmode)
            return it;
    }
    return NULL;
}

STORAGE_ENGINE* storage_engine_foreach_init()
{
    // Assuming at least one engine exists
    return &engines[0];
}

STORAGE_ENGINE* storage_engine_foreach_next(STORAGE_ENGINE* it)
{
    if (!it || !it->name)
        return NULL;

    it++;
    return it->name ? it : NULL;
}
