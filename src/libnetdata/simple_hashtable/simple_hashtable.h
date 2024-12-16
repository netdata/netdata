// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIMPLE_HASHTABLE_H
#define NETDATA_SIMPLE_HASHTABLE_H

typedef uint64_t SIMPLE_HASHTABLE_HASH;
#define SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS 32

/*
 * CONFIGURATION
 *
 * SIMPLE_HASHTABLE_NAME
 * The name of the hashtable - all functions and defines will have this name appended
 * Example: #define SIMPLE_HASHTABLE_NAME _FACET_KEY
 *
 * SIMPLE_HASHTABLE_VALUE_TYPE and SIMPLE_HASHTABLE_KEY_TYPE
 * The data types of values and keys - optional - setting them will enable strict type checking by the compiler.
 * If undefined, they both default to void.
 *
 * SIMPLE_HASHTABLE_SORT_FUNCTION
 * A function name that accepts 2x values and compares them for sorting (returning -1, 0, 1).
 * When set, the hashtable will maintain an always sorted array of the values in the hashtable.
 * Do not use this for non-static hashtables. So, if your data is changing all the time, this can make the
 * hashtable quite slower (it memmove()s an array of pointers to keep it sorted, on every single change).
 *
 * SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION and SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION
 * The hashtable can either compare just hashes (the default), or hashes and keys (when these are set).
 * Both need to be set for this feature to be enabled.
 *
 *    - SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION
 *      The name of a function accepting SIMPLE_HASHTABLE_VALUE_TYPE pointer.
 *      It should return a pointer to SIMPLE_HASHTABLE_KEY_TYPE.
 *      This function is called prior to SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION to extract the key from a value.
 *      It is also called during hashtable resize, to rehash all values in the hashtable.
 *
 *    - SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION
 *      The name of a function accepting 2x SIMPLE_HASHTABLE_KEY_TYPE pointers.
 *      It should return true when the keys match.
 *      This function is only called when the hashes match, to verify that the keys also match.
 *
 * SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION
 * If defined, 3x functions will be injected for easily working with the hashtable.
 *
 */


#ifndef SIMPLE_HASHTABLE_NAME
#define SIMPLE_HASHTABLE_NAME
#endif

#ifndef SIMPLE_HASHTABLE_VALUE_TYPE
#define SIMPLE_HASHTABLE_VALUE_TYPE void *
#endif

#ifndef SIMPLE_HASHTABLE_KEY_TYPE
#define SIMPLE_HASHTABLE_KEY_TYPE void
#endif

#ifndef SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION
#undef SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION
#endif

// check during compilation
_Static_assert(sizeof(SIMPLE_HASHTABLE_VALUE_TYPE) <= sizeof(uint64_t),
               "simple hashtable value cannot be bigger than 8 bytes");

#if defined(SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION)
static inline SIMPLE_HASHTABLE_KEY_TYPE *SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION(SIMPLE_HASHTABLE_VALUE_TYPE);
#endif

#if defined(SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION)
static inline bool SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION(SIMPLE_HASHTABLE_KEY_TYPE *, SIMPLE_HASHTABLE_KEY_TYPE *);
#endif

// First layer of macro for token concatenation
#ifndef CONCAT_INDIRECT
#define CONCAT_INDIRECT(a, b) a ## b
#endif
// Second layer of macro, which ensures proper expansion
#ifndef CONCAT
#define CONCAT(a, b) CONCAT_INDIRECT(a, b)
#endif

// define names for all structures and structures
#define simple_hashtable_init_named CONCAT(simple_hashtable_init, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_destroy_named CONCAT(simple_hashtable_destroy, SIMPLE_HASHTABLE_NAME)

#define simple_hashtable_slot_named CONCAT(simple_hashtable_slot, SIMPLE_HASHTABLE_NAME)
#define SIMPLE_HASHTABLE_SLOT_NAMED CONCAT(SIMPLE_HASHTABLE_SLOT, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_named CONCAT(simple_hashtable, SIMPLE_HASHTABLE_NAME)
#define SIMPLE_HASHTABLE_NAMED CONCAT(SIMPLE_HASHTABLE, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_resize_named CONCAT(simple_hashtable_resize, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_can_use_slot_named CONCAT(simple_hashtable_keys_match, SIMPLE_HASHTABLE_NAME)
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
    union {
        SIMPLE_HASHTABLE_VALUE_TYPE data;
        uint64_t v; // make sure it is always 64bit (required to store our deleted or usernull values)
    };
} SIMPLE_HASHTABLE_SLOT_NAMED;

