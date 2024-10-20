// SPDX-License-Identifier: GPL-3.0-or-later

#include "cached-uid-username.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for caching uid to username mappings
// key is the uid, value is username (STRING)

#define SIMPLE_HASHTABLE_KEY_TYPE uid_t
#define SIMPLE_HASHTABLE_VALUE_TYPE CACHED_USERNAME
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION cached_username_to_uid_ptr
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION compar_uid_ptr
#define SIMPLE_HASHTABLE_NAME _USERNAMES_CACHE
#include "libnetdata/simple_hashtable.h"

static struct {
    bool initialized;
    SPINLOCK spinlock;
    SIMPLE_HASHTABLE_USERNAMES_CACHE ht;
} uc = {
    .spinlock = NETDATA_SPINLOCK_INITIALIZER,
    .ht = { 0 },
};

static uid_t *cached_username_to_uid_ptr(CACHED_USERNAME *cu) {
    return &cu->uid;
}

static bool compar_uid_ptr(uid_t *a, uid_t *b) {
    return *a == *b;
}

void cached_username_populate_by_uid(uid_t uid, const char *username, bool overwrite) {
    internal_fatal(!uc.initialized, "system-users cache needs to be initialized");
    if(!username || !*username) return;

    spinlock_lock(&uc.spinlock);

    XXH64_hash_t hash = XXH3_64bits(&uid, sizeof(uid));
    SIMPLE_HASHTABLE_SLOT_USERNAMES_CACHE *sl = simple_hashtable_get_slot_USERNAMES_CACHE(&uc.ht, hash, &uid, true);
    CACHED_USERNAME *cu = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!cu || (overwrite && string_strcmp(cu->username, username) != 0)) {
        internal_fatal(cu && cu->uid != uid, "invalid uid matched from cache");

        if(cu)
            string_freez(cu->username);
        else
            cu = callocz(1, sizeof(*cu));

        cu->uid = uid;
        cu->username = string_strdupz(username);
        simple_hashtable_set_slot_USERNAMES_CACHE(&uc.ht, sl, hash, cu);
    }

    spinlock_unlock(&uc.spinlock);
}

CACHED_USERNAME cached_username_get_by_uid(uid_t uid) {
    internal_fatal(!uc.initialized, "system-users cache needs to be initialized");

    spinlock_lock(&uc.spinlock);

    XXH64_hash_t hash = XXH3_64bits(&uid, sizeof(uid));
    SIMPLE_HASHTABLE_SLOT_USERNAMES_CACHE *sl = simple_hashtable_get_slot_USERNAMES_CACHE(&uc.ht, hash, &uid, true);
    CACHED_USERNAME *cu = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!cu) {
        cu = callocz(1, sizeof(*cu));

        static char tmp[1024]; // we are inside a global spinlock - it is ok to be static
        struct passwd pw, *result = NULL;

        if (getpwuid_r(uid, &pw, tmp, sizeof(tmp), &result) != 0 || !result || !pw.pw_name || !(*pw.pw_name)) {
            char name[50];
            snprintfz(name, sizeof(name), "%u", uid);
            cu->username = string_strdupz(name);
        }
        else
            cu->username = string_strdupz(pw.pw_name);

        cu->uid = uid;
        simple_hashtable_set_slot_USERNAMES_CACHE(&uc.ht, sl, hash, cu);
    }

    internal_fatal(cu->uid != uid, "invalid uid matched from cache");

    CACHED_USERNAME rc = {
        .uid = cu->uid,
        .username = string_dup(cu->username),
    };

    spinlock_unlock(&uc.spinlock);
    return rc;
}

void cached_username_release(CACHED_USERNAME cu) {
    string_freez(cu.username);
}

void system_usernames_cache_init(void) {
    if(uc.initialized) return;
    uc.initialized = true;

    spinlock_init(&uc.spinlock);
    simple_hashtable_init_USERNAMES_CACHE(&uc.ht, 100);
}

void system_usernames_cache_destroy(void) {
    if(!uc.initialized) return;

    spinlock_lock(&uc.spinlock);

    for(SIMPLE_HASHTABLE_SLOT_USERNAMES_CACHE *sl = simple_hashtable_first_read_only_USERNAMES_CACHE(&uc.ht);
         sl;
         sl = simple_hashtable_next_read_only_USERNAMES_CACHE(&uc.ht, sl)) {
        CACHED_USERNAME *u = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(u) {
            string_freez(u->username);
            freez(u);
            // simple_hashtable_del_slot_USERNAMES_CACHE(&uc.ht, sl);
        }
    }

    simple_hashtable_destroy_USERNAMES_CACHE(&uc.ht);
    uc.initialized = false;
}
