// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDSET_INDEX_ID_H
#define NETDATA_RRDSET_INDEX_ID_H

#include "rrd.h"

RRDSET *rrdset_create_custom(
    RRDHOST *host
    , const char *type
    , const char *id
    , const char *name
    , const char *family
    , const char *context
    , const char *title
    , const char *units
    , const char *plugin
    , const char *module
    , long priority
    , int update_every
    , RRDSET_TYPE chart_type
    ,
    RRD_DB_MODE memory_mode
    , long history_entries
);

static inline
    RRDSET *rrdset_create(RRDHOST *host
                  , const char *type
                  , const char *id
                  , const char *name
                  , const char *family
                  , const char *context
                  , const char *title
                  , const char *units
                  , const char *plugin
                  , const char *module
                  , long priority
                  , int update_every
                  , RRDSET_TYPE chart_type) {
    return rrdset_create_custom(
        host, type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type, (host)->rrd_memory_mode, (host)->rrd_history_entries);
}

static inline
    RRDSET *rrdset_create_localhost(
        const char *type
        , const char *id
        , const char *name
        , const char *family
        , const char *context
        , const char *title
        , const char *units
        , const char *plugin
        , const char *module
        , long priority
        , int update_every
        , RRDSET_TYPE chart_type) {
    return rrdset_create(
        localhost, type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type);
}

void rrdset_index_init(RRDHOST *host);
void rrdset_index_flush(RRDHOST *host);
void rrdset_index_destroy(RRDHOST *host);

void rrdset_free(RRDSET *st);

RRDSET *rrdset_find(RRDHOST *host, const char *id, bool include_obsolete);
RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id, bool include_obsolete);

RRDSET_ACQUIRED *rrdset_acquire(RRDHOST *host, const char *id);
RRDSET_ACQUIRED *rrdset_find_and_acquire(RRDHOST *host, const char *id, bool include_obsolete);

void rrdset_acquired_release(RRDSET_ACQUIRED *rsa);
RRDSET *rrdset_acquired_to_rrdset(RRDSET_ACQUIRED *rsa);

uint16_t rrddim_collection_modulo(RRDSET *st, uint32_t spread);

// The seed of a chart's dbengine page phase: the slot of absolute time its pages close at. It depends only on the
// chart's identity, so the phase survives restarts, and it spreads charts uniformly over the slots.
static inline uint64_t rrdset_storage_phase_seed(const ND_UUID *host_id, const char *chart_full_id) {
    return XXH3_64bits_withSeed(chart_full_id, strlen(chart_full_id), XXH3_64bits(host_id, sizeof(*host_id)));
}

/* This will not return charts that are obsolete */
ALWAYS_INLINE
static RRDSET *rrdset_find_active_localhost(const char *id) {
    RRDSET *st = rrdset_find(localhost, id, false);
    return st;
}

/* This will not return charts that are obsolete */
ALWAYS_INLINE
static RRDSET *rrdset_find_active_bytype_localhost(const char *type, const char *id) {
    RRDSET *st = rrdset_find_bytype(localhost, type, id, false);
    return st;
}

#endif //NETDATA_RRDSET_INDEX_ID_H