typedef struct simple_hashtable_named {
    size_t resizes;
    size_t searches;
    size_t collisions;
    size_t additions;
    size_t deletions;
    size_t deleted;
    size_t used;
    size_t size;
    bool needs_cleanup;
    SIMPLE_HASHTABLE_SLOT_NAMED *hashtable;

#ifdef SIMPLE_HASHTABLE_SORT_FUNCTION
    struct {
        size_t used;
        size_t size;
        SIMPLE_HASHTABLE_VALUE_TYPE *array;
    } sorted;
#endif
} SIMPLE_HASHTABLE_NAMED;

#ifdef SIMPLE_HASHTABLE_SORT_FUNCTION
static inline size_t simple_hashtable_sorted_binary_search_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE value) {
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

static inline void simple_hashtable_add_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE value) {
    size_t index = simple_hashtable_sorted_binary_search_named(ht, value);

    // Ensure there's enough space in the sorted array
    if (ht->sorted.used >= ht->sorted.size) {
        size_t size = ht->sorted.size ? ht->sorted.size * 2 : 64;
        SIMPLE_HASHTABLE_VALUE_TYPE *array = mallocz(size * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE));
        if(ht->sorted.array) {
            memcpy(array, ht->sorted.array, ht->sorted.size * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE));
            freez(ht->sorted.array);
        }
        ht->sorted.array = array;
        ht->sorted.size = size;
    }

    // Use memmove to shift elements and create space for the new element
    memmove(&ht->sorted.array[index + 1], &ht->sorted.array[index], (ht->sorted.used - index) * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE));

    ht->sorted.array[index] = value;
    ht->sorted.used++;
}

static inline void simple_hashtable_del_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE value) {
    size_t index = simple_hashtable_sorted_binary_search_named(ht, value);

    // Check if the value exists at the found index
    assert(index < ht->sorted.used && ht->sorted.array[index] == value);

    // Use memmove to shift elements and close the gap
    memmove(&ht->sorted.array[index], &ht->sorted.array[index + 1], (ht->sorted.used - index - 1) * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE));
    ht->sorted.used--;
}

static inline void simple_hashtable_replace_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE old_value, SIMPLE_HASHTABLE_VALUE_TYPE new_value) {
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

        memmove(&ht->sorted.array[old_value_index], &ht->sorted.array[shift_start], shift_size * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE));
        ht->sorted.array[shift_end] = new_value;
    }
    else {
        // The old value is after the new value
        size_t shift_start = new_value_index;
        size_t shift_end = old_value_index;
        size_t shift_size = shift_end - new_value_index;

        memmove(&ht->sorted.array[new_value_index + 1], &ht->sorted.array[shift_start], shift_size * sizeof(SIMPLE_HASHTABLE_VALUE_TYPE));
        ht->sorted.array[new_value_index] = new_value;
    }
}

static inline SIMPLE_HASHTABLE_VALUE_TYPE *simple_hashtable_sorted_array_first_read_only_named(SIMPLE_HASHTABLE_NAMED *ht) {
    if (ht->sorted.used > 0) {
        return &ht->sorted.array[0];
    }
    return NULL;
}

static inline SIMPLE_HASHTABLE_VALUE_TYPE *simple_hashtable_sorted_array_next_read_only_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_VALUE_TYPE *last) {
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
static inline void simple_hashtable_add_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht __maybe_unused, SIMPLE_HASHTABLE_VALUE_TYPE value __maybe_unused) { ; }
static inline void simple_hashtable_del_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht __maybe_unused, SIMPLE_HASHTABLE_VALUE_TYPE value __maybe_unused) { ; }
static inline void simple_hashtable_replace_value_sorted_named(SIMPLE_HASHTABLE_NAMED *ht __maybe_unused, SIMPLE_HASHTABLE_VALUE_TYPE old_value __maybe_unused, SIMPLE_HASHTABLE_VALUE_TYPE new_value __maybe_unused) { ; }
#endif

static inline void simple_hashtable_init_named(SIMPLE_HASHTABLE_NAMED *ht, size_t size) {
    memset(ht, 0, sizeof(*ht));
    ht->size = size;
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
}

