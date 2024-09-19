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

struct provider_data {
    uint64_t value;                     // the mask of the keyword
    XXH64_hash_t hash;                  // the hash of the name
    uint32_t len;                       // the length of the name
    char *name;                         // the name of the keyword in UTF-8
};

struct provider_list {
    uint64_t min, max, mask;
    bool exceeds_data_type;             // true when the manifest values exceed the capacity of the EvtXXX() API
    uint32_t total;                     // the number of entries in the array
    struct provider_data *array;        // the array of entries, sorted (for binary search)
};

typedef struct publisher {
    ND_UUID uuid;                       // the Provider GUID
    const char *name;                   // the Provider name (UTF-8)
    uint32_t total_handles;             // the number of handles allocated
    uint32_t available_handles;         // the number of available handles
    uint32_t deleted_handles;           // the number of deleted handles
    PROVIDER_META_HANDLE *handles;      // a double linked list of all the handles

    struct provider_list keywords;
    struct provider_list tasks;
    struct provider_list opcodes;
    struct provider_list levels;
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

static void publisher_load_list(PROVIDER_META_HANDLE *h, WEVT_VARIANT *content, WEVT_VARIANT *property, TXT_UNICODE *dst, struct provider_list *l, EVT_PUBLISHER_METADATA_PROPERTY_ID property_id);

static inline ND_UUID *publisher_value_to_key(PUBLISHER *p) {
    return &p->uuid;
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

    bool load_it = false;
    PUBLISHER *p = SIMPLE_HASHTABLE_SLOT_DATA(slot);
    if(!p) {
        p = aral_callocz(pbc.aral_publishers);
        p->uuid = uuid;
        simple_hashtable_set_slot_PROVIDER_GUID(&pbc.hashtable, slot, hash, p);
        load_it = true;
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
        p->available_handles++;
    }

    if(!h->owner) {
        fatal_assert(p->available_handles > 0);
        p->available_handles--;
        h->owner = me;
    }

    h->locks++;

    if(load_it) {
        WEVT_VARIANT content = { 0 };
        WEVT_VARIANT property = { 0 };
        TXT_UNICODE unicode = { 0 };

        publisher_load_list(h, &content, &property, &unicode, &p->keywords, EvtPublisherMetadataKeywords);
        publisher_load_list(h, &content, &property, &unicode, &p->levels, EvtPublisherMetadataLevels);
        publisher_load_list(h, &content, &property, &unicode, &p->opcodes, EvtPublisherMetadataOpcodes);
        publisher_load_list(h, &content, &property, &unicode, &p->tasks, EvtPublisherMetadataTasks);

        txt_unicode_cleanup(&unicode);
        wevt_variant_cleanup(&content);
        wevt_variant_cleanup(&property);
    }

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
// load publisher lists

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

// Comparison function for ascending order (for Levels, Opcodes, Tasks)
static int compare_ascending(const void *a, const void *b) {
    struct provider_data *d1 = (struct provider_data *)a;
    struct provider_data *d2 = (struct provider_data *)b;

    if (d1->value < d2->value) return -1;
    if (d1->value > d2->value) return 1;
    return 0;
}

//// Comparison function for descending order (for Keywords)
//static int compare_descending(const void *a, const void *b) {
//    struct provider_data *d1 = (struct provider_data *)a;
//    struct provider_data *d2 = (struct provider_data *)b;
//
//    if (d1->value > d2->value) return -1;
//    if (d1->value < d2->value) return 1;
//    return 0;
//}

static void publisher_load_list(PROVIDER_META_HANDLE *h, WEVT_VARIANT *content, WEVT_VARIANT *property, TXT_UNICODE *dst, struct provider_list *l, EVT_PUBLISHER_METADATA_PROPERTY_ID property_id) {
    if(!h || !h->hMetadata) return;

    EVT_PUBLISHER_METADATA_PROPERTY_ID name_id, message_id, value_id;
    uint8_t value_bits = 32;
    int (*compare_func)(const void *, const void *);
    bool (*is_valid)(uint64_t, bool);

    switch(property_id) {
        case EvtPublisherMetadataLevels:
            name_id = EvtPublisherMetadataLevelName;
            message_id = EvtPublisherMetadataLevelMessageID;
            value_id = EvtPublisherMetadataLevelValue;
            value_bits = 32;
            compare_func = compare_ascending;
            is_valid = is_valid_publisher_level;
            break;

        case EvtPublisherMetadataOpcodes:
            name_id = EvtPublisherMetadataOpcodeName;
            message_id = EvtPublisherMetadataOpcodeMessageID;
            value_id = EvtPublisherMetadataOpcodeValue;
            value_bits = 32;
            is_valid = is_valid_publisher_opcode;
            compare_func = compare_ascending;
            break;

        case EvtPublisherMetadataTasks:
            name_id = EvtPublisherMetadataTaskName;
            message_id = EvtPublisherMetadataTaskMessageID;
            value_id = EvtPublisherMetadataTaskValue;
            value_bits = 32;
            is_valid = is_valid_publisher_task;
            compare_func = compare_ascending;
            break;

        case EvtPublisherMetadataKeywords:
            name_id = EvtPublisherMetadataKeywordName;
            message_id = EvtPublisherMetadataKeywordMessageID;
            value_id = EvtPublisherMetadataKeywordValue;
            value_bits = 64;
            is_valid = is_valid_publisher_keywords;
            compare_func = NULL;
            break;

        default:
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Internal Error: Can't handle property id %u", property_id);
            return;
    }

    EVT_HANDLE hMetadata = h->hMetadata;
    EVT_HANDLE hArray = NULL;
    DWORD bufferUsed = 0;
    DWORD itemCount = 0;

    // Get the metadata array for the list (e.g., opcodes, tasks, or levels)
    if (!EvtGetPublisherMetadataProperty(hMetadata, property_id, 0, 0, NULL, &bufferUsed)) {
        DWORD status = GetLastError();
        if (status != ERROR_INSUFFICIENT_BUFFER) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetPublisherMetadataProperty() failed");
            goto cleanup;
        }
    }

    wevt_variant_resize(content, bufferUsed);
    if (!EvtGetPublisherMetadataProperty(hMetadata, property_id, 0, content->size, content->data, &bufferUsed)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetPublisherMetadataProperty() failed after resize");
        goto cleanup;
    }

    // Get the number of items (e.g., levels, tasks, or opcodes)
    hArray = content->data->EvtHandleVal;
    if (!EvtGetObjectArraySize(hArray, &itemCount)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "EvtGetObjectArraySize() failed");
        goto cleanup;
    }

    if (itemCount == 0) {
        l->total = 0;
        l->array = NULL;
        goto cleanup;
    }

    // Allocate memory for the list items
    l->array = callocz(itemCount, sizeof(struct provider_data));
    l->total = itemCount;

    uint64_t min = UINT64_MAX, max = 0, mask = 0;

    // Iterate over the list and populate the entries
    for (DWORD i = 0; i < itemCount; ++i) {
        struct provider_data *d = &l->array[i];

        // Get the value (e.g., opcode, task, or level)
        if (wevt_get_property_from_array(property, hArray, i, value_id)) {
            switch(value_bits) {
                case 64:
                    d->value = wevt_field_get_uint64(property->data);
                    break;

                case 32:
                    d->value = wevt_field_get_uint32(property->data);
                    break;
            }

            if(d->value < min)
                min = d->value;

            if(d->value > max)
                max = d->value;

            mask |= d->value;

            if(!is_valid(d->value, false))
                l->exceeds_data_type = true;
        }

        // Get the message ID
        if (wevt_get_property_from_array(property, hArray, i, message_id)) {
            uint32_t messageID = wevt_field_get_uint32(property->data);

            if (messageID != (uint32_t)-1) {
                if (wevt_get_message_unicode(dst, hMetadata, NULL, messageID, EvtFormatMessageId)) {
                    size_t len;
                    d->name = unicode2utf8_strdupz(dst->data, &len);
                    d->len = len;
                }
            }
        }

        // Get the name if the message is missing
        if (!d->name && wevt_get_property_from_array(property, hArray, i, name_id)) {
            fatal_assert(property->data->Type == EvtVarTypeString);
            size_t len;
            d->name = unicode2utf8_strdupz(property->data->StringVal, &len);
            d->len = len;
        }

        // Calculate the hash for the name
        if (d->name)
            d->hash = XXH3_64bits(d->name, d->len);
    }

    l->min = min;
    l->max = max;
    l->mask = mask;

    if(itemCount > 1 && compare_func != NULL) {
        // Sort the array based on the value (ascending for all except keywords, descending for keywords)
        qsort(l->array, itemCount, sizeof(struct provider_data), compare_func);
    }

cleanup:
    if (hArray)
        EvtClose(hArray);
}

