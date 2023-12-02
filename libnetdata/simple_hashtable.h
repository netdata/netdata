// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIMPLE_HASHTABLE_H
#define NETDATA_SIMPLE_HASHTABLE_H

#ifndef XXH_INLINE_ALL
#define XXH_INLINE_ALL
#endif
#include "xxhash.h"

typedef uint64_t SIMPLE_HASHTABLE_HASH;
#define SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS 32

#ifndef SIMPLE_HASHTABLE_NAME
#define SIMPLE_HASHTABLE_NAME
#endif

#ifndef SIMPLE_HASHTABLE_VALUE_TYPE
#define SIMPLE_HASHTABLE_VALUE_TYPE void
#endif

// First layer of macro for token concatenation
#define CONCAT_INTERNAL(a, b) a ## b
// Second layer of macro, which ensures proper expansion
#define CONCAT(a, b) CONCAT_INTERNAL(a, b)

// define names for all structures and structures
#define simple_hashtable_init_named CONCAT(simple_hashtable_init, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_destroy_named CONCAT(simple_hashtable_destroy, SIMPLE_HASHTABLE_NAME)

#define simple_hashtable_slot_named CONCAT(simple_hashtable_slot, SIMPLE_HASHTABLE_NAME)
#define SIMPLE_HASHTABLE_SLOT_NAMED CONCAT(SIMPLE_HASHTABLE_SLOT, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_named CONCAT(simple_hashtable, SIMPLE_HASHTABLE_NAME)
#define SIMPLE_HASHTABLE_NAMED CONCAT(SIMPLE_HASHTABLE, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_resize_named CONCAT(simple_hashtable_resize, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_get_slot_named CONCAT(simple_hashtable_get_slot, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_del_slot_named CONCAT(simple_hashtable_del_slot, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_set_slot_named CONCAT(simple_hashtable_set_slot, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_first_read_only_named CONCAT(simple_hashtable_first_read_only, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_next_read_only_named CONCAT(simple_hashtable_next_read_only, SIMPLE_HASHTABLE_NAME)

#define simple_hashtable_sorted_binary_search_named CONCAT(simple_hashtable_sorted_binary_search, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_add_value_sorted_named CONCAT(simple_hashtable_add_value_sorted, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_del_value_sorted_named CONCAT(simple_hashtable_del_value_sorted, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_replace_value_sorted_named CONCAT(simple_hashtable_replace_value_sorted, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_sorted_array_first_read_only_named CONCAT(simple_hashtable_sorted_array_first_read_only, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_sorted_array_next_read_only_named CONCAT(simple_hashtable_sorted_array_next_read_only, SIMPLE_HASHTABLE_NAME)

typedef struct simple_hashtable_slot_named {
    SIMPLE_HASHTABLE_HASH hash;
    SIMPLE_HASHTABLE_VALUE_TYPE *data;
} SIMPLE_HASHTABLE_SLOT_NAMED;

typedef struct simple_hashtable_named {
    size_t resizes;
    size_t searches;
    size_t collisions;
    size_t deletions;
    size_t deleted;
    size_t used;
    size_t size;
    SIMPLE_HASHTABLE_SLOT_NAMED *hashtable;

#ifdef SIMPLE_HASHTABLE_SORT_FUNCTION
    struct {
        size_t used;
        size_t size;
        SIMPLE_HASHTABLE_VALUE_TYPE **array;
    } sorted;
#endif
} SIMPLE_HASHTABLE_NAMED;

#ifdef SIMPLE_HASHTABLE_SORT_FUNCTION
static inline size_t simple_hashtable_sorted_binary_search_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE *value) {
    size_t left = 0, right = ht->sorted.used;

    while (left < right) {
        size_t mid = left + (right - left) / 2;
        if (SIMPLE_HASHTABLE_SORT_FUNCTION(ht->sorted.array[mid], value) < 0)
            left = mid + 1;
        else
            right = mid;
    }

    return left;
}

