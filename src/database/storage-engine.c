// SPDX-License-Identifier: GPL-3.0-or-later

#include "storage-engine.h"
#include "ram/rrddim_mem.h"
#ifdef ENABLE_DBENGINE
#include "engine/rrdengineapi.h"
#endif

static STORAGE_ENGINE engines[] = {
    {
        .id = RRD_DB_MODE_NONE,
        .name = RRD_DB_MODE_NONE_NAME,
        .seb = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = {
            .metric_get_by_id = rrddim_metric_get_by_id,
            .metric_get_by_uuid = rrddim_metric_get_by_uuid,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_dup = rrddim_metric_dup,
            .metric_release = rrddim_metric_release,
            .metric_retention_by_id = rrddim_metric_retention_by_id,
            .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,
            .metric_retention_delete_by_id = rrddim_retention_delete_by_id,
        }
    },
    {
        .id = RRD_DB_MODE_RAM,
        .name = RRD_DB_MODE_RAM_NAME,
        .seb = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = {
            .metric_get_by_id = rrddim_metric_get_by_id,
            .metric_get_by_uuid = rrddim_metric_get_by_uuid,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_dup = rrddim_metric_dup,
            .metric_release = rrddim_metric_release,
            .metric_retention_by_id = rrddim_metric_retention_by_id,
            .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,
            .metric_retention_delete_by_id = rrddim_retention_delete_by_id,
        }
    },
    {
        .id = RRD_DB_MODE_ALLOC,
        .name = RRD_DB_MODE_ALLOC_NAME,
        .seb = STORAGE_ENGINE_BACKEND_RRDDIM,
        .api = {
            .metric_get_by_id = rrddim_metric_get_by_id,
            .metric_get_by_uuid = rrddim_metric_get_by_uuid,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_dup = rrddim_metric_dup,
            .metric_release = rrddim_metric_release,
            .metric_retention_by_id = rrddim_metric_retention_by_id,
            .metric_retention_by_uuid = rrddim_metric_retention_by_uuid,
            .metric_retention_delete_by_id = rrddim_retention_delete_by_id,
        }
    },
#ifdef ENABLE_DBENGINE
    {
        .id = RRD_DB_MODE_DBENGINE,
        .name = RRD_DB_MODE_DBENGINE_NAME,
        .seb = STORAGE_ENGINE_BACKEND_DBENGINE,
        .api = {
            .metric_get_by_id = rrdeng_metric_get_by_id,
            .metric_get_by_uuid = rrdeng_metric_get_by_uuid,
            .metric_get_or_create = rrdeng_metric_get_or_create,
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