static inline void simple_hashtable_destroy_named(SIMPLE_HASHTABLE_NAMED *ht) {
#ifdef SIMPLE_HASHTABLE_SORT_FUNCTION
    freez(ht->sorted.array);
#endif

    freez(ht->hashtable);
    memset(ht, 0, sizeof(*ht));
}

static inline void simple_hashtable_resize_named(SIMPLE_HASHTABLE_NAMED *ht);

#define simple_hashtable_data_unset ((uint64_t)0)
#define simple_hashtable_data_deleted ((uint64_t)UINT64_MAX)
#define simple_hashtable_data_usernull ((uint64_t)(UINT64_MAX - 1))
#define simple_hashtable_is_slot_unset(sl) ((sl)->v == simple_hashtable_data_unset)
#define simple_hashtable_is_slot_deleted(sl) ((sl)->v == simple_hashtable_data_deleted)
#define simple_hashtable_is_slot_usernull(sl) ((sl)->v == simple_hashtable_data_usernull)
#define SIMPLE_HASHTABLE_SLOT_DATA(sl) \
    ((simple_hashtable_is_slot_unset(sl) || simple_hashtable_is_slot_deleted(sl) || simple_hashtable_is_slot_usernull(sl)) \
     ? (typeof((sl)->data))0 \
     : (sl)->data)

static inline bool simple_hashtable_can_use_slot_named(
        SIMPLE_HASHTABLE_SLOT_NAMED *sl, SIMPLE_HASHTABLE_HASH hash,
        SIMPLE_HASHTABLE_KEY_TYPE *key __maybe_unused) {

    if(simple_hashtable_is_slot_unset(sl))
        return true;

    if(simple_hashtable_is_slot_deleted(sl))
        return false;

    if(sl->hash == hash) {
#if defined(SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION) && defined(SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION)
        return SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION(SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION(SIMPLE_HASHTABLE_SLOT_DATA(sl)), key);
#else
        return true;
#endif
    }

    return false;
}

#define SIMPLE_HASHTABLE_NEEDS_RESIZE(ht) ((ht)->size <= ((ht)->used - (ht)->deleted) << 1 || (ht)->used >= (ht)->size)

// IMPORTANT: the pointer returned by this call is valid up to the next call of this function (or the resize one).
// If you need to cache something, cache the hash, not the slot pointer.
static inline SIMPLE_HASHTABLE_SLOT_NAMED *simple_hashtable_get_slot_named(
        SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_HASH hash,
        SIMPLE_HASHTABLE_KEY_TYPE *key, bool resize) {

    // This function finds the requested hash and key in the hashtable.
    // It uses a second version of the hash in case of collisions, and then linear probing.
    // It may resize the hashtable if it is more than 50% full.

    // Deleted items remain in the hashtable, but they are marked as DELETED.
    // Reuse of DELETED slots happens only if the slot to be returned is UNSET.
    // So, when looking up for an item, it tries to find it, assuming DELETED
    // slots are occupied. If the item to be returned is UNSET, and it has
    // encountered a DELETED slot, it returns the DELETED one instead of the UNSET.

    ht->searches++;

    size_t slot;
    SIMPLE_HASHTABLE_SLOT_NAMED *sl;
    SIMPLE_HASHTABLE_SLOT_NAMED *deleted;

    slot = hash % ht->size;
    sl = &ht->hashtable[slot];
    deleted = simple_hashtable_is_slot_deleted(sl) ? sl : NULL;
    if(likely(simple_hashtable_can_use_slot_named(sl, hash, key)))
        return (simple_hashtable_is_slot_unset(sl) && deleted) ? deleted : sl;

    ht->collisions++;

    if(unlikely(resize && (ht->needs_cleanup || SIMPLE_HASHTABLE_NEEDS_RESIZE(ht)))) {
        simple_hashtable_resize_named(ht);
        deleted = NULL; // our deleted pointer is not valid anymore

        slot = hash % ht->size;
        sl = &ht->hashtable[slot];
        if(likely(simple_hashtable_can_use_slot_named(sl, hash, key)))
            return sl;

        ht->collisions++;
    }

    slot = ((hash >> SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS) + 1) % ht->size;
    sl = &ht->hashtable[slot];
    deleted = (!deleted && simple_hashtable_is_slot_deleted(sl)) ? sl : deleted;

    // Linear probing until we find it
    SIMPLE_HASHTABLE_SLOT_NAMED *sl_started = sl;
    size_t collisions_started = ht->collisions;
    while (!simple_hashtable_can_use_slot_named(sl, hash, key)) {
        slot = (slot + 1) % ht->size;  // Wrap around if necessary
        sl = &ht->hashtable[slot];
        deleted = (!deleted && simple_hashtable_is_slot_deleted(sl)) ? sl : deleted;
        ht->collisions++;

        if(sl == sl_started) {
            if(deleted) {
                // we looped through all items, and we didn't find a free slot,
                // but we have found a deleted slot, so return it.
                return deleted;
            }
            else if(resize) {
                // the hashtable is full, without any deleted slots.
                // we need to resize it now.
                simple_hashtable_resize_named(ht);
                return simple_hashtable_get_slot_named(ht, hash, key, false);
            }
            else {
                // the hashtable is full, but resize is false.
                // this should never happen.
                assert(sl != sl_started);
            }
        }
    }

    if((ht->collisions - collisions_started) > (ht->size / 2) && ht->deleted >= (ht->size / 3)) {
        // we traversed through half of the hashtable to find a slot,
        // but we have more than 1/3 deleted items
        ht->needs_cleanup = true;
    }

    return (simple_hashtable_is_slot_unset(sl) && deleted) ? deleted : sl;
}