static inline void simple_hashtable_add_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE *value) {
    size_t index = simple_hashtable_sorted_binary_search_named(ht, value);

    // Ensure there's enough space in the sorted array
    if (ht->sorted.used >= ht->sorted.size) {
        size_t size = ht->sorted.size ? ht->sorted.size * 2 : 64;
        SIMPLE_HASHTABLE_VALUE_TYPE **array = mallocz(size * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE *));
        if(ht->sorted.array) {
            memcpy(array, ht->sorted.array, ht->sorted.size * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE *));
            freez(ht->sorted.array);
        }
        ht->sorted.array = array;
        ht->sorted.size = size;
    }

    // Use memmove to shift elements and create space for the new element
    memmove(&ht->sorted.array[index + 1], &ht->sorted.array[index], (ht->sorted.used - index) * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE *));

    ht->sorted.array[index] = value;
    ht->sorted.used++;
}

static inline void simple_hashtable_del_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE *value) {
    size_t index = simple_hashtable_sorted_binary_search_named(ht, value);

    // Check if the value exists at the found index
    assert(index < ht->sorted.used && ht->sorted.array[index] == value);

    // Use memmove to shift elements and close the gap
    memmove(&ht->sorted.array[index], &ht->sorted.array[index + 1], (ht->sorted.used - index - 1) * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE *));
    ht->sorted.used--;
}

static inline void simple_hashtable_replace_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE *old_value, SIMPLE_HASHTABLE_VALUE_TYPE *new_value) {
    if(new_value == old_value)
        return;

    size_t old_value_index = simple_hashtable_sorted_binary_search_named(ht, old_value);
    assert(old_value_index < ht->sorted.used && ht->sorted.array[old_value_index] == old_value);

    int r = SIMPLE_HASHTABLE_SORT_FUNCTION(old_value, new_value);
    if(r == 0) {
        // Same value, so use the same index
        ht->sorted.array[old_value_index] = new_value;
        return;
    }

    size_t new_value_index = simple_hashtable_sorted_binary_search_named(ht, new_value);
    if(old_value_index == new_value_index) {
        // Not the same value, but still at the same index
        ht->sorted.array[old_value_index] = new_value;
        return;
    }
    else if (old_value_index < new_value_index) {
        // The old value is before the new value
        size_t shift_start = old_value_index + 1;
        size_t shift_end = new_value_index - 1;
        size_t shift_size = shift_end - old_value_index;

        memmove(&ht->sorted.array[old_value_index], &ht->sorted.array[shift_start], shift_size * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE *));
        ht->sorted.array[shift_end] = new_value;
    }
    else {
        // The old value is after the new value
        size_t shift_start = new_value_index;
        size_t shift_end = old_value_index;
        size_t shift_size = shift_end - new_value_index;

        memmove(&ht->sorted.array[new_value_index + 1], &ht->sorted.array[shift_start], shift_size * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE *));
        ht->sorted.array[new_value_index] = new_value;
    }
}

static inline SIMPLE_HASHTABLE_VALUE_TYPE **simple_hashtable_sorted_array_first_read_only_named(SIMPLE_HASHTABLE_NAMED *ht) {
    if (ht->sorted.used > 0) {
        return &ht->sorted.array[0];
    }
    return NULL;
}

static inline SIMPLE_HASHTABLE_VALUE_TYPE **simple_hashtable_sorted_array_next_read_only_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE **last) {
    if (!last) return NULL;

    // Calculate the current position in the sorted array
    size_t currentIndex = last - ht->sorted.array;

    // Proceed to the next element if it exists
    if (currentIndex + 1 < ht->sorted.used) {
        return &ht->sorted.array[currentIndex + 1];
    }

    // If no more elements, return NULL
    return NULL;
}

