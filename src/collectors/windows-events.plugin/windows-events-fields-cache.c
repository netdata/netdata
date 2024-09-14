// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-fields-cache.h"

typedef struct field_key {
    uint64_t value;
    ND_UUID provider;
} WEVT_FIELD_KEY;

typedef struct field_value {
    WEVT_FIELD_KEY key;
    uint32_t name_size;
    char name[];
} WEVT_FIELD_VALUE;

#define SIMPLE_HASHTABLE_NAME _FIELDS_CACHE
#define SIMPLE_HASHTABLE_VALUE_TYPE WEVT_FIELD_VALUE
#define SIMPLE_HASHTABLE_KEY_TYPE WEVT_FIELD_KEY
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION field_cache_value_to_key
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION field_cache_cache_compar
#define SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION 1
#include "libnetdata/simple_hashtable.h"

static inline WEVT_FIELD_KEY *field_cache_value_to_key(WEVT_FIELD_VALUE *p) {
    return &p->key;
}

static inline bool field_cache_cache_compar(WEVT_FIELD_KEY *a, WEVT_FIELD_KEY *b) {
    return memcmp(a, b, sizeof(WEVT_FIELD_KEY));
}

struct ht {
    SPINLOCK spinlock;
    size_t allocations;
    size_t bytes;
    struct simple_hashtable_FIELDS_CACHE ht;
};

static struct {
    bool initialized;
    struct ht ht[EVT_FIELD_TYPE_MAX];
} fdc = {
        .initialized = false,
};

void field_cache_init(void) {
    for(WEVT_FIELD_TYPE type = 0; type < EVT_FIELD_TYPE_MAX ; type++) {
        spinlock_init(&fdc.ht[type].spinlock);
        simple_hashtable_init_FIELDS_CACHE(&fdc.ht[type].ht, 10000);
    }
}

bool field_cache_get(WEVT_FIELD_TYPE type, ND_UUID uuid, uint64_t value, TXT_UTF8 *dst) {
    fatal_assert(type < EVT_FIELD_TYPE_MAX);

    struct ht *ht = &fdc.ht[type];

    WEVT_FIELD_KEY t = {
            .value = value,
            .provider = uuid,
    };
    XXH64_hash_t hash = XXH3_64bits(&t, sizeof(t));

    spinlock_lock(&ht->spinlock);
    SIMPLE_HASHTABLE_SLOT_FIELDS_CACHE *slot = simple_hashtable_get_slot_FIELDS_CACHE(&ht->ht, hash, &t, false);
    WEVT_FIELD_VALUE *v = SIMPLE_HASHTABLE_SLOT_DATA(slot);
    spinlock_unlock(&ht->spinlock);

    if(v) {
        txt_utf8_resize(dst, v->name_size);
        memcpy(dst->data, v->name, v->name_size);
        return true;
    }

    return false;
}

void field_cache_set(WEVT_FIELD_TYPE type, ND_UUID uuid, uint64_t value, TXT_UTF8 *name) {
    fatal_assert(type < EVT_FIELD_TYPE_MAX);

    struct ht *ht = &fdc.ht[type];

    WEVT_FIELD_KEY t = {
            .value = value,
            .provider = uuid,
    };
    XXH64_hash_t hash = XXH3_64bits(&t, sizeof(t));

    spinlock_lock(&ht->spinlock);
    SIMPLE_HASHTABLE_SLOT_FIELDS_CACHE *slot = simple_hashtable_get_slot_FIELDS_CACHE(&ht->ht, hash, &t, false);
    WEVT_FIELD_VALUE *v = SIMPLE_HASHTABLE_SLOT_DATA(slot);
    if(!v) {
        size_t bytes = sizeof(*v) + name->used;
        v = callocz(1, bytes);
        v->key = t;
        memcpy(v->name, name->data, name->used);
        v->name_size = name->used;

        simple_hashtable_set_slot_FIELDS_CACHE(&ht->ht, slot, hash, v);

        ht->allocations++;
        ht->bytes += bytes;
    }

    spinlock_unlock(&ht->spinlock);
}

