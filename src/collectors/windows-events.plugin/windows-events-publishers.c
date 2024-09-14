// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-publishers.h"

struct publisher;

struct provider_meta_handle {
    pid_t locker;
    uint32_t locks;
    EVT_HANDLE hMetadata;
    struct publisher *publisher;
    struct provider_meta_handle *prev, *next;
};

typedef struct publisher {
    ND_UUID provider;
    uint32_t total_handles;
    uint32_t available_handles;
    uint32_t deleted_handles;
    PROVIDER_META_HANDLE *handles;
} PUBLISHER;

#define SIMPLE_HASHTABLE_NAME _PROVIDER_GUID
#define SIMPLE_HASHTABLE_VALUE_TYPE PUBLISHER
#define SIMPLE_HASHTABLE_KEY_TYPE ND_UUID
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION publisher_value_to_key
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION publisher_cache_compar
#define SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION 1
#include "libnetdata/simple_hashtable.h"

static struct {
    SPINLOCK spinlock;
    uint32_t total_publishers;
    uint32_t total_handles;
    uint32_t deleted_handles;
    struct simple_hashtable_PROVIDER_GUID hashtable;
    ARAL *aral_publishers;
    ARAL *aral_handles;
} pbc = {
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
};

static inline ND_UUID *publisher_value_to_key(PUBLISHER *p) {
    return &p->provider;
}

static inline bool publisher_cache_compar(ND_UUID *a, ND_UUID *b) {
    return nd_uuid_compare(a->uuid, b->uuid);
}

void publisher_cache_init(void) {
    simple_hashtable_init_PROVIDER_GUID(&pbc.hashtable, 100000);
    pbc.aral_publishers = aral_create("wevt_publishers", sizeof(PUBLISHER), 0, 4096, NULL, NULL, NULL, false, true);
    pbc.aral_handles = aral_create("wevt_handles", sizeof(PROVIDER_META_HANDLE), 0, 4096, NULL, NULL, NULL, false, true);
}

PROVIDER_META_HANDLE *publisher_get(ND_UUID uuid, LPCWSTR providerName) {
    if(!providerName || !providerName[0] || UUIDiszero(uuid))
        return NULL;

    // XXH64_hash_t hash = XXH3_64bits(&uuid, sizeof(uuid));
    uint64_t hash = uuid.parts.low64 + uuid.parts.hig64;

    spinlock_lock(&pbc.spinlock);

    SIMPLE_HASHTABLE_SLOT_PROVIDER_GUID *slot =
            simple_hashtable_get_slot_PROVIDER_GUID(&pbc.hashtable, hash, &uuid, true);

    PUBLISHER *p = SIMPLE_HASHTABLE_SLOT_DATA(slot);
    if(!p) {
        p = aral_callocz(pbc.aral_publishers);
        p->provider = uuid;
        simple_hashtable_set_slot_PROVIDER_GUID(&pbc.hashtable, slot, hash, p);
        pbc.total_publishers++;
    }

    pid_t me = gettid_cached();
    PROVIDER_META_HANDLE *h;
    for(h = p->handles; h ;h = h->next) {
        if(!h->locker || h->locker == me)
            break;
    }

    if(!h) {
        h = aral_callocz(pbc.aral_handles);
        h->publisher = p;
        h->hMetadata = EvtOpenPublisherMetadata(
                NULL,          // Local machine
                providerName,  // Provider name
                NULL,          // Log file path (NULL for default)
                0,             // Locale (0 for default locale)
                0              // Flags
        );
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(p->handles, h, prev, next);
        pbc.total_handles++;
        p->total_handles++;
    }
    h->locker = me;
    h->locks++;

    spinlock_unlock(&pbc.spinlock);

    return h;
}

EVT_HANDLE publisher_handle(PROVIDER_META_HANDLE *h) {
    return h ? h->hMetadata : NULL;
}

void publisher_dup(PROVIDER_META_HANDLE *h) {
    if(!h) return;
    h->locks++;
}

void publisher_release(PROVIDER_META_HANDLE *h) {
    if(!h) return;
    pid_t me = gettid_cached();
    fatal_assert(h->locker == me);
    fatal_assert(h->locks > 0);
    if(--h->locks == 0) {
        PUBLISHER *p = h->publisher;

        spinlock_lock(&pbc.spinlock);
        h->locker = 0;

        if(++p->available_handles > 1) {
            // there are multiple handles on this publisher
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(p->handles, h, prev, next);

            if(h->hMetadata)
                EvtClose(h->hMetadata);

            aral_freez(pbc.aral_handles, h);

            pbc.total_handles--;
            p->total_handles--;

            pbc.deleted_handles++;
            p->deleted_handles++;
        }
        else if(h->next) {
            // it is not the last, put it at the end
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(p->handles, h, prev, next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(p->handles, h, prev, next);
        }

        spinlock_unlock(&pbc.spinlock);
    }
}
