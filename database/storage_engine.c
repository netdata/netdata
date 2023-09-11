// SPDX-License-Identifier: GPL-3.0-or-later

#include "storage_engine.h"
#include "ram/rrddim_mem.h"
#ifdef ENABLE_DBENGINE
#include "engine/rrdengineapi.h"
#endif

static STORAGE_ENGINE engines[] = {
    {
        .id = RRD_MEMORY_MODE_NONE,
        .name = RRD_MEMORY_MODE_NONE_NAME,
        .backend = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_dup = rrddim_metric_dup,
            .metric_release = rrddim_metric_release,
            .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,
        }
    },
    {
        .id = RRD_MEMORY_MODE_RAM,
        .name = RRD_MEMORY_MODE_RAM_NAME,
        .backend = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_dup = rrddim_metric_dup,
            .metric_release = rrddim_metric_release,
            .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,
        }
    },
    {
        .id = RRD_MEMORY_MODE_MAP,
        .name = RRD_MEMORY_MODE_MAP_NAME,
        .backend = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_dup = rrddim_metric_dup,
            .metric_release = rrddim_metric_release,
            .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,
        }
    },
    {
        .id = RRD_MEMORY_MODE_SAVE,
        .name = RRD_MEMORY_MODE_SAVE_NAME,
        .backend = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_dup = rrddim_metric_dup,
            .metric_release = rrddim_metric_release,
            .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,
        }
    },
    {
        .id = RRD_MEMORY_MODE_ALLOC,
        .name = RRD_MEMORY_MODE_ALLOC_NAME,
        .backend = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_dup = rrddim_metric_dup,
            .metric_release = rrddim_metric_release,
            .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,
        }
    },
#ifdef ENABLE_DBENGINE
    {
        .id = RRD_MEMORY_MODE_DBENGINE,
        .name = RRD_MEMORY_MODE_DBENGINE_NAME,
        .backend = STORAGE_ENGINE_BACKEND_DBENGINE,
        .api = {
            .metric_get = rrdeng_metric_get,
            .metric_get_or_create = rrdeng_metric_get_or_create,
            .metric_dup = rrdeng_metric_dup,
            .metric_release = rrdeng_metric_release,
            .metric_retention_by_uuid = rrdeng_metric_retention_by_uuid,
        }
    },
#endif
    { .id = RRD_MEMORY_MODE_NONE, .name = NULL }
};

STORAGE_ENGINE* storage_engine_find(const char* name)
{
    for (STORAGE_ENGINE* it = engines; it->name; it++) {
        if (strcmp(it->name, name) == 0)
            return it;
    }
    return NULL;
}

STORAGE_ENGINE* storage_engine_get(RRD_MEMORY_MODE mmode)
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