#define SIMPLE_HASHTABLE_SORTED_FOREACH_READ_ONLY(ht, var, type, name) \
    for (type **(var) = simple_hashtable_sorted_array_first_read_only ## name(ht); \
         var; \
         (var) = simple_hashtable_sorted_array_next_read_only ## name(ht, var))

#define SIMPLE_HASHTABLE_SORTED_FOREACH_READ_ONLY_VALUE(var) (*(var))

#else
static inline void simple_hashtable_add_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht __maybe_unused, SIMPLE_HASHTABLE_VALUE_TYPE *value __maybe_unused) { ; }
static inline void simple_hashtable_del_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht __maybe_unused, SIMPLE_HASHTABLE_VALUE_TYPE *value __maybe_unused) { ; }
static inline void simple_hashtable_replace_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht __maybe_unused, SIMPLE_HASHTABLE_VALUE_TYPE *old_value __maybe_unused, SIMPLE_HASHTABLE_VALUE_TYPE *new_value __maybe_unused) { ; }
#endif

static void simple_hashtable_init_named(SIMPLE_HASHTABLE_NAMED *ht, size_t size) {
    memset(ht, 0, sizeof(*ht));
    ht->size = size;
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
}

static void simple_hashtable_destroy_named(SIMPLE_HASHTABLE_NAMED *ht) {
#ifdef SIMPLE_HASHTABLE_SORT_FUNCTION
    freez(ht->sorted.array);
#endif

    freez(ht->hashtable);
    memset(ht, 0, sizeof(*ht));
}

static inline void simple_hashtable_resize_named(SIMPLE_HASHTABLE_NAMED *ht);

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
static inline SIMPLE_HASHTABLE_SLOT_NAMED *simple_hashtable_get_slot_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_HASH hash, bool resize) {
    ht->searches++;

    size_t slot;
    SIMPLE_HASHTABLE_SLOT_NAMED *sl;
    SIMPLE_HASHTABLE_SLOT_NAMED *deleted;

    slot = hash % ht->size;
    sl = &ht->hashtable[slot];
    deleted = SHTS_IS_DELETED(sl) ? sl : NULL;
    if(likely(!SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(sl) || sl->hash == hash))
        return (SHTS_IS_UNSET(sl) && deleted) ? deleted : sl;

    ht->collisions++;

    if(unlikely(resize && (ht->size <= (ht->used << 1) || ht->used >= ht->size))) {
        simple_hashtable_resize_named(ht);

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

static inline bool simple_hashtable_del_slot_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_SLOT_NAMED *sl) {
    if(SHTS_IS_UNSET(sl) || SHTS_IS_DELETED(sl))
        return false;

    ht->deletions++;
    ht->deleted++;

    simple_hashtable_del_value_sorted_named(ht, SIMPLE_HASHTABLE_SLOT_DATA(sl));

    sl->data = SHTS_DATA_DELETED;
    return true;
}

static inline void simple_hashtable_set_slot_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_SLOT_NAMED *sl, SIMPLE_HASHTABLE_HASH hash, SIMPLE_HASHTABLE_VALUE_TYPE *data) {
    if(data == NULL)
        data = SHTS_DATA_USERNULL;

    if(unlikely(data == SHTS_DATA_UNSET || data == SHTS_DATA_DELETED)) {
        simple_hashtable_del_slot_named(ht, sl);
        return;
    }

    if(likely(SHTS_IS_UNSET(sl))) {
        simple_hashtable_add_value_sorted_named(ht, data);
        ht->used++;
    }

    else if(unlikely(SHTS_IS_DELETED(sl))) {
        ht->deleted--;
    }

    else
        simple_hashtable_replace_value_sorted_named(ht, SIMPLE_HASHTABLE_SLOT_DATA(sl), data);

    sl->hash = hash;
    sl->data = data;
}

// IMPORTANT
// this call invalidates all SIMPLE_HASHTABLE_SLOT_NAMED pointers
static inline void simple_hashtable_resize_named(SIMPLE_HASHTABLE_NAMED *ht) {
    SIMPLE_HASHTABLE_SLOT_NAMED *old = ht->hashtable;
    size_t old_size = ht->size;

    ht->resizes++;
    ht->size = (ht->size << 1) - ((ht->size > 16) ? 1 : 0);
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
    ht->used = ht->deleted = 0;
    for(size_t i = 0 ; i < old_size ; i++) {
        if(!SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(&old[i]))
            continue;

        SIMPLE_HASHTABLE_SLOT_NAMED *slot = simple_hashtable_get_slot_named(ht, old[i].hash, false);
        *slot = old[i];
        ht->used++;
    }

    freez(old);
}

