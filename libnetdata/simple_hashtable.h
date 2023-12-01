// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIMPLE_HASHTABLE_H
#define NETDATA_SIMPLE_HASHTABLE_H

#ifndef XXH_INLINE_ALL
#define XXH_INLINE_ALL
#endif
#include "xxhash.h"

typedef uint64_t SIMPLE_HASHTABLE_HASH;
#define SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS 32

typedef struct simple_hashtable_slot {
    SIMPLE_HASHTABLE_HASH hash;
    void *data;
} SIMPLE_HASHTABLE_SLOT;

typedef struct simple_hashtable {
    size_t resizes;
    size_t searches;
    size_t collisions;
    size_t deletions;
    size_t deleted;
    size_t used;
    size_t size;
    SIMPLE_HASHTABLE_SLOT *hashtable;
} SIMPLE_HASHTABLE;

static void simple_hashtable_init(SIMPLE_HASHTABLE *ht, size_t size) {
    ht->resizes = 0;
    ht->used = 0;
    ht->size = size;
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
}

static void simple_hashtable_free(SIMPLE_HASHTABLE *ht) {
    freez(ht->hashtable);
    ht->hashtable = NULL;
    ht->size = 0;
    ht->used = 0;
    ht->resizes = 0;
}

static inline void simple_hashtable_resize(SIMPLE_HASHTABLE *ht);

#define SHTS_DATA_UNSET ((void *)NULL)
#define SHTS_DATA_DELETED ((void *)0x01)
#define SHTS_DATA_USERNULL ((void *)0x02)
#define SHTS_IS_UNSET(sl) ((sl)->data == SHTS_DATA_UNSET)
#define SHTS_IS_DELETED(sl) ((sl)->data == SHTS_DATA_DELETED)
#define SHTS_IS_USERNULL(sl) ((sl)->data == SHTS_DATA_USERNULL)
#define SIMPLE_HASHTABLE_SLOT_DATA(sl) ((SHTS_IS_UNSET(sl) || SHTS_IS_DELETED(sl) || SHTS_IS_USERNULL(sl)) ? NULL : (sl)->data)
#define SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(sl) ((SHTS_IS_UNSET(sl) || SHTS_IS_DELETED(sl)) ? NULL : (sl)->data)

// IMPORTANT
// The pointer returned by this call is valid up to the next call of this function (or the resize one)
// If you need to cache something, cache the hash, not the slot pointer.
static inline SIMPLE_HASHTABLE_SLOT *simple_hashtable_get_slot(SIMPLE_HASHTABLE *ht, SIMPLE_HASHTABLE_HASH hash, bool resize) {
    ht->searches++;

    size_t slot;
    SIMPLE_HASHTABLE_SLOT *sl;
    SIMPLE_HASHTABLE_SLOT *deleted;

    slot = hash % ht->size;
    sl = &ht->hashtable[slot];
    deleted = SHTS_IS_DELETED(sl) ? sl : NULL;
    if(likely(!SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(sl) || sl->hash == hash))
        return (SHTS_IS_UNSET(sl) && deleted) ? deleted : sl;

    ht->collisions++;

    if(unlikely(resize && ht->size <= (ht->used << 4))) {
        simple_hashtable_resize(ht);

        slot = hash % ht->size;
        sl = &ht->hashtable[slot];
        deleted = (!deleted && SHTS_IS_DELETED(sl)) ? sl : deleted;
        if(likely(!SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(sl) || sl->hash == hash))
            return (SHTS_IS_UNSET(sl) && deleted) ? deleted : sl;

        ht->collisions++;
    }

    slot = ((hash >> SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS) + 1) % ht->size;
    sl = &ht->hashtable[slot];
    deleted = (!deleted && SHTS_IS_DELETED(sl)) ? sl : deleted;

    // Linear probing until we find it
    while (SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(sl) && sl->hash != hash) {
        slot = (slot + 1) % ht->size;  // Wrap around if necessary
        sl = &ht->hashtable[slot];
        deleted = (!deleted && SHTS_IS_DELETED(sl)) ? sl : deleted;
        ht->collisions++;
    }

    return (SHTS_IS_UNSET(sl) && deleted) ? deleted : sl;
}

static inline bool simple_hashtable_del_slot(SIMPLE_HASHTABLE *ht, SIMPLE_HASHTABLE_SLOT *sl) {
    if(SHTS_IS_UNSET(sl) || SHTS_IS_DELETED(sl))
        return false;

    ht->deletions++;
    ht->deleted++;

    sl->data = SHTS_DATA_DELETED;
    return true;
}

static inline void simple_hashtable_set_slot(SIMPLE_HASHTABLE *ht, SIMPLE_HASHTABLE_SLOT *sl, SIMPLE_HASHTABLE_HASH hash, void *data) {
    if(data == NULL)
        data = SHTS_DATA_USERNULL;

    if(unlikely(data == SHTS_DATA_UNSET || data == SHTS_DATA_DELETED)) {
        simple_hashtable_del_slot(ht, sl);
        return;
    }

    if(likely(SHTS_IS_UNSET(sl)))
        ht->used++;

    else if(unlikely(SHTS_IS_DELETED(sl)))
        ht->deleted--;

    sl->hash = hash;
    sl->data = data;
}

// IMPORTANT
// this call invalidates all SIMPLE_HASHTABLE_SLOT pointers
static inline void simple_hashtable_resize(SIMPLE_HASHTABLE *ht) {
    SIMPLE_HASHTABLE_SLOT *old = ht->hashtable;
    size_t old_size = ht->size;

    ht->resizes++;
    ht->size = (ht->size << 3) - 1;
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
    ht->used = ht->deleted = 0;
    for(size_t i = 0 ; i < old_size ; i++) {
        if(!SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(&old[i]))
            continue;

        SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(ht, old[i].hash, false);
        *slot = old[i];
        ht->used++;
    }

    freez(old);
}

// ----------------------------------------------------------------------------
// high level implementation

static inline void *simple_hashtable_set(SIMPLE_HASHTABLE *ht, void *key, size_t key_len, void *data) {
    XXH64_hash_t hash = XXH3_64bits(key, key_len);
    SIMPLE_HASHTABLE_SLOT *sl = simple_hashtable_get_slot(ht, hash, true);
    simple_hashtable_set_slot(ht, sl, hash, data);
    return SIMPLE_HASHTABLE_SLOT_DATA(sl);
}

static inline void *simple_hashtable_get(SIMPLE_HASHTABLE *ht, void *key, size_t key_len, void *data) {
    XXH64_hash_t hash = XXH3_64bits(key, key_len);
    SIMPLE_HASHTABLE_SLOT *sl = simple_hashtable_get_slot(ht, hash, true);
    return SIMPLE_HASHTABLE_SLOT_DATA(sl);
}

static inline bool simple_hashtable_del(SIMPLE_HASHTABLE *ht, void *key, size_t key_len, void *data) {
    XXH64_hash_t hash = XXH3_64bits(key, key_len);
    SIMPLE_HASHTABLE_SLOT *sl = simple_hashtable_get_slot(ht, hash, true);
    return simple_hashtable_del_slot(ht, sl);
}

#endif //NETDATA_SIMPLE_HASHTABLE_H
