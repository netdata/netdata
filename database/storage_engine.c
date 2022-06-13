// SPDX-License-Identifier: GPL-3.0-or-later

#include "storage_engine.h"
#include "ram/rrddim_mem.h"
#ifdef ENABLE_DBENGINE
#include "engine/rrdengineapi.h"
#endif
#include "libnetdata/libnetdata.h"

static STORAGE_ENGINE engines[] = {
    {
        .id = RRD_MEMORY_MODE_NONE,
        .name = RRD_MEMORY_MODE_NONE_NAME,
        .api = {
            .engine_ops = {
                .create = rrddim_storage_engine_instance_new,
                .exit = rrddim_storage_engine_instance_exit,
                .destroy = rrddim_storage_engine_instance_destroy
            },
            .collect_ops = {
                .init = rrddim_collect_init,
                .store_metric = rrddim_collect_store_metric,
                .finalize = rrddim_collect_finalize
            },
            .query_ops = {
                .init = rrddim_query_init,
                .next_metric = rrddim_query_next_metric,
                .is_finished = rrddim_query_is_finished,
                .finalize = rrddim_query_finalize,
                .latest_time = rrddim_query_latest_time,
                .oldest_time = rrddim_query_oldest_time
            }
        },
        .context = NULL
    },
    {
        .id = RRD_MEMORY_MODE_RAM,
        .name = RRD_MEMORY_MODE_RAM_NAME,
        .api = {
            .engine_ops = {
                .create = rrddim_storage_engine_instance_new,
                .exit = rrddim_storage_engine_instance_exit,
                .destroy = rrddim_storage_engine_instance_destroy
            },
            .collect_ops = {
                .init = rrddim_collect_init,
                .store_metric = rrddim_collect_store_metric,
                .finalize = rrddim_collect_finalize
            },
            .query_ops = {
                .init = rrddim_query_init,
                .next_metric = rrddim_query_next_metric,
                .is_finished = rrddim_query_is_finished,
                .finalize = rrddim_query_finalize,
                .latest_time = rrddim_query_latest_time,
                .oldest_time = rrddim_query_oldest_time
            }
        },
        .context = NULL
    },
    {
        .id = RRD_MEMORY_MODE_MAP,
        .name = RRD_MEMORY_MODE_MAP_NAME,
        .api = {
            .engine_ops = {
                .create = rrddim_storage_engine_instance_new,
                .exit = rrddim_storage_engine_instance_exit,
                .destroy = rrddim_storage_engine_instance_destroy
            },
            .collect_ops = {
                .init = rrddim_collect_init,
                .store_metric = rrddim_collect_store_metric,
                .finalize = rrddim_collect_finalize
            },
            .query_ops = {
                .init = rrddim_query_init,
                .next_metric = rrddim_query_next_metric,
                .is_finished = rrddim_query_is_finished,
                .finalize = rrddim_query_finalize,
                .latest_time = rrddim_query_latest_time,
                .oldest_time = rrddim_query_oldest_time
            }
        },
        .context = NULL
    },
    {
        .id = RRD_MEMORY_MODE_SAVE,
        .name = RRD_MEMORY_MODE_SAVE_NAME,
        .api = {
            .engine_ops = {
                .create = rrddim_storage_engine_instance_new,
                .exit = rrddim_storage_engine_instance_exit,
                .destroy = rrddim_storage_engine_instance_destroy
            },
            .collect_ops = {
                .init = rrddim_collect_init,
                .store_metric = rrddim_collect_store_metric,
                .finalize = rrddim_collect_finalize
            },
            .query_ops = {
                .init = rrddim_query_init,
                .next_metric = rrddim_query_next_metric,
                .is_finished = rrddim_query_is_finished,
                .finalize = rrddim_query_finalize,
                .latest_time = rrddim_query_latest_time,
                .oldest_time = rrddim_query_oldest_time
            }
        },
        .context = NULL
    },
    {
        .id = RRD_MEMORY_MODE_ALLOC,
        .name = RRD_MEMORY_MODE_ALLOC_NAME,
        .api = {
            .engine_ops = {
                .create = rrddim_storage_engine_instance_new,
                .exit = rrddim_storage_engine_instance_exit,
                .destroy = rrddim_storage_engine_instance_destroy
            },
            .collect_ops = {
                .init = rrddim_collect_init,
                .store_metric = rrddim_collect_store_metric,
                .finalize = rrddim_collect_finalize
            },
            .query_ops = {
                .init = rrddim_query_init,
                .next_metric = rrddim_query_next_metric,
                .is_finished = rrddim_query_is_finished,
                .finalize = rrddim_query_finalize,
                .latest_time = rrddim_query_latest_time,
                .oldest_time = rrddim_query_oldest_time
            }
        },
        .context = NULL
    },
#ifdef ENABLE_DBENGINE
    {
        .id = RRD_MEMORY_MODE_DBENGINE,
        .name = RRD_MEMORY_MODE_DBENGINE_NAME,
        .api = {
            .engine_ops = {
                .create = rrdeng_init,
                .exit = rrdeng_prepare_exit,
                .destroy = rrdeng_exit
            },
            .collect_ops = {
                .init = rrdeng_store_metric_init,
                .store_metric = rrdeng_store_metric_next,
                .finalize = rrdeng_store_metric_finalize
            },
            .query_ops = {
                .init = rrdeng_load_metric_init,
                .next_metric = rrdeng_load_metric_next,
                .is_finished = rrdeng_load_metric_is_finished,
                .finalize = rrdeng_load_metric_finalize,
                .latest_time = rrdeng_metric_latest_time,
                .oldest_time = rrdeng_metric_oldest_time
            }
        },
        .context = NULL
    }
#endif
};

const size_t engine_count = sizeof(engines) / sizeof(engines[0]);

STORAGE_ENGINE* storage_engine_find(const char* name)
{
    for (size_t i = 0; i != engine_count; i++)
        if (strcmp(engines[i].name, name) == 0)
            return &engines[i];

    error("No storage engine found for memory mode %s.", name);
    return storage_engine_get(default_rrd_memory_mode);
}

STORAGE_ENGINE* storage_engine_get(RRD_MEMORY_MODE mmode)
{
    for (size_t i = 0; i != engine_count; i++)
        if (engines[i].id == mmode)
            return &engines[i];
    STORAGE_ENGINE* eng = mmode == default_rrd_memory_mode
        ? &engines[0] // default engine not available, use NONE
        : storage_engine_get(default_rrd_memory_mode);
    error("No storage engine for memory mode %u, will use %s (%u) instead.", mmode, eng->name, eng->id);
    return eng;
}

STORAGE_ENGINE* storage_engine_foreach_init()
{
    // Assuming at least one engine exists
    return &engines[0];
}

STORAGE_ENGINE* storage_engine_foreach_next(STORAGE_ENGINE* it)
{
    it++;
    return (it >= &engines[engine_count]) ? NULL : it;
}

STORAGE_ENGINE_INSTANCE* storage_engine_new(STORAGE_ENGINE* eng, RRDHOST *host)
{
    STORAGE_ENGINE_INSTANCE* instance = host->rrdeng_ctx;
    if (!instance && eng) {
        instance = eng->api.engine_ops.create(eng, host);
        if (instance) {
            instance->engine = eng;
        }
    }
    return instance;
}

void storage_engine_delete(STORAGE_ENGINE_INSTANCE* instance) {
    if (instance) {
        STORAGE_ENGINE* eng = instance->engine;
        if (eng) {
            eng->api.engine_ops.destroy(instance);
        }
    }
}