// --------------------------------------------------------------------------------------------------------------------
// lookup functions

// lookup bitmap metdata (returns a comma separated list of strings)
static bool publisher_bitmap_metadata(TXT_UTF8 *dst, struct provider_list *l, uint64_t value) {
    if(!(value & l->mask) || !l->total || !l->array || l->exceeds_data_type)
        return false;

    // do not empty the buffer, there may be reserved keywords in it
    // dst->used = 0;

    if(dst->used)
        dst->used--;

    size_t added = 0;
    for(size_t k = 0; value && k < l->total; k++) {
        struct provider_data *d = &l->array[k];

        if(d->value && (value & d->value) == d->value && d->name && d->len) {
            const char *s = d->name;
            size_t slen = d->len;

            // remove the mask from the value
            value &= ~(d->value);

            txt_utf8_resize(dst, dst->used + slen + 2 + 1, true);

            if(dst->used) {
                // add a comma and a space
                dst->data[dst->used++] = ',';
                dst->data[dst->used++] = ' ';
            }

            memcpy(&dst->data[dst->used], s, slen);
            dst->used += slen;
            dst->src = TXT_SOURCE_PUBLISHER;
            added++;
        }
    }

    if(dst->used > 1) {
        txt_utf8_resize(dst, dst->used + 1, true);
        dst->data[dst->used++] = 0;
    }

    fatal_assert(dst->used <= dst->size);
    return added;
}

