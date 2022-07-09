// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCONTEXT_H
#define NETDATA_RRDCONTEXT_H 1

#include "rrd.h"

typedef struct rrdmetric RRDMETRIC;
typedef struct rrdmetric_acquired RRDMETRIC_ACQUIRED;
typedef struct rrdinstance RRDINSTANCE;
typedef struct rrdinstance_acquired RRDINSTANCE_ACQUIRED;


// ----------------------------------------------------------------------------
// RRDCONTEXT

typedef struct rrdcontext RRDCONTEXT;
typedef struct rrdcontext_acquired RRDCONTEXT_ACQUIRED;

extern void rrdcontexts_create(RRDHOST *host);

static inline RRDCONTEXT_ACQUIRED *rrdcontext_acquire(RRDHOST *host, const char *name) {
    return (RRDCONTEXT_ACQUIRED *)dictionary_get_and_acquire_item(host->rrdcontexts, name);
}

static inline void rrdcontext_release(RRDHOST *host, RRDCONTEXT_ACQUIRED *context) {
    dictionary_acquired_item_release(host->rrdcontexts, (DICTIONARY_ITEM *)context);
}

static inline const char *rrdcontext_acquired_name(RRDHOST *host, RRDCONTEXT_ACQUIRED *item) {
    return dictionary_acquired_item_name(host->rrdcontexts, (DICTIONARY_ITEM *)item);
}

static inline RRDCONTEXT *rrdcontext_acquired_value(RRDHOST *host, RRDCONTEXT_ACQUIRED *item) {
    return dictionary_acquired_item_value(host->rrdcontexts, (DICTIONARY_ITEM *)item);
}


#endif // NETDATA_RRDCONTEXT_H

