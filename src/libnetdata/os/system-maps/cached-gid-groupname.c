// SPDX-License-Identifier: GPL-3.0-or-later

#include "cached-gid-groupname.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for caching gid to groupname mappings
// key is the gid, value is groupname (STRING)

#define SIMPLE_HASHTABLE_KEY_TYPE gid_t
#define SIMPLE_HASHTABLE_VALUE_TYPE CACHED_GROUPNAME
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION cached_groupname_to_gid_ptr
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION compar_gid_ptr
#define SIMPLE_HASHTABLE_NAME _GROUPNAMES_CACHE
#include "libnetdata/simple_hashtable.h"

static struct {
    bool initialized;
    SPINLOCK spinlock;
    SIMPLE_HASHTABLE_GROUPNAMES_CACHE ht;
} uc = {
    .spinlock = NETDATA_SPINLOCK_INITIALIZER,
    .ht = { 0 },
};

static gid_t *cached_groupname_to_gid_ptr(CACHED_GROUPNAME *cu) {
    return &cu->gid;
}

static bool compar_gid_ptr(gid_t *a, gid_t *b) {
    return *a == *b;
}

void cached_groupname_populate_by_gid(gid_t gid, const char *groupname, bool overwrite) {
    internal_fatal(!uc.initialized, "system-users cache needs to be initialized");
    if(!groupname || !*groupname) return;

    spinlock_lock(&uc.spinlock);

    XXH64_hash_t hash = XXH3_64bits(&gid, sizeof(gid));
    SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_get_slot_GROUPNAMES_CACHE(&uc.ht, hash, &gid, true);
    CACHED_GROUPNAME *cu = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!cu || (overwrite && string_strcmp(cu->groupname, groupname) != 0)) {
        internal_fatal(cu && cu->gid != gid, "invalid gid matched from cache");

        if(cu)
            string_freez(cu->groupname);
        else
            cu = callocz(1, sizeof(*cu));

        cu->gid = gid;
        cu->groupname = string_strdupz(groupname);
        simple_hashtable_set_slot_GROUPNAMES_CACHE(&uc.ht, sl, hash, cu);
    }

    spinlock_unlock(&uc.spinlock);
}

CACHED_GROUPNAME cached_groupname_get_by_gid(gid_t gid) {
    internal_fatal(!uc.initialized, "system-users cache needs to be initialized");

    spinlock_lock(&uc.spinlock);

    XXH64_hash_t hash = XXH3_64bits(&gid, sizeof(gid));
    SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_get_slot_GROUPNAMES_CACHE(&uc.ht, hash, &gid, true);
    CACHED_GROUPNAME *cu = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!cu) {
        cu = callocz(1, sizeof(*cu));

        static char tmp[1024]; // we are inside a global spinlock - it is ok to be static
        struct group gr, *result = NULL;

        if (getgrgid_r(gid, &gr, tmp, sizeof(tmp), &result) != 0 || !result || !gr.gr_name || !(*gr.gr_name)) {
            char name[50];
            snprintfz(name, sizeof(name), "%u", gid);
            cu->groupname = string_strdupz(name);
        }
        else
            cu->groupname = string_strdupz(gr.gr_name);

        cu->gid = gid;
        simple_hashtable_set_slot_GROUPNAMES_CACHE(&uc.ht, sl, hash, cu);
    }

    internal_fatal(cu->gid != gid, "invalid gid matched from cache");

    CACHED_GROUPNAME rc = {
        .gid = cu->gid,
        .groupname = string_dup(cu->groupname),
    };

    spinlock_unlock(&uc.spinlock);
    return rc;
}

void cached_groupname_release(CACHED_GROUPNAME cu) {
    string_freez(cu.groupname);
}

void system_groupnames_cache_init(void) {
    if(uc.initialized) return;
    uc.initialized = true;

    spinlock_init(&uc.spinlock);
    simple_hashtable_init_GROUPNAMES_CACHE(&uc.ht, 100);
}

void system_groupnames_cache_destroy(void) {
    if(!uc.initialized) return;

    spinlock_lock(&uc.spinlock);

    for(SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_first_read_only_GROUPNAMES_CACHE(&uc.ht);
         sl;
         sl = simple_hashtable_next_read_only_GROUPNAMES_CACHE(&uc.ht, sl)) {
        CACHED_GROUPNAME *u = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(u) {
            string_freez(u->groupname);
            freez(u);
            // simple_hashtable_del_slot_GROUPNAMES_CACHE(&uc.ht, sl);
        }
    }

    simple_hashtable_destroy_GROUPNAMES_CACHE(&uc.ht);
    uc.initialized = false;
}
