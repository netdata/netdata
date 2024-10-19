// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_GROUPS_H
#define NETDATA_SYSTEM_GROUPS_H

#include "libnetdata/libnetdata.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for caching uid to username mappings
// key is the uid, value is username (STRING)

#define SIMPLE_HASHTABLE_VALUE_TYPE STRING
#define SIMPLE_HASHTABLE_NAME _GROUPNAMES_CACHE
#include "libnetdata/simple_hashtable.h"

typedef struct groupnames_cache {
    SPINLOCK spinlock;
    SIMPLE_HASHTABLE_GROUPNAMES_CACHE ht;
} GROUPNAMES_CACHE;

static inline STRING *system_groupnames_cache_lookup_gid(GROUPNAMES_CACHE *gc, gid_t gid) {
    spinlock_lock(&gc->spinlock);

    XXH64_hash_t hash = XXH3_64bits(&gid, sizeof(gid));
    SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_get_slot_GROUPNAMES_CACHE(&gc->ht, hash, &gid, true);
    STRING *g = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!g) {
        char tmp[1024 + 1];
        struct group grp, *result = NULL;

        if (getgrgid_r(gid, &grp, tmp, sizeof(tmp), &result) != 0 || !result || !grp.gr_name || !(*grp.gr_name)) {
            char name[50];
            snprintfz(name, sizeof(name), "%u", gid);
            g = string_strdupz(name);
        }
        else
            g = string_strdupz(grp.gr_name);

        simple_hashtable_set_slot_GROUPNAMES_CACHE(&gc->ht, sl, hash, g);
    }

    g = string_dup(g);
    spinlock_unlock(&gc->spinlock);
    return g;
}

static inline GROUPNAMES_CACHE *system_groupnames_cache_init(void) {
    GROUPNAMES_CACHE *gc = callocz(1, sizeof(*gc));
    spinlock_init(&gc->spinlock);
    simple_hashtable_init_GROUPNAMES_CACHE(&gc->ht, 100);
    return gc;
}

static inline void system_groupnames_cache_destroy(GROUPNAMES_CACHE *gc) {
    spinlock_lock(&gc->spinlock);

    for(SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_first_read_only_GROUPNAMES_CACHE(&gc->ht);
         sl;
         sl = simple_hashtable_next_read_only_GROUPNAMES_CACHE(&gc->ht, sl)) {
        STRING *u = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        string_freez(u);
    }

    simple_hashtable_destroy_GROUPNAMES_CACHE(&gc->ht);
    freez(gc);
}

#endif //NETDATA_SYSTEM_GROUPS_H
