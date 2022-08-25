// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDFAMILY index

int rrdfamily_compare(void *a, void *b) {
    // a and b are STRING pointers
    return (int)(a - b);
}

#define rrdfamily_index_add(host, rc) (RRDFAMILY *)avl_insert_lock(&((host)->rrdfamily_root_index), (avl_t *)(rc))
#define rrdfamily_index_del(host, rc) (RRDFAMILY *)avl_remove_lock(&((host)->rrdfamily_root_index), (avl_t *)(rc))

static RRDFAMILY *rrdfamily_index_find(RRDHOST *host, const char *id) {
    RRDFAMILY tmp = {
        .family = string_strdupz(id)
    };

    return (RRDFAMILY *)avl_search_lock(&(host->rrdfamily_root_index), (avl_t *) &tmp);
}

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
            debug(D_RRD_CALLS, "RRDFAMILY: Cleaning up remaining family variables for host '%s', family '%s'", host->hostname, string2str(rc->family));
            rrdvar_free_remaining_variables(host, &rc->rrdvar_root_index);

            string_freez(rc->family);
            freez(rc);
        }
    }
}

