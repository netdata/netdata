// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata.h"

#if defined(OS_WINDOWS)
#include "cached-sid-username.h"

typedef struct {
    size_t len;
    uint8_t sid[];
} SID_KEY;

typedef struct {
    // IMPORTANT:
    // This is malloc'd ! You have to manually set fields to zero.

    STRING *account;
    STRING *domain;
    STRING *full;
    STRING *sid_str;

    // this needs to be last, because of its variable size
    SID_KEY key;
} SID_VALUE;

#define SIMPLE_HASHTABLE_NAME _SID
#define SIMPLE_HASHTABLE_VALUE_TYPE SID_VALUE *
#define SIMPLE_HASHTABLE_KEY_TYPE SID_KEY
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION sid_value_to_key
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION sid_cache_compar
#define SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION 1
#include "libnetdata/simple_hashtable/simple_hashtable.h"

static struct {
    SPINLOCK spinlock;
    struct simple_hashtable_SID hashtable;
} sid_globals = {
    .spinlock = SPINLOCK_INITIALIZER,
    .hashtable = { 0 },
};

static inline SID_KEY *sid_value_to_key(SID_VALUE *s) {
    return &s->key;
}

static inline bool sid_cache_compar(SID_KEY *a, SID_KEY *b) {
    return a->len == b->len && memcmp(&a->sid, &b->sid, a->len) == 0;
}

void cached_sid_username_init(void) {
    simple_hashtable_init_SID(&sid_globals.hashtable, 100);
}

static char *account2utf8(const wchar_t *user) {
    static __thread char buffer[256];
    if(utf16_to_utf8(buffer, sizeof(buffer), user, -1, NULL) == 0)
        buffer[0] = '\0';
    return buffer;
}

static char *domain2utf8(const wchar_t *domain) {
    static __thread char buffer[256];
    if(utf16_to_utf8(buffer, sizeof(buffer), domain, -1, NULL) == 0)
        buffer[0] = '\0';
    return buffer;
}

static void lookup_user_in_system(SID_VALUE *sv) {
    static __thread wchar_t account_unicode[256];
    static __thread wchar_t domain_unicode[256];
    static __thread char tmp[512 + 2];

    DWORD account_name_size = sizeof(account_unicode) / sizeof(account_unicode[0]);
    DWORD domain_name_size = sizeof(domain_unicode) / sizeof(domain_unicode[0]);
    SID_NAME_USE sid_type;

    if (LookupAccountSidW(NULL, sv->key.sid, account_unicode, &account_name_size, domain_unicode, &domain_name_size, &sid_type)) {
        const char *account = account2utf8(account_unicode);
        const char *domain = domain2utf8(domain_unicode);
        snprintfz(tmp, sizeof(tmp), "%s\\%s", domain, account);
        sv->domain = string_strdupz(domain);
        sv->account = string_strdupz(account);
        sv->full = string_strdupz(tmp);
    }
    else {
        sv->domain = NULL;
        sv->account = NULL;
        sv->full = NULL;
    }

    wchar_t *sid_string = NULL;
    if (ConvertSidToStringSidW(sv->key.sid, &sid_string))
        sv->sid_str = string_strdupz(account2utf8(sid_string));
    else
        sv->sid_str = NULL;
}

static SID_VALUE *lookup_or_convert_user_id_to_name_lookup(PSID sid) {
    if(!sid || !IsValidSid(sid))
        return NULL;

    size_t size = GetLengthSid(sid);

    size_t tmp_size = sizeof(SID_VALUE) + size;
    size_t tmp_key_size = sizeof(SID_KEY) + size;
    uint8_t buf[tmp_size];
    SID_VALUE *tmp = (SID_VALUE *)&buf;
    memcpy(&tmp->key.sid, sid, size);
    tmp->key.len = size;

    spinlock_lock(&sid_globals.spinlock);
    SID_VALUE *found = simple_hashtable_get_SID(&sid_globals.hashtable, &tmp->key, tmp_key_size);
    spinlock_unlock(&sid_globals.spinlock);
    if(found) return found;

    // allocate the SID_VALUE
    found = mallocz(tmp_size);
    memcpy(found, buf, tmp_size);

    lookup_user_in_system(found);

    // add it to the cache
    spinlock_lock(&sid_globals.spinlock);
    simple_hashtable_set_SID(&sid_globals.hashtable, &found->key, tmp_key_size, found);
    spinlock_unlock(&sid_globals.spinlock);

    return found;
}

bool cached_sid_to_account_domain_sidstr(PSID sid, TXT_UTF8 *dst_account, TXT_UTF8 *dst_domain, TXT_UTF8 *dst_sid_str) {
    SID_VALUE *found = lookup_or_convert_user_id_to_name_lookup(sid);

    if(found) {
        if (found->account) {
            txt_utf8_resize(dst_account, string_strlen(found->account) + 1, false);
            memcpy(dst_account->data, string2str(found->account), string_strlen(found->account) + 1);
            dst_account->used = string_strlen(found->account) + 1;
        }
        else
            txt_utf8_empty(dst_account);

        if (found->domain) {
            txt_utf8_resize(dst_domain, string_strlen(found->domain) + 1, false);
            memcpy(dst_domain->data, string2str(found->domain), string_strlen(found->domain) + 1);
            dst_domain->used = string_strlen(found->domain) + 1;
        }
        else
            txt_utf8_empty(dst_domain);

        if (found->sid_str) {
            txt_utf8_resize(dst_sid_str, string_strlen(found->sid_str) + 1, false);
            memcpy(dst_sid_str->data, string2str(found->sid_str), string_strlen(found->sid_str) + 1);
            dst_sid_str->used = string_strlen(found->sid_str) + 1;
        }
        else
            txt_utf8_empty(dst_sid_str);

        return true;
    }

    txt_utf8_empty(dst_account);
    txt_utf8_empty(dst_domain);
    txt_utf8_empty(dst_sid_str);
    return false;
}

bool cached_sid_to_buffer_append(PSID sid, BUFFER *dst, const char *prefix) {
    SID_VALUE *found = lookup_or_convert_user_id_to_name_lookup(sid);
    size_t added = 0;

    if(found) {
        if (found->full) {
            if (prefix && *prefix)
                buffer_strcat(dst, prefix);

            buffer_fast_strcat(dst, string2str(found->full), string_strlen(found->full));
            added++;
        }
        if (found->sid_str) {
            if (prefix && *prefix)
                buffer_strcat(dst, prefix);

            buffer_fast_strcat(dst, string2str(found->sid_str), string_strlen(found->sid_str));
            added++;
        }
    }

    return added > 0;
}

STRING *cached_sid_fullname_or_sid_str(PSID sid) {
    SID_VALUE *found = lookup_or_convert_user_id_to_name_lookup(sid);
    if(found) {
        if(found->full) return string_dup(found->full);
        return string_dup(found->sid_str);
    }
    return NULL;
}

#endif