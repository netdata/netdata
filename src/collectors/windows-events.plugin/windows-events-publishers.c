// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-publishers.h"

#define MAX_OPEN_HANDLES_PER_PUBLISHER 5

struct publisher;

// typedef as PROVIDER_META_HANDLE in include file
struct provider_meta_handle {
    pid_t owner;                        // the owner of the handle, or zero
    uint32_t locks;                     // the number of locks the owner has on this handle
    EVT_HANDLE hMetadata;               // the handle
    struct publisher *publisher;        // a pointer back to the publisher

    // double linked list
    PROVIDER_META_HANDLE *prev;
    PROVIDER_META_HANDLE *next;
};

struct provider_keyword {
    uint64_t mask;                      // the mask of the keyword
    uint32_t name_len;                  // the length of the name
    uint32_t message_len;               // the length of the message
    char *name;                         // the name of the keyword in UTF-8
    char *message;                      // the message of the keyword in UTF-8
};

typedef struct publisher {
    ND_UUID provider;                   // the Provider GUID
    uint32_t total_handles;             // the number of handles allocated
    uint32_t available_handles;         // the number of available handles
    uint32_t deleted_handles;           // the number of deleted handles
    PROVIDER_META_HANDLE *handles;      // a double linked list of all the handles

    uint32_t total_keywords;            // the number of keywords in the array
    struct provider_keyword *keywords;  // the array of keywords, sorted by importance
} PUBLISHER;

// A hashtable implementation for publishers
// using the provider GUID as key and PUBLISHER as value
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

static void publisher_load_keywords(PUBLISHER *p, LPCWSTR providerName);

static inline ND_UUID *publisher_value_to_key(PUBLISHER *p) {
    return &p->provider;
}

static inline bool publisher_cache_compar(ND_UUID *a, ND_UUID *b) {
    return UUIDeq(*a, *b);
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
        publisher_load_keywords(p, providerName);
        pbc.total_publishers++;
    }

    pid_t me = gettid_cached();
    PROVIDER_META_HANDLE *h;
    for(h = p->handles; h ;h = h->next) {
        // find the first that is mine,
        // or the first not owned by anyone
        if(!h->owner || h->owner == me)
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
        // we put it at the beginning of the list
        // to find it first if the same owner needs more locks on it
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(p->handles, h, prev, next);
        pbc.total_handles++;
        p->total_handles++;
    }
    h->owner = me;
    h->locks++;

    spinlock_unlock(&pbc.spinlock);

    return h;
}

EVT_HANDLE publisher_handle(PROVIDER_META_HANDLE *h) {
    return h ? h->hMetadata : NULL;
}

PROVIDER_META_HANDLE *publisher_dup(PROVIDER_META_HANDLE *h) {
    if(h) h->locks++;
    return h;
}