//// lookup a single value (returns its string)
//static bool publisher_value_metadata_linear(TXT_UTF8 *dst, struct provider_list *l, uint64_t value) {
//    if(value < l->min || value > l->max || !l->total || !l->array || l->exceeds_data_type)
//        return false;
//
//    dst->used = 0;
//
//    for(size_t k = 0; k < l->total; k++) {
//        struct provider_data *d = &l->array[k];
//
//        if(d->value == value && d->name && d->len) {
//            const char *s = d->name;
//            size_t slen = d->len;
//
//            txt_utf8_resize(dst, slen + 1, false);
//
//            memcpy(dst->data, s, slen);
//            dst->used = slen;
//            dst->src = TXT_SOURCE_PUBLISHER;
//
//            break;
//        }
//    }
//
//    if(dst->used) {
//        txt_utf8_resize(dst, dst->used + 1, true);
//        dst->data[dst->used++] = 0;
//    }
//
//    fatal_assert(dst->used <= dst->size);
//
//    return (dst->used > 0);
//}

static bool publisher_value_metadata(TXT_UTF8 *dst, struct provider_list *l, uint64_t value) {
    if(value < l->min || value > l->max || !l->total || !l->array || l->exceeds_data_type)
        return false;

    // if(l->total < 3) return publisher_value_metadata_linear(dst, l, value);

    dst->used = 0;

    size_t left = 0;
    size_t right = l->total - 1;

    // Binary search within bounds
    while (left <= right) {
        size_t mid = left + (right - left) / 2;
        struct provider_data *d = &l->array[mid];

        if (d->value == value) {
            // Value found, now check if it has a valid name and length
            if (d->name && d->len) {
                const char *s = d->name;
                size_t slen = d->len;

                txt_utf8_resize(dst, slen + 1, false);
                memcpy(dst->data, s, slen);
                dst->used = slen;
                dst->data[dst->used++] = 0;
                dst->src = TXT_SOURCE_PUBLISHER;
            }
            break;
        }

        if (d->value < value)
            left = mid + 1;
        else {
            if (mid == 0) break;
            right = mid - 1;
        }
    }

    fatal_assert(dst->used <= dst->size);
    return (dst->used > 0);
}

// --------------------------------------------------------------------------------------------------------------------
// public API to lookup metadata

bool publisher_keywords_cacheable(PROVIDER_META_HANDLE *h) {
    return h && !h->publisher->keywords.exceeds_data_type;
}

bool publisher_tasks_cacheable(PROVIDER_META_HANDLE *h) {
    return h && !h->publisher->tasks.exceeds_data_type;
}

bool is_useful_publisher_for_levels(PROVIDER_META_HANDLE *h) {
    return h && !h->publisher->levels.exceeds_data_type;
}

bool publisher_opcodes_cacheable(PROVIDER_META_HANDLE *h) {
    return h && !h->publisher->opcodes.exceeds_data_type;
}

bool publisher_get_keywords(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value) {
    if(!h) return false;
    return publisher_bitmap_metadata(dst, &h->publisher->keywords, value);
}

bool publisher_get_level(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value) {
    if(!h) return false;
    return publisher_value_metadata(dst, &h->publisher->levels, value);
}

bool publisher_get_task(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value) {
    if(!h) return false;
    return publisher_value_metadata(dst, &h->publisher->tasks, value);
}

bool publisher_get_opcode(TXT_UTF8 *dst, PROVIDER_META_HANDLE *h, uint64_t value) {
    if(!h) return false;
    return publisher_value_metadata(dst, &h->publisher->opcodes, value);
}
