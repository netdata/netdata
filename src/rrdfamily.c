// SPDX-License-Identifier: GPL-3.0+

#define NETDATA_RRD_INTERNALS 1
#include "common.h"

// ----------------------------------------------------------------------------
// RRDFAMILY index

int rrdfamily_compare(void *a, void *b) {
    if(((RRDFAMILY *)a)->hash_family < ((RRDFAMILY *)b)->hash_family) return -1;
    else if(((RRDFAMILY *)a)->hash_family > ((RRDFAMILY *)b)->hash_family) return 1;
    else return strcmp(((RRDFAMILY *)a)->family, ((RRDFAMILY *)b)->family);
}

#define rrdfamily_index_add(host, rc) (RRDFAMILY *)avl_insert_lock(&((host)->rrdfamily_root_index), (avl *)(rc))
#define rrdfamily_index_del(host, rc) (RRDFAMILY *)avl_remove_lock(&((host)->rrdfamily_root_index), (avl *)(rc))

static RRDFAMILY *rrdfamily_index_find(RRDHOST *host, const char *id, uint32_t hash) {
    RRDFAMILY tmp;
    tmp.family = id;
    tmp.hash_family = (hash)?hash:simple_hash(tmp.family);

    return (RRDFAMILY *)avl_search_lock(&(host->rrdfamily_root_index), (avl *) &tmp);
}

RRDFAMILY *rrdfamily_create(RRDHOST *host, const char *id) {
    RRDFAMILY *rc = rrdfamily_index_find(host, id, 0);
    if(!rc) {
        rc = callocz(1, sizeof(RRDFAMILY));

        rc->family = strdupz(id);
        rc->hash_family = simple_hash(rc->family);

        // initialize the variables index
        avl_init_lock(&rc->rrdvar_root_index, rrdvar_compare);

        RRDFAMILY *ret = rrdfamily_index_add(host, rc);
        if(ret != rc)
            error("RRDFAMILY: INTERNAL ERROR: Expected to INSERT RRDFAMILY '%s' into index, but inserted '%s'.", rc->family, (ret)?ret->family:"NONE");
    }

    rc->use_count++;
    return rc;
}

void rrdfamily_free(RRDHOST *host, RRDFAMILY *rc) {
    rc->use_count--;
    if(!rc->use_count) {
        RRDFAMILY *ret = rrdfamily_index_del(host, rc);
        if(ret != rc)
            error("RRDFAMILY: INTERNAL ERROR: Expected to DELETE RRDFAMILY '%s' from index, but deleted '%s'.", rc->family, (ret)?ret->family:"NONE");
        else {
            debug(D_RRD_CALLS, "RRDFAMILY: Cleaning up remaining family variables for host '%s', family '%s'", host->hostname, rc->family);
            rrdvar_free_remaining_variables(host, &rc->rrdvar_root_index);

            freez((void *) rc->family);
            freez(rc);
        }
    }
}

