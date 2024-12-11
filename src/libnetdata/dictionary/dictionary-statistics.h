// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_STATISTICS_H
#define NETDATA_DICTIONARY_STATISTICS_H

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// memory statistics

#ifdef DICT_WITH_STATS
static inline void DICTIONARY_STATS_PLUS_MEMORY(DICTIONARY *dict, size_t key_size __maybe_unused, size_t item_size, size_t value_size) {
    if(item_size)
        __atomic_fetch_add(&dict->stats->memory.dict, (long)item_size, __ATOMIC_RELAXED);

    if(value_size)
        __atomic_fetch_add(&dict->stats->memory.values, (long)value_size, __ATOMIC_RELAXED);
}

static inline void DICTIONARY_STATS_MINUS_MEMORY(DICTIONARY *dict, size_t key_size __maybe_unused, size_t item_size, size_t value_size) {
    if(item_size)
        __atomic_fetch_sub(&dict->stats->memory.dict, (long)item_size, __ATOMIC_RELAXED);

    if(value_size)
        __atomic_fetch_sub(&dict->stats->memory.values, (long)value_size, __ATOMIC_RELAXED);
}
#else
#define DICTIONARY_STATS_PLUS_MEMORY(dict, key_size, item_size, value_size) do {(void)item_size;} while(0)
#define DICTIONARY_STATS_MINUS_MEMORY(dict, key_size, item_size, value_size) do {;} while(0)
#endif

// ----------------------------------------------------------------------------
// internal statistics API

#ifdef DICT_WITH_STATS
static inline void DICTIONARY_STATS_SEARCHES_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.searches, 1, __ATOMIC_RELAXED);
}
#else
#define DICTIONARY_STATS_SEARCHES_PLUS1(dict) do {;} while(0)
#endif

static inline void DICTIONARY_ENTRIES_PLUS1(DICTIONARY *dict) {
#ifdef DICT_WITH_STATS
    // statistics
    __atomic_fetch_add(&dict->stats->items.entries, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&dict->stats->items.referenced, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&dict->stats->ops.inserts, 1, __ATOMIC_RELAXED);
#endif

    if(unlikely(is_dictionary_single_threaded(dict))) {
        dict->version++;
        dict->entries++;
        dict->referenced_items++;

    }
    else {
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_RELAXED);
        __atomic_fetch_add(&dict->entries, 1, __ATOMIC_RELAXED);
        __atomic_fetch_add(&dict->referenced_items, 1, __ATOMIC_RELAXED);
    }
}

static inline void DICTIONARY_ENTRIES_MINUS1(DICTIONARY *dict) {
#ifdef DICT_WITH_STATS
    // statistics
    __atomic_fetch_add(&dict->stats->ops.deletes, 1, __ATOMIC_RELAXED);
    __atomic_fetch_sub(&dict->stats->items.entries, 1, __ATOMIC_RELAXED);
#endif

    size_t entries; (void)entries;
    if(unlikely(is_dictionary_single_threaded(dict))) {
        dict->version++;
        entries = dict->entries--;
    }
    else {
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_RELAXED);
        entries = __atomic_fetch_sub(&dict->entries, 1, __ATOMIC_RELAXED);
    }

    internal_fatal(entries == 0,
                   "DICT: negative number of entries in dictionary created from %s() (%zu@%s)",
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);
}

static inline void DICTIONARY_VALUE_RESETS_PLUS1(DICTIONARY *dict) {
#ifdef DICT_WITH_STATS
    __atomic_fetch_add(&dict->stats->ops.resets, 1, __ATOMIC_RELAXED);
#endif

    if(unlikely(is_dictionary_single_threaded(dict)))
        dict->version++;
    else
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_RELAXED);
}

