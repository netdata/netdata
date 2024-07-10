// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEM_USERS_H
#define NETDATA_SYSTEM_USERS_H

#include "libnetdata/libnetdata.h"

#if defined(OS_WINDOWS)
    #include <windows.h>
    #include <lm.h>
    #include <sddl.h>
#endif

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

#if defined(OS_WINDOWS)
static inline STRING *system_usernames_cache_get_username_from_uid(uid_t uid) {
    STRING *u = NULL;

    HANDLE token = NULL;
    if (OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) {
        DWORD size = 0;
        GetTokenInformation(token, TokenUser, NULL, 0, &size);
        if (GetLastError() == ERROR_INSUFFICIENT_BUFFER) {
            TOKEN_USER *tokenUser = (TOKEN_USER *)malloc(size);
            if (GetTokenInformation(token, TokenUser, tokenUser, size, &size)) {
                char username[UNLEN + 1];
                char domain[DNLEN + 1];
                DWORD username_len = UNLEN + 1;
                DWORD domain_len = DNLEN + 1;
                SID_NAME_USE sidType;

                if (LookupAccountSid(NULL, tokenUser->User.Sid, username, &username_len, domain, &domain_len, &sidType))
                    u = string_strdup(username);
            }
            free(tokenUser);
        }
        CloseHandle(token);
    }

    if(!u) {
        char name[50];
        snprintf(name, sizeof(name), "%u", uid);
        u = string_strdupz(name);
    }

    return u;
}
#else
static inline STRING *system_usernames_cache_get_username_from_uid(uid_t uid) {
    STRING *u;

    char tmp[1024 + 1];
    struct passwd pw, *result = NULL;

    if (getpwuid_r(uid, &pw, tmp, sizeof(tmp), &result) != 0 || !result || !pw.pw_name || !(*pw.pw_name)) {
        char name[50];
        snprintfz(name, sizeof(name), "%u", uid);
        u = string_strdupz(name);
    }
    else
        u = string_strdupz(pw.pw_name);

    return u;
}
#endif

static inline STRING *system_usernames_cache_lookup_uid(USERNAMES_CACHE *uc, uid_t uid) {
    spinlock_lock(&uc->spinlock);

    SIMPLE_HASHTABLE_SLOT_USERNAMES_CACHE *sl = simple_hashtable_get_slot_USERNAMES_CACHE(&uc->ht, uid, &uid, true);
    STRING *u = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if (!u) {
        u = system_usernames_cache_get_username_from_uid(uid);
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