// ----------------------------------------------------------------------------
// hashtable traversal, in read-only mode
// the hashtable should not be modified while the traversal is taking place

static inline SIMPLE_HASHTABLE_SLOT_NAMED *simple_hashtable_first_read_only_named(SIMPLE_HASHTABLE_NAMED *ht) {
    for(size_t i = 0; i < ht->used ;i++) {
        SIMPLE_HASHTABLE_SLOT_NAMED *sl = &ht->hashtable[i];
        if(!SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(sl))
            return sl;
    }

    return NULL;
}

static inline SIMPLE_HASHTABLE_SLOT_NAMED *simple_hashtable_next_read_only_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_SLOT_NAMED *last) {
    if (!last) return NULL;

    // Calculate the current position in the array
    size_t currentIndex = last - ht->hashtable;

    // Iterate over the hashtable starting from the next element
    for (size_t i = currentIndex + 1; i < ht->size; i++) {
        SIMPLE_HASHTABLE_SLOT_NAMED *sl = &ht->hashtable[i];
        if (!SIMPLE_HASHTABLE_SLOT_UNSET_OR_DELETED(sl)) {
            return sl;
        }
    }

    // If no more data slots are found, return NULL
    return NULL;
}

#define SIMPLE_HASHTABLE_FOREACH_READ_ONLY(ht, var, name) \
    for(struct simple_hashtable_slot ## name *(var) = simple_hashtable_first_read_only ## name(ht); \
    var;                                                                                                             \
    (var) = simple_hashtable_next_read_only ## name(ht, var))

#define SIMPLE_HASHTABLE_FOREACH_READ_ONLY_VALUE(var) SIMPLE_HASHTABLE_SLOT_DATA(var)

// ----------------------------------------------------------------------------
// high level implementation

#ifdef SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION

#define simple_hashtable_set_named CONCAT(simple_hashtable_set, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_get_named CONCAT(simple_hashtable_get, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_del_named CONCAT(simple_hashtable_del, SIMPLE_HASHTABLE_NAME)

static inline SIMPLE_HASHTABLE_VALUE_TYPE *simple_hashtable_set_named(SIMPLE_HASHTABLE_NAMED *ht, void *key, size_t key_len, SIMPLE_HASHTABLE_VALUE_TYPE *data) {
    XXH64_hash_t hash = XXH3_64bits(key, key_len);
    SIMPLE_HASHTABLE_SLOT_NAMED *sl = simple_hashtable_get_slot_named(ht, hash, true);
    simple_hashtable_set_slot_named(ht, sl, hash, data);
    return SIMPLE_HASHTABLE_SLOT_DATA(sl);
}

static inline SIMPLE_HASHTABLE_VALUE_TYPE *simple_hashtable_get_named(SIMPLE_HASHTABLE_NAMED *ht, void *key, size_t key_len, SIMPLE_HASHTABLE_VALUE_TYPE *data) {
    XXH64_hash_t hash = XXH3_64bits(key, key_len);
    SIMPLE_HASHTABLE_SLOT_NAMED *sl = simple_hashtable_get_slot_named(ht, hash, true);
    return SIMPLE_HASHTABLE_SLOT_DATA(sl);
}

static inline bool simple_hashtable_del_named(SIMPLE_HASHTABLE_NAMED *ht, void *key, size_t key_len, SIMPLE_HASHTABLE_VALUE_TYPE *data) {
    XXH64_hash_t hash = XXH3_64bits(key, key_len);
    SIMPLE_HASHTABLE_SLOT_NAMED *sl = simple_hashtable_get_slot_named(ht, hash, true);
    return simple_hashtable_del_slot_named(ht, sl);
}

#endif // SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION

#endif //NETDATA_SIMPLE_HASHTABLE_H
