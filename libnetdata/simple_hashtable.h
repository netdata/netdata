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

static inline SIMPLE_HASHTABLE_SLOT *simple_hashtable_get_slot(SIMPLE_HASHTABLE *ht, SIMPLE_HASHTABLE_HASH hash, bool resize) {
    // IMPORTANT:
    // If the hashtable supported deletions, we would need to have a special slot.data value
    // to mark deleted values and assume they are occupied during lookup, but empty during insert.
    // But for our case, we don't need it, since we never delete items from the hashtable.

    ht->searches++;

    size_t slot = hash % ht->size;
    if(likely(!ht->hashtable[slot].data || ht->hashtable[slot].hash == hash))
        return &ht->hashtable[slot];

    ht->collisions++;

    if(unlikely(resize && ht->size <= (ht->used << 4))) {
        simple_hashtable_resize(ht);

        slot = hash % ht->size;
        if(likely(!ht->hashtable[slot].data || ht->hashtable[slot].hash == hash))
            return &ht->hashtable[slot];

        ht->collisions++;
    }

    slot = ((hash >> SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS) + 1) % ht->size;
    // Linear probing until we find it
    while (ht->hashtable[slot].data && ht->hashtable[slot].hash != hash) {
        slot = (slot + 1) % ht->size;  // Wrap around if necessary
        ht->collisions++;
    }

    return &ht->hashtable[slot];
}

static inline void simple_hashtable_resize(SIMPLE_HASHTABLE *ht) {
    SIMPLE_HASHTABLE_SLOT *old = ht->hashtable;
    size_t old_size = ht->size;

    ht->resizes++;
    ht->size = (ht->size << 3) - 1;
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
    for(size_t i = 0 ; i < old_size ; i++) {
        if(!old[i].data)
            continue;

        SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(ht, old[i].hash, false);
        *slot = old[i];
    }

    freez(old);
}

#endif //NETDATA_SIMPLE_HASHTABLE_H
