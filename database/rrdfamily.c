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
    RRDFAMILY *rc = rrdfamily_index_find(host, id);
    if(!rc) {
        rc = callocz(1, sizeof(RRDFAMILY));

        rc->family = string_strdupz(id);

        // initialize the variables index
        avl_init_lock(&rc->rrdvar_root_index, rrdvar_compare);

        RRDFAMILY *ret = rrdfamily_index_add(host, rc);
        if(ret != rc)
            error("RRDFAMILY: INTERNAL ERROR: Expected to INSERT RRDFAMILY '%s' into index, but inserted '%s'.", string2str(rc->family), (ret)?string2str(ret->family):"NONE");
    }

    rc->use_count++;
    return rc;
}

void rrdfamily_free(RRDHOST *host, RRDFAMILY *rc) {
    rc->use_count--;
    if(!rc->use_count) {
        RRDFAMILY *ret = rrdfamily_index_del(host, rc);
        if(ret != rc)
            error("RRDFAMILY: INTERNAL ERROR: Expected to DELETE RRDFAMILY '%s' from index, but deleted '%s'.", string2str(rc->family), (ret)?string2str(ret->family):"NONE");
        else {
            debug(D_RRD_CALLS, "RRDFAMILY: Cleaning up remaining family variables for host '%s', family '%s'", rrdhost_hostname(host), string2str(rc->family));
            rrdvar_free_remaining_variables(host, &rc->rrdvar_root_index);

            string_freez(rc->family);
            freez(rc);
        }
    }
}

