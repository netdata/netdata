// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIMPLE_HASHTABLE_H
#define NETDATA_SIMPLE_HASHTABLE_H

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

#define SHTS_IS_UNSET(sl) ((sl)->data == NULL)
#define SHTS_IS_DELETED(sl) ((sl)->data == (void *)0x01)
#define SIMPLE_HASHTABLE_SLOT_DATA(sl) ((SHTS_IS_UNSET(sl) || SHTS_IS_DELETED(sl)) ? NULL : (sl)->data)

static inline SIMPLE_HASHTABLE_SLOT *simple_hashtable_get_slot(SIMPLE_HASHTABLE *ht, SIMPLE_HASHTABLE_HASH hash, bool resize) {
    ht->searches++;

    size_t slot;
    SIMPLE_HASHTABLE_SLOT *sl;
    SIMPLE_HASHTABLE_SLOT *deleted;

    slot = hash % ht->size;
    sl = &ht->hashtable[slot];
    deleted = SHTS_IS_DELETED(sl) ? sl : NULL;
    if(likely(!SIMPLE_HASHTABLE_SLOT_DATA(sl) || sl->hash == hash))
        return (SHTS_IS_UNSET(sl) && deleted) ? deleted : sl;

    ht->collisions++;

    if(unlikely(resize && ht->size <= (ht->used << 4))) {
        simple_hashtable_resize(ht);

        slot = hash % ht->size;
        sl = &ht->hashtable[slot];
        deleted = (!deleted && SHTS_IS_DELETED(sl)) ? sl : deleted;
        if(likely(!SIMPLE_HASHTABLE_SLOT_DATA(sl) || sl->hash == hash))
            return (SHTS_IS_UNSET(sl) && deleted) ? deleted : sl;

        ht->collisions++;
    }

    slot = ((hash >> SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS) + 1) % ht->size;
    sl = &ht->hashtable[slot];
    deleted = (!deleted && SHTS_IS_DELETED(sl)) ? sl : deleted;

    // Linear probing until we find it
    while (SIMPLE_HASHTABLE_SLOT_DATA(sl) && sl->hash != hash) {
        slot = (slot + 1) % ht->size;  // Wrap around if necessary
        sl = &ht->hashtable[slot];
        deleted = (!deleted && SHTS_IS_DELETED(sl)) ? sl : deleted;
        ht->collisions++;
    }

    return (SHTS_IS_UNSET(sl) && deleted) ? deleted : sl;
}

static inline void simple_hashtable_delete_slot(SIMPLE_HASHTABLE *ht, SIMPLE_HASHTABLE_SLOT *sl) {
    if(SHTS_IS_UNSET(sl) || SHTS_IS_DELETED(sl))
        return;

    ht->deletions++;
    ht->deleted++;

    sl->data = (void *)0x01;
}

static inline void simple_hashtable_set_slot(SIMPLE_HASHTABLE *ht, SIMPLE_HASHTABLE_SLOT *sl, SIMPLE_HASHTABLE_HASH hash, void *data) {
    if(unlikely(data == NULL || data == (void *)0x01)) {
        simple_hashtable_delete_slot(ht, data);
        return;
    }

    if(likely(SHTS_IS_UNSET(sl)))
        ht->used++;

    else if(unlikely(SHTS_IS_DELETED(sl)))
        ht->deleted--;

    sl->hash = hash;
    sl->data = data;
}

static inline void simple_hashtable_resize(SIMPLE_HASHTABLE *ht) {
    SIMPLE_HASHTABLE_SLOT *old = ht->hashtable;
    size_t old_size = ht->size;

    ht->resizes++;
    ht->size = (ht->size << 3) - 1;
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
    ht->used = ht->deleted = 0;
    for(size_t i = 0 ; i < old_size ; i++) {
        if(!SIMPLE_HASHTABLE_SLOT_DATA(&old[i]))
            continue;

        SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(ht, old[i].hash, false);
        *slot = old[i];
        ht->used++;
    }

    freez(old);
}

#endif //NETDATA_SIMPLE_HASHTABLE_H
