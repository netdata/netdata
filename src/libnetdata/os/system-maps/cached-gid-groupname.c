// SPDX-License-Identifier: GPL-3.0-or-later

#include "cached-gid-groupname.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for caching gid to groupname mappings
// key is the gid, value is groupname (STRING)

#define SIMPLE_HASHTABLE_KEY_TYPE gid_t
#define SIMPLE_HASHTABLE_VALUE_TYPE CACHED_GROUPNAME *
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION cached_groupname_to_gid_ptr
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION compar_gid_ptr
#define SIMPLE_HASHTABLE_NAME _GROUPNAMES_CACHE
#include "libnetdata/simple_hashtable/simple_hashtable.h"

static struct {
    bool initialized;
    SPINLOCK spinlock;
    SIMPLE_HASHTABLE_GROUPNAMES_CACHE ht;
} group_cache = {
    .initialized = false,
    .spinlock = SPINLOCK_INITIALIZER,
    .ht = { 0 },
};

static gid_t *cached_groupname_to_gid_ptr(CACHED_GROUPNAME *cu) {
    return &cu->gid;
}

static bool compar_gid_ptr(gid_t *a, gid_t *b) {
    return *a == *b;
}

void cached_groupname_populate_by_gid(gid_t gid, const char *groupname, uint32_t version) {
    internal_fatal(!group_cache.initialized, "system-users cache needs to be initialized");
    if(!groupname || !*groupname) return;

    spinlock_lock(&group_cache.spinlock);

    XXH64_hash_t hash = XXH3_64bits(&gid, sizeof(gid));
    SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_get_slot_GROUPNAMES_CACHE(&group_cache.ht, hash, &gid, true);
    CACHED_GROUPNAME *cg = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!cg || (cg->version && version > cg->version)) {
        internal_fatal(cg && cg->gid != gid, "invalid gid matched from cache");

        if(cg)
            string_freez(cg->groupname);
        else
            cg = callocz(1, sizeof(*cg));

        cg->version = version;
        cg->gid = gid;
        cg->groupname = string_strdupz(groupname);
        simple_hashtable_set_slot_GROUPNAMES_CACHE(&group_cache.ht, sl, hash, cg);
    }

    spinlock_unlock(&group_cache.spinlock);
}

CACHED_GROUPNAME cached_groupname_get_by_gid(gid_t gid) {
    internal_fatal(!group_cache.initialized, "system-users cache needs to be initialized");

    spinlock_lock(&group_cache.spinlock);

    XXH64_hash_t hash = XXH3_64bits(&gid, sizeof(gid));
    SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_get_slot_GROUPNAMES_CACHE(&group_cache.ht, hash, &gid, true);
    CACHED_GROUPNAME *cg = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!cg) {
        cg = callocz(1, sizeof(*cg));

        static char tmp[1024]; // we are inside a global spinlock - it is ok to be static
        struct group gr, *result = NULL;

        if (getgrgid_r(gid, &gr, tmp, sizeof(tmp), &result) != 0 || !result || !gr.gr_name || !(*gr.gr_name)) {
            char name[UINT64_MAX_LENGTH];
            print_uint64(name, gid);
            cg->groupname = string_strdupz(name);
        }
        else
            cg->groupname = string_strdupz(gr.gr_name);

        cg->gid = gid;
        simple_hashtable_set_slot_GROUPNAMES_CACHE(&group_cache.ht, sl, hash, cg);
    }

    internal_fatal(cg->gid != gid, "invalid gid matched from cache");

    CACHED_GROUPNAME rc = {
        .version = cg->version,
        .gid = cg->gid,
        .groupname = string_dup(cg->groupname),
    };

    spinlock_unlock(&group_cache.spinlock);
    return rc;
}

void cached_groupname_release(CACHED_GROUPNAME cg) {
    string_freez(cg.groupname);
}

void cached_groupnames_init(void) {
    if(group_cache.initialized) return;
    group_cache.initialized = true;

    spinlock_init(&group_cache.spinlock);
    simple_hashtable_init_GROUPNAMES_CACHE(&group_cache.ht, 100);
}

void cached_groupnames_destroy(void) {
    if(!group_cache.initialized) return;

    spinlock_lock(&group_cache.spinlock);

    for(SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_first_read_only_GROUPNAMES_CACHE(&group_cache.ht);
         sl;
         sl = simple_hashtable_next_read_only_GROUPNAMES_CACHE(&group_cache.ht, sl)) {
        CACHED_GROUPNAME *u = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(u) {
            string_freez(u->groupname);
            freez(u);
            // simple_hashtable_del_slot_GROUPNAMES_CACHE(&uc.ht, sl);
        }
    }

    simple_hashtable_destroy_GROUPNAMES_CACHE(&group_cache.ht);
    group_cache.initialized = false;

    spinlock_unlock(&group_cache.spinlock);
}

void cached_groupnames_delete_old_versions(uint32_t version) {
    if(!group_cache.initialized) return;

    spinlock_lock(&group_cache.spinlock);

    for(SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_first_read_only_GROUPNAMES_CACHE(&group_cache.ht);
         sl;
         sl = simple_hashtable_next_read_only_GROUPNAMES_CACHE(&group_cache.ht, sl)) {
        CACHED_GROUPNAME *cg = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(cg && cg->version && cg->version < version) {
            string_freez(cg->groupname);
            freez(cg);
            simple_hashtable_del_slot_GROUPNAMES_CACHE(&group_cache.ht, sl);
        }
    }

    spinlock_unlock(&group_cache.spinlock);
}