void publisher_release(PROVIDER_META_HANDLE *h) {
    if(!h) return;
    pid_t me = gettid_cached();
    fatal_assert(h->owner == me);
    fatal_assert(h->locks > 0);
    if(--h->locks == 0) {
        PUBLISHER *p = h->publisher;

        spinlock_lock(&pbc.spinlock);
        h->owner = 0;

        if(++p->available_handles > MAX_OPEN_HANDLES_PER_PUBLISHER) {
            // there are multiple handles on this publisher
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(p->handles, h, prev, next);

            if(h->hMetadata)
                EvtClose(h->hMetadata);

            aral_freez(pbc.aral_handles, h);

            pbc.total_handles--;
            p->total_handles--;

            pbc.deleted_handles++;
            p->deleted_handles++;

            p->available_handles--;
        }
        else if(h->next) {
            // it is not the last, put it at the end
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(p->handles, h, prev, next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(p->handles, h, prev, next);
        }

        spinlock_unlock(&pbc.spinlock);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// keywords handling

static bool wevt_get_property_from_array(WEVT_VARIANT *property, EVT_HANDLE handle, DWORD dwIndex, EVT_PUBLISHER_METADATA_PROPERTY_ID PropertyId) {
    DWORD used = 0;

    if (!EvtGetObjectArrayProperty(handle, PropertyId, dwIndex, 0, property->size, property->data, &used)) {
        DWORD status = GetLastError();
        if (status != ERROR_INSUFFICIENT_BUFFER) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetObjectArrayProperty() failed");
            return false;
        }

        wevt_variant_resize(property, used);
        if (!EvtGetObjectArrayProperty(handle, PropertyId, dwIndex, 0, property->size, property->data, &used)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetObjectArrayProperty() failed");
            return false;
        }
    }

    property->used = used;
    return true;
}

static bool wevt_get_message_id(TXT_UNICODE *dst, EVT_HANDLE hMetadata, DWORD dwMessageId) {
    DWORD used;
    if (!EvtFormatMessage(hMetadata, NULL, dwMessageId, 0, NULL, EvtFormatMessageId, dst->size, dst->data, &used)) {
        DWORD status = GetLastError();
        if (status != ERROR_INSUFFICIENT_BUFFER) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed");
            return false;
        }

        txt_unicode_resize(dst, used);
        if (!EvtFormatMessage(hMetadata, NULL, dwMessageId, 0, NULL, EvtFormatMessageId, dst->size, dst->data, &used)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtFormatMessage() failed after resize");
            return false;
        }
    }

    dst->used = used;
    return true;
}

static void publisher_load_keywords(PUBLISHER *p, LPCWSTR providerName) {
    EVT_HANDLE hMetadata = NULL;
    EVT_HANDLE hKeywordArray = NULL;
    DWORD bufferUsed = 0;
    DWORD keywordCount = 0;
    WEVT_VARIANT content = { 0 };
    WEVT_VARIANT property = { 0 };
    TXT_UNICODE unicode = { 0 };

    // Open the publisher metadata
    hMetadata = EvtOpenPublisherMetadata(
            NULL,          // Local machine
            providerName,  // Provider name
            NULL,          // Log file path
            0,             // Locale
            0              // Flags
    );
    if (!hMetadata) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtOpenPublisherMetadata() failed");
        return;
    }

    // Get the array of keyword objects
    if (!EvtGetPublisherMetadataProperty(hMetadata, EvtPublisherMetadataKeywords, 0, 0, NULL, &bufferUsed)) {
        DWORD status = GetLastError();
        if (status != ERROR_INSUFFICIENT_BUFFER) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetPublisherMetadataProperty() failed");
            goto cleanup;
        }
    }

    wevt_variant_resize(&content, bufferUsed);
    if (!EvtGetPublisherMetadataProperty(hMetadata, EvtPublisherMetadataKeywords, 0, content.size, content.data, &bufferUsed)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetPublisherMetadataProperty() failed after resize");
        goto cleanup;
    }

    // Get the number of keywords
    hKeywordArray = content.data->EvtHandleVal;
    if (!EvtGetObjectArraySize(hKeywordArray, &keywordCount)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetObjectArraySize() failed");
        goto cleanup;
    }

    if (keywordCount == 0) {
        p->total_keywords = 0;
        p->keywords = NULL;
        goto cleanup;
    }

    // Allocate memory for the keywords
    p->keywords = callocz(keywordCount, sizeof(struct provider_keyword));
    p->total_keywords = keywordCount;

    // Iterate over the keywords
    for (DWORD i = 0; i < keywordCount; ++i) {
        struct provider_keyword* pk = &p->keywords[i];

        // Get the keyword mask (value)
        if(wevt_get_property_from_array(&property, hKeywordArray, i, EvtPublisherMetadataKeywordValue))
            pk->mask = property.data->UInt64Val;

        // Get the keyword name
        if (wevt_get_property_from_array(&property, hKeywordArray, i, EvtPublisherMetadataKeywordName)) {
            size_t len;
            pk->name = unicode2utf8_strdupz(property.data->StringVal, &len);
            pk->name_len = len;
        }

        // Get the keyword message ID
        if(wevt_get_property_from_array(&property, hKeywordArray, i, EvtPublisherMetadataKeywordMessageID)) {
            DWORD messageID = property.data->UInt32Val;
            if ((int) messageID < 0) continue;

            if (!wevt_get_message_id(&unicode, hMetadata, messageID))
                continue;

            size_t len;
            pk->message = unicode2utf8_strdupz(unicode.data, &len);
            pk->message_len = len;
        }
    }

cleanup:
    txt_unicode_cleanup(&unicode);

    wevt_variant_cleanup(&content);
    wevt_variant_cleanup(&property);

    if (hKeywordArray)
        EvtClose(hKeywordArray);

    if (hMetadata)
        EvtClose(hMetadata);
}


bool publisher_keywords(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value) {
    if(!h) return false;

    dst->used = 0;

    PUBLISHER *p = h->publisher;
    for(size_t k = 0; k < p->total_keywords; k++) {
        struct provider_keyword *pk = &p->keywords[k];

        if(pk->mask && (value & pk->mask) == pk->mask && (pk->name || pk->message)) {
            const char *s;
            size_t slen;

            // if the message is available, prefer it
            // otherwise use the name
            if(pk->message && *pk->message) {
                s = pk->message;
                slen = pk->message_len;
            }
            else {
                s = pk->name;
                slen = pk->name_len;
            }

            if(!s || !slen)
                continue;

            // remove the mask from the value
            value &= ~(pk->mask);

            txt_utf8_resize(dst, dst->used + slen + 2 + 1, true);

            if(dst->used) {
                // add a comma and a space
                dst->data[dst->used++] = ',';
                dst->data[dst->used++] = ' ';
            }

            memcpy(&dst->data[dst->used], s, slen);
            dst->used += slen;
        }
    }

    if(dst->used) {
        txt_utf8_resize(dst, dst->used + 1, true);
        dst->data[dst->used++] = 0;
    }

    return (dst->used > 0);
}
