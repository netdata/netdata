// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-sid.h"
#include <sddl.h>

typedef struct {
    size_t len;
    uint8_t sid[];
} SID_KEY;

typedef struct {
    const char *user;
    size_t user_len;

    // this needs to be last, because of its variable size
    SID_KEY key;
} SID_VALUE;

#define SIMPLE_HASHTABLE_NAME _SID
#define SIMPLE_HASHTABLE_VALUE_TYPE SID_VALUE
#define SIMPLE_HASHTABLE_KEY_TYPE SID_KEY
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION sid_value_to_key
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION sid_cache_compar
#define SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION 1
#include "libnetdata/simple_hashtable.h"

static struct {
    SPINLOCK spinlock;
    bool initialized;
    struct simple_hashtable_SID hashtable;
} sid_globals = {
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
        .hashtable = { 0 },
};

static inline SID_KEY *sid_value_to_key(SID_VALUE *s) {
    return &s->key;
}

static inline bool sid_cache_compar(SID_KEY *a, SID_KEY *b) {
    return a->len == b->len && memcmp(&a->sid, &b->sid, a->len) == 0;
}

static bool update_user(SID_VALUE *found, TXT_UTF8 *dst) {
    if(found && found->user) {
        txt_utf8_resize(dst, found->user_len + 1);
        memcpy(dst->data, found->user, found->user_len + 1);
        dst->used = found->user_len + 1;
        return true;
    }

    txt_utf8_resize(dst, 1);
    dst->data[0] = '\0';
    dst->used = 1;
    return false;
}

static void lookup_user(PSID *sid, TXT_UTF8 *dst) {
    static __thread wchar_t account_unicode[256];
    static __thread wchar_t domain_unicode[256];
    DWORD account_name_size = sizeof(account_unicode) / sizeof(account_unicode[0]);
    DWORD domain_name_size = sizeof(domain_unicode) / sizeof(domain_unicode[0]);
    SID_NAME_USE sid_type;

    txt_utf8_resize(dst, 1024);

    if (LookupAccountSidW(NULL, sid, account_unicode, &account_name_size, domain_unicode, &domain_name_size, &sid_type)) {
        const char *user = account2utf8(account_unicode);
        const char *domain = domain2utf8(domain_unicode);
        dst->used = snprintfz(dst->data, dst->size, "%s\\%s", domain, user) + 1;
    }
    else {
        wchar_t *sid_string = NULL;
        if (ConvertSidToStringSidW(sid, &sid_string)) {
            const char *user = account2utf8(sid_string);
            dst->used = snprintfz(dst->data, dst->size, "%s", user) + 1;
        }
        else
            dst->used = snprintfz(dst->data, dst->size, "[invalid]") + 1;
    }
}

bool wevt_convert_user_id_to_name(PSID sid, TXT_UTF8 *dst) {
    if(!sid || !IsValidSid(sid))
        return update_user(NULL, dst);

    size_t size = GetLengthSid(sid);

    size_t tmp_size = sizeof(SID_VALUE) + size;
    size_t tmp_key_size = sizeof(SID_KEY) + size;
    uint8_t buf[tmp_size];
    SID_VALUE *tmp = (SID_VALUE *)&buf;
    memcpy(&tmp->key.sid, sid, size);
    tmp->key.len = size;

    spinlock_lock(&sid_globals.spinlock);
    if(!sid_globals.initialized) {
        simple_hashtable_init_SID(&sid_globals.hashtable, 100);
        sid_globals.initialized = true;
    }
    SID_VALUE *found = simple_hashtable_get_SID(&sid_globals.hashtable, &tmp->key, tmp_key_size);
    spinlock_unlock(&sid_globals.spinlock);
    if(found) return update_user(found, dst);

    // allocate the SID_VALUE
    found = mallocz(tmp_size);
    memcpy(found, buf, tmp_size);

    // lookup the user
    lookup_user(sid, dst);
    found->user = strdupz(dst->data);
    found->user_len = dst->used - 1;

    // add it to the cache
    spinlock_lock(&sid_globals.spinlock);
    simple_hashtable_set_SID(&sid_globals.hashtable, &found->key, tmp_key_size, found);
    spinlock_unlock(&sid_globals.spinlock);

    return update_user(found, dst);
}
