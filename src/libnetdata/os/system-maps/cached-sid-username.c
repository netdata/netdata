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
    if (ConvertSidToStringSidW(sv->key.sid, &sid_string)) {
        sv->sid_str = string_strdupz(account2utf8(sid_string));
        LocalFree(sid_string);
    }
    else
        sv->sid_str = NULL;
}

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
    lookup_user_in_system(tmp);

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

#define SIDCACHE_TEST_SIDS 2000
#define SIDCACHE_TEST_THREADS 8

static struct {
    PSID sids[SIDCACHE_TEST_SIDS];
    SID_VALUE *seen[SIDCACHE_TEST_THREADS][SIDCACHE_TEST_SIDS];
    int32_t waiting;
} sidcache_test;

// bytes in use in the default process heap, where LocalAlloc() allocates
static size_t sidcache_test_process_heap_used(void) {
    HANDLE heap = GetProcessHeap();
    PROCESS_HEAP_ENTRY e = { 0 };
    size_t used = 0;

    if(!HeapLock(heap))
        return 0;

    while(HeapWalk(heap, &e))
        if(e.wFlags & PROCESS_HEAP_ENTRY_BUSY)
            used += e.cbData;

    HeapUnlock(heap);
    return used;
}

// SIDs of a random domain, so that none of them is in the cache yet
static void sidcache_test_make_sids(void) {
    SID_IDENTIFIER_AUTHORITY nt = SECURITY_NT_AUTHORITY;
    uint32_t a = os_random32(), b = os_random32(), c = os_random32();

    for(size_t i = 0; i < SIDCACHE_TEST_SIDS; i++) {
        sidcache_test.sids[i] = NULL;
        if(!AllocateAndInitializeSid(&nt, 5, SECURITY_NT_NON_UNIQUE, a, b, c, 1000 + (DWORD)i, 0, 0, 0, &sidcache_test.sids[i]))
            fatal("SIDCACHE TEST: AllocateAndInitializeSid() failed");
    }
}

static void sidcache_test_free_sids(void) {
    for(size_t i = 0; i < SIDCACHE_TEST_SIDS; i++) {
        FreeSid(sidcache_test.sids[i]);
        sidcache_test.sids[i] = NULL;
    }
}

static void *sidcache_test_thread(void *arg) {
    size_t t = (size_t)arg;

    // start all threads together, so that they miss the cache for the same SIDs at the same time
    __atomic_sub_fetch(&sidcache_test.waiting, 1, __ATOMIC_RELEASE);
    while(__atomic_load_n(&sidcache_test.waiting, __ATOMIC_ACQUIRE) > 0)
        tinysleep();

    for(size_t i = 0; i < SIDCACHE_TEST_SIDS; i++)
        sidcache_test.seen[t][i] = lookup_or_convert_user_id_to_name_lookup(sidcache_test.sids[i]);

    return NULL;
}

int cached_sid_username_unittest(void) {
    int errors = 0;
    cached_sid_username_init();

    // 1. a lookup must not leave Windows allocations behind
    {
        sidcache_test_make_sids();

        size_t before = sidcache_test_process_heap_used();
        size_t resolved = 0;
        for(size_t i = 0; i < SIDCACHE_TEST_SIDS; i++) {
            SID_VALUE *sv = lookup_or_convert_user_id_to_name_lookup(sidcache_test.sids[i]);
            if(sv && sv->sid_str)
                resolved++;
        }
        size_t after = sidcache_test_process_heap_used();

        sidcache_test_free_sids();

        // a leaked "S-1-5-21-..." string costs about 100 bytes per lookup;
        // Windows' own caches add about 10-20 bytes per lookup without it
        size_t growth = after > before ? after - before : 0;
        bool ok = resolved == SIDCACHE_TEST_SIDS && growth < SIDCACHE_TEST_SIDS * 48;
        fprintf(stderr, "SIDCACHE TEST: %zu cold lookups, %zu resolved, process heap grew by %zu bytes (%.1f per lookup): %s\n",
                (size_t)SIDCACHE_TEST_SIDS, resolved, growth, (double)growth / SIDCACHE_TEST_SIDS, ok ? "OK" : "FAILED");
        if(!ok) errors++;
    }

    // 2. threads missing the same SID together must all get the one cached entry
    {
        sidcache_test_make_sids();

        pthread_t threads[SIDCACHE_TEST_THREADS];
        __atomic_store_n(&sidcache_test.waiting, SIDCACHE_TEST_THREADS, __ATOMIC_RELEASE);
        for(size_t t = 0; t < SIDCACHE_TEST_THREADS; t++)
            if(pthread_create(&threads[t], NULL, sidcache_test_thread, (void *)t) != 0)
                fatal("SIDCACHE TEST: pthread_create() failed");
        for(size_t t = 0; t < SIDCACHE_TEST_THREADS; t++)
            pthread_join(threads[t], NULL);

        size_t mismatches = 0;
        for(size_t i = 0; i < SIDCACHE_TEST_SIDS; i++) {
            SID_VALUE *cached = lookup_or_convert_user_id_to_name_lookup(sidcache_test.sids[i]);
            for(size_t t = 0; t < SIDCACHE_TEST_THREADS; t++) {
                if(sidcache_test.seen[t][i] != cached) {
                    mismatches++;
                    break;
                }
            }
        }

        sidcache_test_free_sids();

        bool ok = mismatches == 0;
        fprintf(stderr, "SIDCACHE TEST: %zu SIDs looked up by %d threads together, %zu got an entry that is not the cached one: %s\n",
                (size_t)SIDCACHE_TEST_SIDS, SIDCACHE_TEST_THREADS, mismatches, ok ? "OK" : "FAILED");
        if(!ok) errors++;
    }

    fprintf(stderr, "SIDCACHE TEST: %s\n", errors ? "FAILED" : "PASSED");
    return errors ? 1 : 0;
}

#endif