#ifdef DICT_WITH_STATS
static inline void DICTIONARY_STATS_TRAVERSALS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.traversals, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_WALKTHROUGHS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.walkthroughs, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_CHECK_SPINS_PLUS(DICTIONARY *dict, size_t count) {
    __atomic_fetch_add(&dict->stats->spin_locks.use_spins, count, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_INSERT_SPINS_PLUS(DICTIONARY *dict, size_t count) {
    __atomic_fetch_add(&dict->stats->spin_locks.insert_spins, count, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_DELETE_SPINS_PLUS(DICTIONARY *dict, size_t count) {
    __atomic_fetch_add(&dict->stats->spin_locks.delete_spins, count, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_SEARCH_IGNORES_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->spin_locks.search_spins, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_CALLBACK_INSERTS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->callbacks.inserts, 1, __ATOMIC_RELEASE);
}
static inline void DICTIONARY_STATS_CALLBACK_CONFLICTS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->callbacks.conflicts, 1, __ATOMIC_RELEASE);
}
static inline void DICTIONARY_STATS_CALLBACK_REACTS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->callbacks.reacts, 1, __ATOMIC_RELEASE);
}
static inline void DICTIONARY_STATS_CALLBACK_DELETES_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->callbacks.deletes, 1, __ATOMIC_RELEASE);
}
static inline void DICTIONARY_STATS_GARBAGE_COLLECTIONS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.garbage_collections, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_DICT_CREATIONS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->dictionaries.active, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&dict->stats->ops.creations, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_DICT_DESTRUCTIONS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_sub(&dict->stats->dictionaries.active, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&dict->stats->ops.destructions, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_DICT_DESTROY_QUEUED_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->dictionaries.deleted, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_DICT_DESTROY_QUEUED_MINUS1(DICTIONARY *dict) {
    __atomic_fetch_sub(&dict->stats->dictionaries.deleted, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_DICT_FLUSHES_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.flushes, 1, __ATOMIC_RELAXED);
}
#else
#define DICTIONARY_STATS_TRAVERSALS_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_CHECK_SPINS_PLUS(dict, count) do {;} while(0)
#define DICTIONARY_STATS_INSERT_SPINS_PLUS(dict, count) do {;} while(0)
#define DICTIONARY_STATS_DELETE_SPINS_PLUS(dict, count) do {;} while(0)
#define DICTIONARY_STATS_SEARCH_IGNORES_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_CALLBACK_INSERTS_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_CALLBACK_CONFLICTS_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_CALLBACK_REACTS_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_CALLBACK_DELETES_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_GARBAGE_COLLECTIONS_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_DICT_CREATIONS_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_DICT_DESTRUCTIONS_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_DICT_DESTROY_QUEUED_PLUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_DICT_DESTROY_QUEUED_MINUS1(dict) do {;} while(0)
#define DICTIONARY_STATS_DICT_FLUSHES_PLUS1(dict) do {;} while(0)
#endif

static inline void DICTIONARY_REFERENCED_ITEMS_PLUS1(DICTIONARY *dict) {
#ifdef DICT_WITH_STATS
    __atomic_fetch_add(&dict->stats->items.referenced, 1, __ATOMIC_RELAXED);
#endif

    if(unlikely(is_dictionary_single_threaded(dict)))
        ++dict->referenced_items;
    else
        __atomic_add_fetch(&dict->referenced_items, 1, __ATOMIC_RELAXED);
}

static inline void DICTIONARY_REFERENCED_ITEMS_MINUS1(DICTIONARY *dict) {
#ifdef DICT_WITH_STATS
    __atomic_fetch_sub(&dict->stats->items.referenced, 1, __ATOMIC_RELAXED);
#endif

    long int referenced_items; (void)referenced_items;
    if(unlikely(is_dictionary_single_threaded(dict)))
        referenced_items = --dict->referenced_items;
    else
        referenced_items = __atomic_sub_fetch(&dict->referenced_items, 1, __ATOMIC_SEQ_CST);

    internal_fatal(referenced_items < 0,
                   "DICT: negative number of referenced items (%ld) in dictionary created from %s() (%zu@%s)",
                   referenced_items,
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);
}

static inline void DICTIONARY_PENDING_DELETES_PLUS1(DICTIONARY *dict) {
#ifdef DICT_WITH_STATS
    __atomic_fetch_add(&dict->stats->items.pending_deletion, 1, __ATOMIC_RELAXED);
#endif

    if(unlikely(is_dictionary_single_threaded(dict)))
        ++dict->pending_deletion_items;
    else
        __atomic_add_fetch(&dict->pending_deletion_items, 1, __ATOMIC_RELEASE);
}

static inline long int DICTIONARY_PENDING_DELETES_MINUS1(DICTIONARY *dict) {
#ifdef DICT_WITH_STATS
    __atomic_fetch_sub(&dict->stats->items.pending_deletion, 1, __ATOMIC_RELEASE);
#endif

    if(unlikely(is_dictionary_single_threaded(dict)))
        return --dict->pending_deletion_items;
    else
        return __atomic_sub_fetch(&dict->pending_deletion_items, 1, __ATOMIC_ACQUIRE);
}

static inline long int DICTIONARY_PENDING_DELETES_GET(DICTIONARY *dict) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return dict->pending_deletion_items;
    else
        return __atomic_load_n(&dict->pending_deletion_items, __ATOMIC_SEQ_CST);
}

static inline REFCOUNT DICTIONARY_ITEM_REFCOUNT_GET(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    if(unlikely(dict && is_dictionary_single_threaded(dict))) // this is an exception, dict can be null
        return item->refcount;
    else
        return (REFCOUNT)__atomic_load_n(&item->refcount, __ATOMIC_ACQUIRE);
}

static inline REFCOUNT DICTIONARY_ITEM_REFCOUNT_GET_SOLE(DICTIONARY_ITEM *item) {
    return (REFCOUNT)__atomic_load_n(&item->refcount, __ATOMIC_ACQUIRE);
}


#endif //NETDATA_DICTIONARY_STATISTICS_H