static inline bool simple_hashtable_del_slot_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_SLOT_NAMED *sl) {
    if(simple_hashtable_is_slot_unset(sl) || simple_hashtable_is_slot_deleted(sl))
        return false;

    ht->deletions++;
    ht->deleted++;

    simple_hashtable_del_value_sorted_named(ht, SIMPLE_HASHTABLE_SLOT_DATA(sl));

    sl->v = simple_hashtable_data_deleted;
    return true;
}

static inline void simple_hashtable_set_slot_named(
        SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_SLOT_NAMED *sl,
        SIMPLE_HASHTABLE_HASH hash, SIMPLE_HASHTABLE_VALUE_TYPE data) {

    uint64_t v;
    if(unlikely(data == (SIMPLE_HASHTABLE_VALUE_TYPE)0))
        v = simple_hashtable_data_usernull;
    else {
#ifdef SIMPLE_HASHTABLE_VALUE_TYPE_IS_NOT_POINTER
        v = (uint64_t)data;
#else
        v = (uint64_t)(uintptr_t)data;
#endif
    }

    if(unlikely(v == simple_hashtable_data_unset || v == simple_hashtable_data_deleted)) {
        // the new value is unset or deleted,
        // mark the slot as deleted (updating the sorted array as necessary)
        simple_hashtable_del_slot_named(ht, sl);
        return;
    }

    if(likely(simple_hashtable_is_slot_unset(sl))) {
        // the slot is empty,
        // add the new value to the sorted array (when sorting is requested)
        simple_hashtable_add_value_sorted_named(ht, data);
        ht->used++;
    }

    else if(unlikely(simple_hashtable_is_slot_deleted(sl))) {
        // the slot is deleted,
        // add the new value to the sorted array (when sorting is requested)
        simple_hashtable_add_value_sorted_named(ht, data);
        ht->deleted--;
    }

    else {
        // the slot is occupied,
        // replace the old value with the new value in the sorted array (when sorting is requested)
        simple_hashtable_replace_value_sorted_named(ht, SIMPLE_HASHTABLE_SLOT_DATA(sl), data);
    }

    // update the slot with the new value
    sl->hash = hash;
    sl->v = v;

    ht->additions++;
}

