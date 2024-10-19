// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_USERS_H
#define NETDATA_SYSTEM_USERS_H

#include "libnetdata/libnetdata.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for caching uid to username mappings
// key is the uid, value is username (STRING)

#define SIMPLE_HASHTABLE_VALUE_TYPE STRING
#define SIMPLE_HASHTABLE_NAME _USERNAMES_CACHE
#include "libnetdata/simple_hashtable.h"

typedef struct usernames_cache {
    SPINLOCK spinlock;
    SIMPLE_HASHTABLE_USERNAMES_CACHE ht;
} USERNAMES_CACHE;

static inline STRING *system_usernames_cache_lookup_uid(USERNAMES_CACHE *uc, uid_t uid) {
    spinlock_lock(&uc->spinlock);

    XXH64_hash_t hash = XXH3_64bits(&uid, sizeof(uid));
    SIMPLE_HASHTABLE_SLOT_USERNAMES_CACHE *sl = simple_hashtable_get_slot_USERNAMES_CACHE(&uc->ht, hash, &uid, true);
    STRING *u = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!u) {
        char tmp[1024 + 1];
        struct passwd pw, *result = NULL;

        if (getpwuid_r(uid, &pw, tmp, sizeof(tmp), &result) != 0 || !result || !pw.pw_name || !(*pw.pw_name)) {
            char name[50];
            snprintfz(name, sizeof(name), "%u", uid);
            u = string_strdupz(name);
        }
        else
            u = string_strdupz(pw.pw_name);

        simple_hashtable_set_slot_USERNAMES_CACHE(&uc->ht, sl, uid, u);
    }

    u = string_dup(u);
    spinlock_unlock(&uc->spinlock);
    return u;
}

static inline USERNAMES_CACHE *system_usernames_cache_init(void) {
    USERNAMES_CACHE *uc = callocz(1, sizeof(*uc));
    spinlock_init(&uc->spinlock);
    simple_hashtable_init_USERNAMES_CACHE(&uc->ht, 100);
    return uc;
}

static inline void system_usernames_cache_destroy(USERNAMES_CACHE *uc) {
    spinlock_lock(&uc->spinlock);

    for(SIMPLE_HASHTABLE_SLOT_USERNAMES_CACHE *sl = simple_hashtable_first_read_only_USERNAMES_CACHE(&uc->ht);
         sl;
         sl = simple_hashtable_next_read_only_USERNAMES_CACHE(&uc->ht, sl)) {
        STRING *u = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        string_freez(u);
    }

    simple_hashtable_destroy_USERNAMES_CACHE(&uc->ht);
    freez(uc);
}

#endif //NETDATA_SYSTEM_USERS_H
