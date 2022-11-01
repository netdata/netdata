// SPDX-License-Identifier: GPL-3.0-or-later

#include "storage_engine.h"
#include "ram/rrddim_mem.h"
#ifdef ENABLE_DBENGINE
#include "engine/rrdengineapi.h"
#endif

#define im_collect_ops { \
    .init = rrddim_collect_init,\
    .store_metric = rrddim_collect_store_metric,\
    .flush = rrddim_store_metric_flush,\
    .finalize = rrddim_collect_finalize, \
    .change_collection_frequency = rrddim_store_metric_change_collection_frequency, \
    .metrics_group_get = rrddim_metrics_group_get, \
    .metrics_group_release = rrddim_metrics_group_release, \
}

#define im_query_ops { \
    .init = rrddim_query_init, \
    .next_metric = rrddim_query_next_metric, \
    .is_finished = rrddim_query_is_finished, \
    .finalize = rrddim_query_finalize, \
    .latest_time = rrddim_query_latest_time, \
    .oldest_time = rrddim_query_oldest_time \
}

static STORAGE_ENGINE engines[] = {
    {
        .id = RRD_MEMORY_MODE_NONE,
        .name = RRD_MEMORY_MODE_NONE_NAME,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_release = rrddim_metric_release,
            .group_get = rrddim_metrics_group_get,
            .group_release = rrddim_metrics_group_release,
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        }
    },
    {
        .id = RRD_MEMORY_MODE_RAM,
        .name = RRD_MEMORY_MODE_RAM_NAME,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_release = rrddim_metric_release,
            .group_get = rrddim_metrics_group_get,
            .group_release = rrddim_metrics_group_release,
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        }
    },
    {
        .id = RRD_MEMORY_MODE_MAP,
        .name = RRD_MEMORY_MODE_MAP_NAME,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_release = rrddim_metric_release,
            .group_get = rrddim_metrics_group_get,
            .group_release = rrddim_metrics_group_release,
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        }
    },
    {
        .id = RRD_MEMORY_MODE_SAVE,
        .name = RRD_MEMORY_MODE_SAVE_NAME,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_release = rrddim_metric_release,
            .group_get = rrddim_metrics_group_get,
            .group_release = rrddim_metrics_group_release,
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        }
    },
    {
        .id = RRD_MEMORY_MODE_ALLOC,
        .name = RRD_MEMORY_MODE_ALLOC_NAME,
        .api = {
            .metric_get = rrddim_metric_get,
            .metric_get_or_create = rrddim_metric_get_or_create,
            .metric_release = rrddim_metric_release,
            .group_get = rrddim_metrics_group_get,
            .group_release = rrddim_metrics_group_release,
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        }
    },
#ifdef ENABLE_DBENGINE
    {
        .id = RRD_MEMORY_MODE_DBENGINE,
        .name = RRD_MEMORY_MODE_DBENGINE_NAME,
        .api = {
            .metric_get = rrdeng_metric_get,
            .metric_get_or_create = rrdeng_metric_get_or_create,
            .metric_release = rrdeng_metric_release,
            .group_get = rrdeng_metrics_group_get,
            .group_release = rrdeng_metrics_group_release,
            .collect_ops = {
                .init = rrdeng_store_metric_init,
                .store_metric = rrdeng_store_metric_next,
                .flush = rrdeng_store_metric_flush_current_page,
                .finalize = rrdeng_store_metric_finalize,
                .change_collection_frequency = rrdeng_store_metric_change_collection_frequency,
                .metrics_group_get = rrdeng_metrics_group_get,
                .metrics_group_release = rrdeng_metrics_group_release,
            },
            .query_ops = {
                .init = rrdeng_load_metric_init,
                .next_metric = rrdeng_load_metric_next,
                .is_finished = rrdeng_load_metric_is_finished,
                .finalize = rrdeng_load_metric_finalize,
                .latest_time = rrdeng_metric_latest_time,
                .oldest_time = rrdeng_metric_oldest_time
            }
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