// IMPORTANT
// this call invalidates all SIMPLE_HASHTABLE_SLOT_NAMED pointers
static inline void simple_hashtable_resize_named(SIMPLE_HASHTABLE_NAMED *ht) {
    SIMPLE_HASHTABLE_SLOT_NAMED *old = ht->hashtable;
    size_t old_size = ht->size;

    size_t new_size = ht->size;

    if(SIMPLE_HASHTABLE_NEEDS_RESIZE(ht))
        new_size = (ht->size << 1) - ((ht->size > 16) ? 1 : 0);

    ht->resizes++;
    ht->size = new_size;
    ht->hashtable = callocz(new_size, sizeof(*ht->hashtable));
    size_t used = 0;
    for(size_t i = 0 ; i < old_size ; i++) {
        SIMPLE_HASHTABLE_SLOT_NAMED *slot = &old[i];
        if(simple_hashtable_is_slot_unset(slot) || simple_hashtable_is_slot_deleted(slot))
            continue;

        SIMPLE_HASHTABLE_KEY_TYPE *key = NULL;

#if defined(SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION) && defined(SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION)
        SIMPLE_HASHTABLE_VALUE_TYPE value = SIMPLE_HASHTABLE_SLOT_DATA(slot);
        key = SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION(value);
#endif

        SIMPLE_HASHTABLE_SLOT_NAMED *slot2 = simple_hashtable_get_slot_named(ht, slot->hash, key, false);
        *slot2 = *slot;
        used++;
    }

    assert(used == ht->used - ht->deleted);

    ht->used = used;
    ht->deleted = 0;
    ht->needs_cleanup = false;

    freez(old);
}

// ----------------------------------------------------------------------------
// hashtable traversal, in read-only mode
// the hashtable should not be modified while the traversal is taking place

static inline SIMPLE_HASHTABLE_SLOT_NAMED *simple_hashtable_first_read_only_named(SIMPLE_HASHTABLE_NAMED *ht) {
    for(size_t i = 0; i < ht->size ;i++) {
        SIMPLE_HASHTABLE_SLOT_NAMED *sl = &ht->hashtable[i];
        if(!simple_hashtable_is_slot_unset(sl) && !simple_hashtable_is_slot_deleted(sl))
            return sl;
    }

    return NULL;
}

static inline SIMPLE_HASHTABLE_SLOT_NAMED *simple_hashtable_next_read_only_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_SLOT_NAMED *last) {
    if (!last) return NULL;

    // Calculate the current position in the array
    size_t index = last - ht->hashtable;

    // Iterate over the hashtable starting from the next element
    for (size_t i = index + 1; i < ht->size; i++) {
        SIMPLE_HASHTABLE_SLOT_NAMED *sl = &ht->hashtable[i];
        if (!simple_hashtable_is_slot_unset(sl) && !simple_hashtable_is_slot_deleted(sl)) {
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

#ifndef XXH_INLINE_ALL
#define XXH_INLINE_ALL
#endif
#include "../xxHash/xxhash.h"

#define simple_hashtable_set_named CONCAT(simple_hashtable_set, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_get_named CONCAT(simple_hashtable_get, SIMPLE_HASHTABLE_NAME)
#define simple_hashtable_del_named CONCAT(simple_hashtable_del, SIMPLE_HASHTABLE_NAME)

static inline SIMPLE_HASHTABLE_VALUE_TYPE simple_hashtable_set_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_KEY_TYPE *key, size_t key_len, SIMPLE_HASHTABLE_VALUE_TYPE data) {
    XXH64_hash_t hash = XXH3_64bits((void *)key, key_len);
    SIMPLE_HASHTABLE_SLOT_NAMED *sl = simple_hashtable_get_slot_named(ht, hash, key, true);
    simple_hashtable_set_slot_named(ht, sl, hash, data);
    return SIMPLE_HASHTABLE_SLOT_DATA(sl);
}

static inline SIMPLE_HASHTABLE_VALUE_TYPE simple_hashtable_get_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_KEY_TYPE *key, size_t key_len) {
    XXH64_hash_t hash = XXH3_64bits((void *)key, key_len);
    SIMPLE_HASHTABLE_SLOT_NAMED *sl = simple_hashtable_get_slot_named(ht, hash, key, true);
    return SIMPLE_HASHTABLE_SLOT_DATA(sl);
}

static inline bool simple_hashtable_del_named(SIMPLE_HASHTABLE_NAMED *ht, SIMPLE_HASHTABLE_KEY_TYPE *key, size_t key_len) {
    XXH64_hash_t hash = XXH3_64bits((void *)key, key_len);
    SIMPLE_HASHTABLE_SLOT_NAMED *sl = simple_hashtable_get_slot_named(ht, hash, key, true);
    return simple_hashtable_del_slot_named(ht, sl);
}

#endif // SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION

// ----------------------------------------------------------------------------
// Clear the preprocessor defines of simple_hashtable.h
// allowing simple_hashtable.h to be included multiple times
// with different configuration each time.

#include "simple_hashtable_undef.h"

#endif //NETDATA_SIMPLE_HASHTABLE_H
