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
void rrdset_index_destroy(RRDHOST *host);

void rrdset_free(RRDSET *st);

RRDSET *rrdset_find(RRDHOST *host, const char *id);
RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id);

RRDSET_ACQUIRED *rrdset_find_and_acquire(RRDHOST *host, const char *id);

void rrdset_acquired_release(RRDSET_ACQUIRED *rsa);
RRDSET *rrdset_acquired_to_rrdset(RRDSET_ACQUIRED *rsa);

uint16_t rrddim_collection_modulo(RRDSET *st, uint32_t spread);

#define rrdset_find_localhost(id) rrdset_find(localhost, id)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_localhost(const char *id) {
    RRDSET *st = rrdset_find_localhost(id);
    return st;
}

#define rrdset_find_bytype_localhost(type, id) rrdset_find_bytype(localhost, type, id)
/* This will not return charts that are archived */
static inline RRDSET *rrdset_find_active_bytype_localhost(const char *type, const char *id) {
    RRDSET *st = rrdset_find_bytype_localhost(type, id);
    return st;
}

#endif //NETDATA_RRDSET_INDEX_ID_H
