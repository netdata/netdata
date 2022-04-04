// SPDX-License-Identifier: GPL-3.0-or-later

#include "storage_engine.h"
#include "rrddim_mem.h"
#ifdef ENABLE_DBENGINE
#include "engine/rrdengineapi.h"
#endif

static void dimension_destroy_freez(RRDDIM *rd) {
    freez(rd);
}
static void dimension_destroy_unmap(RRDDIM *rd) {
    munmap(rd, rd->memsize);
}
static void set_destroy_freez(RRDSET *st) {
    freez(st);
}
static void set_destroy_unmap(RRDSET *st) {
    munmap(st, st->memsize);
}

// Initialize a rrdset with calloc and the provided memory mode
static RRDSET* set_init_calloc(RRD_MEMORY_MODE mode) {
    RRDSET* st = callocz(1, sizeof(RRDSET));
    st->rrd_memory_mode = mode;
    return st;
}

#define SET_INIT(MMODE) \
static RRDSET* rrdset_init_##MMODE(const char *id, const char *fullid, const char *filename, long entries, int update_every) { \
    return set_init_calloc(RRD_MEMORY_MODE_##MMODE); \
}
SET_INIT(NONE)
SET_INIT(ALLOC)
SET_INIT(DBENGINE)

// Initialize a rrddim with calloc and the provided memory mode
static RRDDIM* dim_init_calloc(RRDSET *st, RRD_MEMORY_MODE mode) {
    size_t size = sizeof(RRDDIM) + (st->entries * sizeof(storage_number));
    RRDDIM* rd = callocz(1, size);
    rd->memsize = size;
    rd->rrd_memory_mode = mode;
    return rd;
}

#define DIM_INIT(MMODE) \
static RRDDIM* rrddim_init_##MMODE(RRDSET *st, const char *id, const char *filename, collected_number multiplier, collected_number divisor, RRD_ALGORITHM algorithm) { \
    return dim_init_calloc((st), RRD_MEMORY_MODE_##MMODE); \
}
DIM_INIT(NONE)
DIM_INIT(ALLOC)
DIM_INIT(DBENGINE)

static const struct rrddim_collect_ops im_collect_ops = {
    .init = rrddim_collect_init,
    .store_metric = rrddim_collect_store_metric,
    .finalize = rrddim_collect_finalize
};

static const struct rrddim_query_ops im_query_ops = {
    .init = rrddim_query_init,
    .next_metric = rrddim_query_next_metric,
    .is_finished = rrddim_query_is_finished,
    .finalize = rrddim_query_finalize,
    .latest_time = rrddim_query_latest_time,
    .oldest_time = rrddim_query_oldest_time
};

