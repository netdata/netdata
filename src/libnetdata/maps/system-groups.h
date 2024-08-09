// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_GROUPS_H
#define NETDATA_SYSTEM_GROUPS_H

#include "libnetdata/libnetdata.h"

#if defined(OS_WINDOWS)
    #include <windows.h>
    #include <lm.h>
    #include <sddl.h>
#endif

// --------------------------------------------------------------------------------------------------------------------
// hashtable for caching gid to groupname mappings
// key is the gid, value is groupname (STRING)

#define SIMPLE_HASHTABLE_VALUE_TYPE STRING
#define SIMPLE_HASHTABLE_NAME _GROUPNAMES_CACHE
#include "libnetdata/simple_hashtable.h"

typedef struct groupnames_cache {
    SPINLOCK spinlock;
    SIMPLE_HASHTABLE_GROUPNAMES_CACHE ht;
} GROUPNAMES_CACHE;

#if defined(OS_WINDOWS)
static inline STRING *system_groupnames_cache_get_groupname_from_gid(gid_t gid) {
    STRING *g = NULL;

    HANDLE token = NULL;
    if (OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) {
        DWORD size = 0;
        GetTokenInformation(token, TokenPrimaryGroup, NULL, 0, &size);
        if (GetLastError() == ERROR_INSUFFICIENT_BUFFER) {
            TOKEN_PRIMARY_GROUP *tokenGroup = (TOKEN_PRIMARY_GROUP *)malloc(size);
            if (GetTokenInformation(token, TokenPrimaryGroup, tokenGroup, size, &size)) {
                char groupname[UNLEN + 1];
                char domain[DNLEN + 1];
                DWORD groupname_len = UNLEN + 1;
                DWORD domain_len = DNLEN + 1;
                SID_NAME_USE sidType;

                if (LookupAccountSid(NULL, tokenGroup->PrimaryGroup, groupname, &groupname_len, domain, &domain_len, &sidType))
                    g = string_strdup(groupname);
            }
            free(tokenGroup);
        }
        CloseHandle(token);
    }

    if(!g) {
        char name[50];
        snprintf(name, sizeof(name), "%u", gid);
        g = string_strdupz(name);
    }

    return g;
}
#else
static inline STRING *system_groupnames_cache_get_groupname_from_gid(gid_t gid) {
    STRING *g;

    char tmp[1024 + 1];
    struct group grp, *result = NULL;

    if (getgrgid_r(gid, &grp, tmp, sizeof(tmp), &result) != 0 || !result || !grp.gr_name || !(*grp.gr_name)) {
        char name[50];
        snprintfz(name, sizeof(name), "%u", gid);
        g = string_strdupz(name);
    }
    else
        g = string_strdupz(grp.gr_name);

    return g;
}
#endif

static inline STRING *system_groupnames_cache_lookup_gid(GROUPNAMES_CACHE *gc, gid_t gid) {
    spinlock_lock(&gc->spinlock);

    SIMPLE_HASHTABLE_SLOT_GROUPNAMES_CACHE *sl = simple_hashtable_get_slot_GROUPNAMES_CACHE(&gc->ht, gid, &gid, true);
    STRING *g = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if (!g) {
        g = system_groupnames_cache_get_groupname_from_gid(gid);
        simple_hashtable_set_slot_GROUPNAMES_CACHE(&gc->ht, sl, gid, g);
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
        STRING *g = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        string_freez(g);
    }

    simple_hashtable_destroy_GROUPNAMES_CACHE(&gc->ht);
    freez(gc);
}

#endif //NETDATA_SYSTEM_GROUPS_H
