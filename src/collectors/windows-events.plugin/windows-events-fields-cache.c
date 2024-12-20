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
#define SIMPLE_HASHTABLE_VALUE_TYPE WEVT_FIELD_VALUE *
#define SIMPLE_HASHTABLE_KEY_TYPE WEVT_FIELD_KEY
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION field_cache_value_to_key
#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION field_cache_cache_compar
#define SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION 1
#include "libnetdata/simple_hashtable/simple_hashtable.h"

static inline WEVT_FIELD_KEY *field_cache_value_to_key(WEVT_FIELD_VALUE *p) {
    return &p->key;
}

static inline bool field_cache_cache_compar(WEVT_FIELD_KEY *a, WEVT_FIELD_KEY *b) {
    return memcmp(a, b, sizeof(WEVT_FIELD_KEY)) == 0;
}

struct ht {
    SPINLOCK spinlock;
    size_t allocations;
    size_t bytes;
    struct simple_hashtable_FIELDS_CACHE ht;
};

static struct {
    bool initialized;
    struct ht ht[WEVT_FIELD_TYPE_MAX];
} fdc = {
        .initialized = false,
};

void field_cache_init(void) {
    for(size_t type = 0; type < WEVT_FIELD_TYPE_MAX ; type++) {
        spinlock_init(&fdc.ht[type].spinlock);
        simple_hashtable_init_FIELDS_CACHE(&fdc.ht[type].ht, 10000);
    }
}

static inline bool should_zero_provider(WEVT_FIELD_TYPE type, uint64_t value) {
    switch(type) {
        case WEVT_FIELD_TYPE_LEVEL:
            return !is_valid_provider_level(value, true);

        case WEVT_FIELD_TYPE_KEYWORD:
            return !is_valid_provider_keyword(value, true);

        case WEVT_FIELD_TYPE_OPCODE:
            return !is_valid_provider_opcode(value, true);

        case WEVT_FIELD_TYPE_TASK:
            return !is_valid_provider_task(value, true);

        default:
            return false;
    }
}

bool field_cache_get(WEVT_FIELD_TYPE type, const ND_UUID *uuid, uint64_t value, TXT_UTF8 *dst) {
    fatal_assert(type < WEVT_FIELD_TYPE_MAX);

    struct ht *ht = &fdc.ht[type];

    WEVT_FIELD_KEY t = {
            .value = value,
            .provider = should_zero_provider(type, value) ? UUID_ZERO : *uuid,
    };
    XXH64_hash_t hash = XXH3_64bits(&t, sizeof(t));

    spinlock_lock(&ht->spinlock);
    SIMPLE_HASHTABLE_SLOT_FIELDS_CACHE *slot = simple_hashtable_get_slot_FIELDS_CACHE(&ht->ht, hash, &t, true);
    WEVT_FIELD_VALUE *v = SIMPLE_HASHTABLE_SLOT_DATA(slot);
    spinlock_unlock(&ht->spinlock);

    if(v) {
        txt_utf8_resize(dst, v->name_size, false);
        memcpy(dst->data, v->name, v->name_size);
        dst->used = v->name_size;
        dst->src = TXT_SOURCE_FIELD_CACHE;
        return true;
    }

    return false;
}

static WEVT_FIELD_VALUE *wevt_create_cache_entry(WEVT_FIELD_KEY *t, TXT_UTF8 *name, size_t *bytes) {
    *bytes = sizeof(WEVT_FIELD_VALUE) + name->used;
    WEVT_FIELD_VALUE *v = callocz(1, *bytes);
    v->key = *t;
    memcpy(v->name, name->data, name->used);
    v->name_size = name->used;
    return v;
}

//static bool is_numeric(const char *s) {
//    while(*s) {
//        if(!isdigit((uint8_t)*s++))
//            return false;
//    }
//
//    return true;
//}

void field_cache_set(WEVT_FIELD_TYPE type, const ND_UUID *uuid, uint64_t value, TXT_UTF8 *name) {
    fatal_assert(type < WEVT_FIELD_TYPE_MAX);

    struct ht *ht = &fdc.ht[type];

    WEVT_FIELD_KEY t = {
            .value = value,
            .provider = should_zero_provider(type, value) ? UUID_ZERO : *uuid,
    };
    XXH64_hash_t hash = XXH3_64bits(&t, sizeof(t));

    spinlock_lock(&ht->spinlock);
    SIMPLE_HASHTABLE_SLOT_FIELDS_CACHE *slot = simple_hashtable_get_slot_FIELDS_CACHE(&ht->ht, hash, &t, true);
    WEVT_FIELD_VALUE *v = SIMPLE_HASHTABLE_SLOT_DATA(slot);
    if(!v) {
        size_t bytes;
        v = wevt_create_cache_entry(&t, name, &bytes);
        simple_hashtable_set_slot_FIELDS_CACHE(&ht->ht, slot, hash, v);

        ht->allocations++;
        ht->bytes += bytes;
    }
//    else {
//        if((v->name_size == 1 && name->used > 0) || is_numeric(v->name)) {
//            size_t bytes;
//            WEVT_FIELD_VALUE *nv = wevt_create_cache_entry(&t, name, &bytes);
//            simple_hashtable_set_slot_FIELDS_CACHE(&ht->ht, slot, hash, nv);
//            ht->bytes += name->used;
//            ht->bytes -= v->name_size;
//            freez(v);
//        }
//        else if(name->used > 2 && !is_numeric(name->data) && (v->name_size != name->used || strcasecmp(v->name, name->data) != 0)) {
//            const char *a = v->name;
//            const char *b = name->data;
//            int x = 0;
//            x++;
//        }
//    }

    spinlock_unlock(&ht->spinlock);
}

