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
    static bool initialized = false;

    if(__atomic_load_n(&initialized, __ATOMIC_ACQUIRE))
        return;

    spinlock_lock(&sid_globals.spinlock);
    if(!__atomic_load_n(&initialized, __ATOMIC_ACQUIRE)) {
        simple_hashtable_init_SID(&sid_globals.hashtable, 100);
        __atomic_store_n(&initialized, true, __ATOMIC_RELEASE);
    }
    spinlock_unlock(&sid_globals.spinlock);
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

static STRING *sid_to_sid_str(PSID sid) {
    wchar_t *sid_string = NULL;
    if (!ConvertSidToStringSidW(sid, &sid_string))
        return NULL;

    STRING *s = string_strdupz(account2utf8(sid_string));
    LocalFree(sid_string);
    return s;
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

    sv->sid_str = sid_to_sid_str(sv->key.sid);
}

// the unit test replaces it, to make two threads resolve the same SID together
static void (*sid_resolve)(SID_VALUE *sv) = lookup_user_in_system;

static void sid_value_free(SID_VALUE *sv) {
    string_freez(sv->account);
    string_freez(sv->domain);
    string_freez(sv->full);
    string_freez(sv->sid_str);
    freez(sv);
}

static SID_VALUE *lookup_or_convert_user_id_to_name_lookup(PSID sid) {
    if(!sid || !IsValidSid(sid))
        return NULL;

    size_t size = GetLengthSid(sid);

    size_t tmp_size = sizeof(SID_VALUE) + size;
    size_t tmp_key_size = sizeof(SID_KEY) + size;
    SID_VALUE *tmp = mallocz(tmp_size);
    memcpy(&tmp->key.sid, sid, size);
    tmp->key.len = size;

    spinlock_lock(&sid_globals.spinlock);
    SID_VALUE *found = simple_hashtable_get_SID(&sid_globals.hashtable, &tmp->key, tmp_key_size);
    spinlock_unlock(&sid_globals.spinlock);
    if(found) {
        freez(tmp);
        return found;
    }

    // resolve it outside the lock, it may be slow
    sid_resolve(tmp);

    // add it to the cache, unless another thread added it meanwhile
    spinlock_lock(&sid_globals.spinlock);
    found = simple_hashtable_get_SID(&sid_globals.hashtable, &tmp->key, tmp_key_size);
    if(!found) {
        simple_hashtable_set_SID(&sid_globals.hashtable, &tmp->key, tmp_key_size, tmp);
        found = tmp;
        tmp = NULL;
    }
    spinlock_unlock(&sid_globals.spinlock);

    if(tmp)
        sid_value_free(tmp);

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

// --------------------------------------------------------------------------------------------------------------------
// unit test: -W sidcachetest

#define SIDCACHE_TEST_CONVERSIONS 1000
#define SIDCACHE_TEST_RACES 16

static struct {
    uint32_t salt;
    int32_t arrived;
    int32_t target;
    PSID sid;
    SID_VALUE *seen[2];
} sidcache_test;

// bytes in use in the default process heap, where LocalAlloc() allocates
static bool sidcache_test_process_heap_used(size_t *used) {
    HANDLE heap = GetProcessHeap();
    PROCESS_HEAP_ENTRY e = { 0 };
    *used = 0;

    if(!HeapLock(heap))
        return false;

    while(HeapWalk(heap, &e))
        if(e.wFlags & PROCESS_HEAP_ENTRY_BUSY)
            *used += e.cbData;

    DWORD error = GetLastError();
    HeapUnlock(heap);
    return error == ERROR_NO_MORE_ITEMS;
}

// a SID of a random domain, so that it is not in the cache
static PSID sidcache_test_make_sid(DWORD rid) {
    SID_IDENTIFIER_AUTHORITY nt = SECURITY_NT_AUTHORITY;
    PSID sid = NULL;
    if(!AllocateAndInitializeSid(&nt, 5, SECURITY_NT_NON_UNIQUE,
                                 sidcache_test.salt, os_random32(), os_random32(), rid, 0, 0, 0, &sid))
        fatal("SIDCACHE TEST: AllocateAndInitializeSid() failed");
    return sid;
}

// replaces lookup_user_in_system(): both threads get here after missing the cache,
// and wait for each other, so that both publish the same SID
static void sidcache_test_resolve(SID_VALUE *sv) {
    int32_t me = __atomic_add_fetch(&sidcache_test.arrived, 1, __ATOMIC_ACQ_REL);

    usec_t until = now_monotonic_usec() + 5 * USEC_PER_SEC;
    while(__atomic_load_n(&sidcache_test.arrived, __ATOMIC_ACQUIRE) < sidcache_test.target && now_monotonic_usec() < until)
        tinysleep();

    // a unique string per candidate, so that the string entries count how many candidates are alive
    char buf[64];
    snprintfz(buf, sizeof(buf), "sidcache-test-%u-%d", sidcache_test.salt, me);
    sv->account = NULL;
    sv->domain = NULL;
    sv->full = NULL;
    sv->sid_str = string_strdupz(buf);
}

static void *sidcache_test_thread(void *arg) {
    size_t t = (size_t)arg;
    sidcache_test.seen[t] = lookup_or_convert_user_id_to_name_lookup(sidcache_test.sid);
    return NULL;
}

int cached_sid_username_unittest(void) {
    int errors = 0;
    cached_sid_username_init();
    sidcache_test.salt = os_random32();

    // 1. converting a SID to a string must not leave Windows allocations behind
    {
        PSID sid = sidcache_test_make_sid(1000);
        string_freez(sid_to_sid_str(sid)); // warm up

        size_t before = 0, after = 0;
        bool walked = sidcache_test_process_heap_used(&before);
        size_t converted = 0;
        for(size_t i = 0; i < SIDCACHE_TEST_CONVERSIONS; i++) {
            STRING *s = sid_to_sid_str(sid);
            if(s) converted++;
            string_freez(s);
        }
        walked = sidcache_test_process_heap_used(&after) && walked;
        FreeSid(sid);

        // a leaked "S-1-5-21-..." string costs about 100 bytes per conversion
        size_t growth = after > before ? after - before : 0;
        bool ok = walked && converted == SIDCACHE_TEST_CONVERSIONS && growth < SIDCACHE_TEST_CONVERSIONS * 16;
        fprintf(stderr, "SIDCACHE TEST: %zu SID conversions, %zu converted, heap walk %s, process heap grew by %zu bytes: %s\n",
                (size_t)SIDCACHE_TEST_CONVERSIONS, converted, walked ? "ok" : "failed", growth, ok ? "OK" : "FAILED");
        if(!ok) errors++;
    }

    // 2. two threads missing the cache for the same SID must get the same entry, and the other one must be freed
    {
        void (*saved)(SID_VALUE *sv) = sid_resolve;
        sid_resolve = sidcache_test_resolve;

        size_t entries_before = 0, entries_after = 0;
        string_statistics(NULL, NULL, NULL, &entries_before, NULL, NULL, NULL, NULL, NULL);

        size_t failed = 0;
        for(int32_t r = 0; r < SIDCACHE_TEST_RACES; r++) {
            sidcache_test.sid = sidcache_test_make_sid(2000 + r);
            sidcache_test.target = 2 * (r + 1);
            sidcache_test.seen[0] = sidcache_test.seen[1] = NULL;

            pthread_t threads[2];
            for(size_t t = 0; t < 2; t++)
                if(pthread_create(&threads[t], NULL, sidcache_test_thread, (void *)t) != 0)
                    fatal("SIDCACHE TEST: pthread_create() failed");
            for(size_t t = 0; t < 2; t++)
                pthread_join(threads[t], NULL);

            SID_VALUE *cached = lookup_or_convert_user_id_to_name_lookup(sidcache_test.sid);
            if(!cached || sidcache_test.seen[0] != cached || sidcache_test.seen[1] != cached)
                failed++;

            FreeSid(sidcache_test.sid);
            sidcache_test.sid = NULL;
        }

        string_statistics(NULL, NULL, NULL, &entries_after, NULL, NULL, NULL, NULL, NULL);
        sid_resolve = saved;

        // each race publishes one candidate with one string; a leaked candidate keeps a second string alive
        size_t alive = entries_after - entries_before;
        bool ok = failed == 0 && alive == SIDCACHE_TEST_RACES;
        fprintf(stderr, "SIDCACHE TEST: %d races of 2 threads, %zu returned an entry that is not the cached one, %zu candidates alive (expected %d): %s\n",
                SIDCACHE_TEST_RACES, failed, alive, SIDCACHE_TEST_RACES, ok ? "OK" : "FAILED");
        if(!ok) errors++;
    }

    fprintf(stderr, "SIDCACHE TEST: %s\n", errors ? "FAILED" : "PASSED");
    return errors ? 1 : 0;
}

#endif
