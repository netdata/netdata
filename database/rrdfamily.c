// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDFAMILY index

static inline RRDFAMILY *rrdfamily_index_add(RRDHOST *host, RRDFAMILY *rc) {
    return dictionary_set(host->rrdfamily_root_index, string2str(rc->family), rc, sizeof(RRDFAMILY));
}

static inline RRDFAMILY *rrdfamily_index_del(RRDHOST *host, RRDFAMILY *rc) {
    dictionary_del(host->rrdfamily_root_index, string2str(rc->family));
    return rc;
}

static inline RRDFAMILY *rrdfamily_index_find(RRDHOST *host, const char *id) {
    return dictionary_get(host->rrdfamily_root_index, id);
}

// ----------------------------------------------------------------------------
// RRDFAMILY management

RRDFAMILY *rrdfamily_create(RRDHOST *host, const char *id) {
    RRDFAMILY *rf = rrdfamily_index_find(host, id);
    if(!rf) {
        rf = callocz(1, sizeof(RRDFAMILY));

        rf->family = string_strdupz(id);

        rf->rrdvariables = rrdvariables_create();

        RRDFAMILY *ret = rrdfamily_index_add(host, rf);
        if(ret != rf)
            error("RRDFAMILY: INTERNAL ERROR: Expected to INSERT RRDFAMILY '%s' into index, but inserted '%s'.", string2str(rf->family), (ret)?string2str(ret->family):"NONE");
    }

    rf->use_count++;
    return rf;
}

void rrdfamily_free(RRDHOST *host, RRDFAMILY *rf) {
    rf->use_count--;
    if(!rf->use_count) {
        RRDFAMILY *ret = rrdfamily_index_del(host, rf);
        if(ret != rf)
            error("RRDFAMILY: INTERNAL ERROR: Expected to DELETE RRDFAMILY '%s' from index, but deleted '%s'.", string2str(rf->family), (ret)?string2str(ret->family):"NONE");
        else {
            debug(D_RRD_CALLS, "RRDFAMILY: Cleaning up remaining family variables for host '%s', family '%s'", rrdhost_hostname(host), string2str(rf->family));
            rrdvariables_destroy(rf->rrdvariables);
            string_freez(rf->family);
            freez(rf);
        }
    }
}