STORAGE_ENGINE engines[] = {
    {
        .id = RRD_MEMORY_MODE_NONE,
        .name = RRD_MEMORY_MODE_NONE_NAME,
        .api = {
            .engine_ops = {
                .create = NULL,
                .exit = NULL,
                .destroy = NULL
            },
            .set_ops = {
                .create = rrdset_init_NONE,
                .destroy = set_destroy_freez,
            },
            .dim_ops = {
                .create = rrddim_init_NONE,
                .init = NULL,
                .destroy = dimension_destroy_freez,
            },
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        },
        .instance_per_host = true,
        .multidb_instance = NULL
    },
    {
        .id = RRD_MEMORY_MODE_RAM,
        .name = RRD_MEMORY_MODE_RAM_NAME,
        .api = {
            .engine_ops = {
                .create = NULL,
                .exit = NULL,
                .destroy = NULL
            },
            .set_ops = {
                .create = rrdset_init_ram,
                .destroy = set_destroy_unmap,
            },
            .dim_ops = {
                .create = rrddim_init_ram,
                .init = NULL,
                .destroy = dimension_destroy_unmap,
            },
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        },
        .instance_per_host = true,
        .multidb_instance = NULL
    },
    {
        .id = RRD_MEMORY_MODE_MAP,
        .name = RRD_MEMORY_MODE_MAP_NAME,
        .api = {
            .engine_ops = {
                .create = NULL,
                .exit = NULL,
                .destroy = NULL
            },
            .set_ops = {
                .create = rrdset_init_map,
                .destroy = set_destroy_unmap,
            },
            .dim_ops = {
                .create = rrddim_init_map,
                .init = NULL,
                .destroy = dimension_destroy_unmap,
            },
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        },
        .instance_per_host = true,
        .multidb_instance = NULL
    },
    {
        .id = RRD_MEMORY_MODE_SAVE,
        .name = RRD_MEMORY_MODE_SAVE_NAME,
        .api = {
            .engine_ops = {
                .create = NULL,
                .exit = NULL,
                .destroy = NULL
            },
            .set_ops = {
                .create = rrdset_init_save,
                .destroy = set_destroy_unmap,
            },
            .dim_ops = {
                .create = rrddim_init_save,
                .init = NULL,
                .destroy = dimension_destroy_unmap,
            },
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        },
        .instance_per_host = true,
        .multidb_instance = NULL
    },
    {
        .id = RRD_MEMORY_MODE_ALLOC,
        .name = RRD_MEMORY_MODE_ALLOC_NAME,
        .api = {
            .engine_ops = {
                .create = NULL,
                .exit = NULL,
                .destroy = NULL
            },
            .set_ops = {
                .create = rrdset_init_ALLOC,
                .destroy = set_destroy_unmap,
            },
            .dim_ops = {
                .create = rrddim_init_ALLOC,
                .init = NULL,
                .destroy = dimension_destroy_unmap,
            },
            .collect_ops = im_collect_ops,
            .query_ops = im_query_ops
        },
        .instance_per_host = true,
        .multidb_instance = NULL
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
            .set_ops = {
                .create = rrdset_init_DBENGINE,
                .destroy = set_destroy_freez,
            },
            .dim_ops = {
                .create = rrddim_init_DBENGINE,
                .init = rrdeng_metric_init,
                .destroy = dimension_destroy_freez,
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
        .instance_per_host = false,
        .multidb_instance = NULL
    },
#endif
    { .id = RRD_MEMORY_MODE_NONE, .name = NULL }
};

STORAGE_ENGINE* engine_find(const char* name)
{
    for (STORAGE_ENGINE* it = engines; it->name; it++) {
        if (strcmp(it->name, name) == 0)
            return it;
    }
    return NULL;
}

STORAGE_ENGINE* engine_get(RRD_MEMORY_MODE mmode)
{
    for (STORAGE_ENGINE* it = engines; it->name; it++) {
        if (it->id == mmode)
            return it;
    }
    return NULL;
}

STORAGE_ENGINE* engine_foreach_init()
{
    // Assuming at least one engine exists
    return &engines[0];
}

STORAGE_ENGINE* engine_foreach_next(STORAGE_ENGINE* it)
{
    if (!it || !it->name)
        return NULL;

    it++;
    return it->name ? it : NULL;
}

STORAGE_ENGINE_INSTANCE* engine_new(STORAGE_ENGINE* eng, RRDHOST *host, bool force_new)
{
    STORAGE_ENGINE_INSTANCE* instance = NULL;
    if (eng) {
        bool multidb = !force_new && !eng->instance_per_host;
        if (multidb && eng->multidb_instance) {
            instance = eng->multidb_instance;
        } else {
            if (eng->api.engine_ops.create) {
                instance = eng->api.engine_ops.create(eng, host);
            }
            if (instance) {
                instance->engine = eng;
                if (multidb) {
                    eng->multidb_instance = instance;
                }
            }
        }
    }
    //host->rrdeng_ctx = instance;
    return instance;
}

void engine_delete(STORAGE_ENGINE_INSTANCE* instance) {
    if (instance) {
        STORAGE_ENGINE* eng = instance->engine;
        if (instance == eng->multidb_instance) {
            eng->multidb_instance = NULL;
        }
        if (eng && eng->api.engine_ops.destroy) {
            eng->api.engine_ops.destroy(instance);
        }
    }
}
