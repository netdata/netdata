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
    , RRD_MEMORY_MODE memory_mode
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

int rrdset_reset_name(RRDSET *st, const char *name);

void rrdset_index_init(RRDHOST *host);
void rrdset_index_destroy(RRDHOST *host);

void rrdset_free(RRDSET *st);

RRDSET *rrdset_find(RRDHOST *host, const char *id);
RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id);

RRDSET_ACQUIRED *rrdset_find_and_acquire(RRDHOST *host, const char *id);

void rrdset_acquired_release(RRDSET_ACQUIRED *rsa);
RRDSET *rrdset_acquired_to_rrdset(RRDSET_ACQUIRED *rsa);

#endif //NETDATA_RRDSET_INDEX_ID_H
