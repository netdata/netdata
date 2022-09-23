// SPDX-License-Identifier: GPL-3.0-or-later

#define DICTIONARY_INTERNALS

#include "../libnetdata.h"
#include <Judy.h>

// runtime flags of the dictionary - must be checked with atomics
typedef enum {
    DICT_FLAG_NONE                  = 0,
    DICT_FLAG_DESTROYED             = (1 << 0), // this dictionary has been destroyed
} DICT_FLAGS;

#define dict_flag_check(dict, flag) (__atomic_load_n(&((dict)->flags), __ATOMIC_SEQ_CST) & (flag))
#define dict_flag_set(dict, flag)   __atomic_or_fetch(&((dict)->flags), flag, __ATOMIC_SEQ_CST)
#define dict_flag_clear(dict, flag) __atomic_and_fetch(&((dict)->flags), ~(flag), __ATOMIC_SEQ_CST)

// flags macros
#define is_dictionary_destroyed(dict) dict_flag_check(dict, DICT_FLAG_DESTROYED)

// configuration options macros
#define is_dictionary_single_threaded(dict) ((dict)->options & DICT_OPTION_SINGLE_THREADED)
#define is_view_dictionary(dict) ((dict)->master)
#define is_master_dictionary(dict) (!is_view_dictionary(dict))

typedef enum item_options {
    ITEM_OPTION_NONE            = 0,
    ITEM_OPTION_ALLOCATED_NAME  = (1 << 0), // the name pointer is a STRING

    // IMPORTANT: This is 1-bit - to add more change ITEM_OPTIONS_BITS
} ITEM_OPTIONS;

typedef enum item_flags {
    ITEM_FLAG_NONE              = 0,
    ITEM_FLAG_DELETED           = (1 << 0), // this item is deleted, so it is not available for traversal

    // IMPORTANT: This is 8-bit
} ITEM_FLAGS;

#define item_flag_check(item, flag) (__atomic_load_n(&((item)->flags), __ATOMIC_SEQ_CST) & (flag))
#define item_flag_set(item, flag)   __atomic_or_fetch(&((item)->flags), flag, __ATOMIC_SEQ_CST)
#define item_flag_clear(item, flag) __atomic_and_fetch(&((item)->flags), ~(flag), __ATOMIC_SEQ_CST)

#define item_shared_flag_check(item, flag) (__atomic_load_n(&((item)->shared->flags), __ATOMIC_SEQ_CST) & (flag))
#define item_shared_flag_set(item, flag)   __atomic_or_fetch(&((item)->shared->flags), flag, __ATOMIC_SEQ_CST)
#define item_shared_flag_clear(item, flag) __atomic_and_fetch(&((item)->shared->flags), ~(flag), __ATOMIC_SEQ_CST)

#define REFCOUNT_DELETING         (-100)

#define ITEM_FLAGS_TYPE uint8_t
#define KEY_LEN_TYPE    uint32_t
#define VALUE_LEN_TYPE  uint32_t

#define ITEM_OPTIONS_BITS 1
#define KEY_LEN_BITS ((sizeof(KEY_LEN_TYPE) * 8) - (sizeof(ITEM_FLAGS_TYPE) * 8) - ITEM_OPTIONS_BITS)
#define KEY_LEN_MAX ((1 << KEY_LEN_BITS) - 1)

#define VALUE_LEN_BITS ((sizeof(VALUE_LEN_TYPE) * 8) - (sizeof(ITEM_FLAGS_TYPE) * 8))
#define VALUE_LEN_MAX ((1 << VALUE_LEN_BITS) - 1)


/*
 * Every item in the dictionary has the following structure.
 */

typedef int32_t REFCOUNT;

typedef struct dictionary_item_shared {
    void *value;                            // the value of the dictionary item

    // the order of the following items is important!
    // The total of their storage should be 64-bits

    REFCOUNT links;                         // how many links this item has
    VALUE_LEN_TYPE value_len:VALUE_LEN_BITS; // the size of the value
    ITEM_FLAGS_TYPE flags;                  // shared flags
} DICTIONARY_ITEM_SHARED;

struct dictionary_item {
#ifdef NETDATA_INTERNAL_CHECKS
    DICTIONARY *dict;
#endif

    DICTIONARY_ITEM_SHARED *shared;

    struct dictionary_item *next;           // a double linked list to allow fast insertions and deletions
    struct dictionary_item *prev;

    union {
        STRING *string_name;                // the name of the dictionary item
        char *caller_name;                  // the user supplied string pointer
//        void *key_ptr;                      // binary key pointer
    };

    // the order of the following items is important!
    // The total of their storage should be 64-bits

    REFCOUNT refcount;                      // the private reference counter

    KEY_LEN_TYPE key_len:KEY_LEN_BITS;      // the size of key indexed (for strings, including the null terminator)
                                            // this is (2^23 - 1) = 8.388.607 bytes max key length.

    ITEM_OPTIONS options:ITEM_OPTIONS_BITS; // permanent configuration options
                                            // (no atomic operations on this - they never change)

    ITEM_FLAGS_TYPE flags;                  // runtime changing flags for this item (atomic operations on this)
                                            // cannot be a bit field because of atomics.
};

struct dictionary_hooks {
    REFCOUNT links;
    usec_t last_master_deletion_us;

    void (*ins_callback)(const DICTIONARY_ITEM *item, void *value, void *data);
    void *ins_callback_data;

    bool (*conflict_callback)(const DICTIONARY_ITEM *item, void *old_value, void *new_value, void *data);
    void *conflict_callback_data;

    void (*react_callback)(const DICTIONARY_ITEM *item, void *value, void *data);
    void *react_callback_data;

    void (*del_callback)(const DICTIONARY_ITEM *item, void *value, void *data);
    void *del_callback_data;
};

struct dictionary_stats dictionary_stats_category_other = {
    .name = "other",
};

struct dictionary {
#ifdef NETDATA_INTERNAL_CHECKS
    const char *creation_function;
    const char *creation_file;
    size_t creation_line;
#endif

    usec_t last_gc_run_us;
    DICT_OPTIONS options;               // the configuration flags of the dictionary (they never change - no atomics)
    DICT_FLAGS flags;                   // run time flags for the dictionary (they change all the time - atomics needed)

    struct {                            // support for multiple indexing engines
        Pvoid_t JudyHSArray;            // the hash table
        netdata_rwlock_t rwlock;        // protect the index
    } index;

    struct {
        DICTIONARY_ITEM *list;          // the double linked list of all items in the dictionary
        netdata_rwlock_t rwlock;        // protect the linked-list
        pid_t writer_pid;               // the gettid() of the writer
        size_t writer_depth;            // nesting of write locks
    } items;

    struct dictionary_hooks *hooks;     // pointer to external function callbacks to be called at certain points
    struct dictionary_stats *stats;     // statistics data, when DICT_OPTION_STATS is set

    DICTIONARY *master;                 // the master dictionary
    DICTIONARY *next;                   // linked list for delayed destruction (garbage collection of whole dictionaries)

    size_t version;                     // the current version of the dictionary
                                        // it is incremented when:
                                        //   - item added
                                        //   - item removed
                                        //   - item value reset
                                        //   - conflict callback returns true
                                        //   - function dictionary_version_increment() is called

    long int entries;                   // how many items are currently in the index (the linked list may have more)
    long int referenced_items;          // how many items of the dictionary are currently being used by 3rd parties
    long int pending_deletion_items;    // how many items of the dictionary have been deleted, but have not been removed yet
};

// forward definitions of functions used in reverse order in the code
static void garbage_collect_pending_deletes(DICTIONARY *dict);
static inline void item_linked_list_remove(DICTIONARY *dict, DICTIONARY_ITEM *item);
static size_t item_free_with_hooks(DICTIONARY *dict, DICTIONARY_ITEM *item);
static inline const char *item_get_name(const DICTIONARY_ITEM *item);
static bool item_is_not_referenced_and_can_be_removed(DICTIONARY *dict, DICTIONARY_ITEM *item);
static inline int hashtable_delete_unsafe(DICTIONARY *dict, const char *name, size_t name_len, void *item);
static void item_release(DICTIONARY *dict, DICTIONARY_ITEM *item);

#define ITEM_OK 0
#define ITEM_MARKED_FOR_DELETION (-1)  // the item is marked for deletion
#define ITEM_IS_CURRENTLY_BEING_DELETED (-2) // the item is currently being deleted
#define item_check_and_acquire(dict, item) (item_check_and_acquire_advanced(dict, item, false) == ITEM_OK)
static int item_check_and_acquire_advanced(DICTIONARY *dict, DICTIONARY_ITEM *item, bool having_index_lock);

// ----------------------------------------------------------------------------
// memory statistics

static inline void DICTIONARY_STATS_PLUS_MEMORY(DICTIONARY *dict, size_t key_size, size_t item_size, size_t value_size) {
    if(key_size)
        __atomic_fetch_add(&dict->stats->memory.indexed, (long)key_size, __ATOMIC_RELAXED);

    if(item_size)
        __atomic_fetch_add(&dict->stats->memory.dict, (long)item_size, __ATOMIC_RELAXED);

    if(value_size)
        __atomic_fetch_add(&dict->stats->memory.values, (long)value_size, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_MINUS_MEMORY(DICTIONARY *dict, size_t key_size, size_t item_size, size_t value_size) {
    if(key_size)
        __atomic_fetch_sub(&dict->stats->memory.indexed, (long)key_size, __ATOMIC_RELAXED);

    if(item_size)
        __atomic_fetch_sub(&dict->stats->memory.dict, (long)item_size, __ATOMIC_RELAXED);

    if(value_size)
        __atomic_fetch_sub(&dict->stats->memory.values, (long)value_size, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// callbacks registration

static inline void dictionary_hooks_allocate(DICTIONARY *dict) {
    if(dict->hooks) return;

    dict->hooks = callocz(1, sizeof(struct dictionary_hooks));
    dict->hooks->links = 1;

    DICTIONARY_STATS_PLUS_MEMORY(dict, 0, sizeof(struct dictionary_hooks), 0);
}

static inline size_t dictionary_hooks_free(DICTIONARY *dict) {
    if(!dict->hooks) return 0;

    REFCOUNT links = __atomic_sub_fetch(&dict->hooks->links, 1, __ATOMIC_SEQ_CST);
    if(links == 0) {
        freez(dict->hooks);
        dict->hooks = NULL;

        DICTIONARY_STATS_MINUS_MEMORY(dict, 0, sizeof(struct dictionary_hooks), 0);
        return sizeof(struct dictionary_hooks);
    }

    return 0;
}

void dictionary_register_insert_callback(DICTIONARY *dict, void (*ins_callback)(const DICTIONARY_ITEM *item, void *value, void *data), void *data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    dictionary_hooks_allocate(dict);
    dict->hooks->ins_callback = ins_callback;
    dict->hooks->ins_callback_data = data;
}

void dictionary_register_conflict_callback(DICTIONARY *dict, bool (*conflict_callback)(const DICTIONARY_ITEM *item, void *old_value, void *new_value, void *data), void *data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    dictionary_hooks_allocate(dict);
    dict->hooks->conflict_callback = conflict_callback;
    dict->hooks->conflict_callback_data = data;
}

void dictionary_register_react_callback(DICTIONARY *dict, void (*react_callback)(const DICTIONARY_ITEM *item, void *value, void *data), void *data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    dictionary_hooks_allocate(dict);
    dict->hooks->react_callback = react_callback;
    dict->hooks->react_callback_data = data;
}

void dictionary_register_delete_callback(DICTIONARY *dict, void (*del_callback)(const DICTIONARY_ITEM *item, void *value, void *data), void *data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    dictionary_hooks_allocate(dict);
    dict->hooks->del_callback = del_callback;
    dict->hooks->del_callback_data = data;
}

// ----------------------------------------------------------------------------
// dictionary statistics API

size_t dictionary_version(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;

    // this is required for views to return the right number
    garbage_collect_pending_deletes(dict);

    return __atomic_load_n(&dict->version, __ATOMIC_SEQ_CST);
}
size_t dictionary_entries(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;

    // this is required for views to return the right number
    garbage_collect_pending_deletes(dict);

    long int entries = __atomic_load_n(&dict->entries, __ATOMIC_SEQ_CST);
    if(entries < 0)
        fatal("DICTIONARY: entries is negative: %ld", entries);

    return entries;
}
size_t dictionary_referenced_items(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;

    long int referenced_items = __atomic_load_n(&dict->referenced_items, __ATOMIC_SEQ_CST);
    if(referenced_items < 0)
        fatal("DICTIONARY: referenced items is negative: %ld", referenced_items);

    return referenced_items;
}

long int dictionary_stats_for_registry(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;
    return (dict->stats->memory.indexed + dict->stats->memory.dict);
}
void dictionary_version_increment(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->version, 1, __ATOMIC_SEQ_CST);
}

// ----------------------------------------------------------------------------
// internal statistics API

static inline void DICTIONARY_STATS_SEARCHES_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.searches, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_ENTRIES_PLUS1(DICTIONARY *dict) {
    // statistics
    __atomic_fetch_add(&dict->stats->items.entries, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&dict->stats->items.referenced, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&dict->stats->ops.inserts, 1, __ATOMIC_RELAXED);

    if(unlikely(is_dictionary_single_threaded(dict))) {
        dict->version++;
        dict->entries++;
        dict->referenced_items++;

    }
    else {
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_SEQ_CST);
        __atomic_fetch_add(&dict->entries, 1, __ATOMIC_SEQ_CST);
        __atomic_fetch_add(&dict->referenced_items, 1, __ATOMIC_SEQ_CST);
    }
}
static inline void DICTIONARY_ENTRIES_MINUS1(DICTIONARY *dict) {
    // statistics
    __atomic_fetch_add(&dict->stats->ops.deletes, 1, __ATOMIC_RELAXED);
    __atomic_fetch_sub(&dict->stats->items.entries, 1, __ATOMIC_RELAXED);

    if(unlikely(is_dictionary_single_threaded(dict))) {
        dict->version++;
        dict->entries--;
    }
    else {
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_SEQ_CST);
        __atomic_fetch_sub(&dict->entries, 1, __ATOMIC_SEQ_CST);
    }
}
static inline void DICTIONARY_VALUE_RESETS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.resets, 1, __ATOMIC_RELAXED);

    if(unlikely(is_dictionary_single_threaded(dict)))
        dict->version++;
    else
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_SEQ_CST);
}
static inline void DICTIONARY_STATS_TRAVERSALS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.traversals, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_WALKTHROUGHS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->ops.walkthroughs, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_CHECK_SPINS_PLUS(DICTIONARY *dict, size_t count) {
    __atomic_fetch_add(&dict->stats->spin_locks.use, count, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_INSERT_SPINS_PLUS(DICTIONARY *dict, size_t count) {
    __atomic_fetch_add(&dict->stats->spin_locks.insert, count, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_SEARCH_IGNORES_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->spin_locks.search, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_CALLBACK_INSERTS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->callbacks.inserts, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_CALLBACK_CONFLICTS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->callbacks.conflicts, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_CALLBACK_REACTS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->callbacks.reacts, 1, __ATOMIC_RELAXED);
}
static inline void DICTIONARY_STATS_CALLBACK_DELETES_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->callbacks.deletes, 1, __ATOMIC_RELAXED);
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

static inline long int DICTIONARY_REFERENCED_ITEMS_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->items.referenced, 1, __ATOMIC_RELAXED);

    if(unlikely(is_dictionary_single_threaded(dict)))
        return ++dict->referenced_items;
    else
        return __atomic_add_fetch(&dict->referenced_items, 1, __ATOMIC_SEQ_CST);
}

static inline long int DICTIONARY_REFERENCED_ITEMS_MINUS1(DICTIONARY *dict) {
    __atomic_fetch_sub(&dict->stats->items.referenced, 1, __ATOMIC_RELAXED);

    if(unlikely(is_dictionary_single_threaded(dict)))
        return --dict->referenced_items;
    else
        return __atomic_sub_fetch(&dict->referenced_items, 1, __ATOMIC_SEQ_CST);
}

static inline long int DICTIONARY_PENDING_DELETES_PLUS1(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->stats->items.pending_deletion, 1, __ATOMIC_RELAXED);

    if(unlikely(is_dictionary_single_threaded(dict)))
        return ++dict->pending_deletion_items;
    else
        return __atomic_add_fetch(&dict->pending_deletion_items, 1, __ATOMIC_SEQ_CST);
}

static inline long int DICTIONARY_PENDING_DELETES_MINUS1(DICTIONARY *dict) {
    __atomic_fetch_sub(&dict->stats->items.pending_deletion, 1, __ATOMIC_RELAXED);

    if(unlikely(is_dictionary_single_threaded(dict)))
        return --dict->pending_deletion_items;
    else
        return __atomic_sub_fetch(&dict->pending_deletion_items, 1, __ATOMIC_SEQ_CST);
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
        return (REFCOUNT)__atomic_load_n(&item->refcount, __ATOMIC_SEQ_CST);
}

// ----------------------------------------------------------------------------
// callbacks execution

static void dictionary_execute_insert_callback(DICTIONARY *dict, DICTIONARY_ITEM *item, void *constructor_data) {
    if(likely(!dict->hooks || !dict->hooks->ins_callback))
        return;

    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    internal_error(false,
                   "DICTIONARY: Running insert callback on item '%s' of dictionary created from %s() %zu@%s.",
                   item_get_name(item),
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);

    DICTIONARY_STATS_CALLBACK_INSERTS_PLUS1(dict);
    dict->hooks->ins_callback(item, item->shared->value, constructor_data?constructor_data:dict->hooks->ins_callback_data);
}

static bool dictionary_execute_conflict_callback(DICTIONARY *dict, DICTIONARY_ITEM *item, void *new_value, void *constructor_data) {
    if(likely(!dict->hooks || !dict->hooks->conflict_callback))
        return false;

    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    internal_error(false,
                   "DICTIONARY: Running conflict callback on item '%s' of dictionary created from %s() %zu@%s.",
                   item_get_name(item),
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);

    DICTIONARY_STATS_CALLBACK_CONFLICTS_PLUS1(dict);
    return dict->hooks->conflict_callback(
        item, item->shared->value, new_value,
        constructor_data ? constructor_data : dict->hooks->conflict_callback_data);
}

static void dictionary_execute_react_callback(DICTIONARY *dict, DICTIONARY_ITEM *item, void *constructor_data) {
    if(likely(!dict->hooks || !dict->hooks->react_callback))
        return;

    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    internal_error(false,
                   "DICTIONARY: Running react callback on item '%s' of dictionary created from %s() %zu@%s.",
                   item_get_name(item),
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);

    DICTIONARY_STATS_CALLBACK_REACTS_PLUS1(dict);
    dict->hooks->react_callback(item, item->shared->value,
                                constructor_data?constructor_data:dict->hooks->react_callback_data);
}

static void dictionary_execute_delete_callback(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    if(likely(!dict->hooks || !dict->hooks->del_callback))
        return;

    // We may execute the delete callback on items deleted from a view,
    // because we may have references to it, after the master is gone
    // so, the shared structure will remain until the last reference is released.

    internal_error(false,
                   "DICTIONARY: Running delete callback on item '%s' of dictionary created from %s() %zu@%s.",
                   item_get_name(item),
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);

    DICTIONARY_STATS_CALLBACK_DELETES_PLUS1(dict);
    dict->hooks->del_callback(item, item->shared->value, dict->hooks->del_callback_data);
}

// ----------------------------------------------------------------------------
// dictionary locks

static inline size_t dictionary_locks_init(DICTIONARY *dict) {
    if(likely(!is_dictionary_single_threaded(dict))) {
        netdata_rwlock_init(&dict->index.rwlock);
        netdata_rwlock_init(&dict->items.rwlock);
        return 0;
    }
    return 0;
}

static inline size_t dictionary_locks_destroy(DICTIONARY *dict) {
    if(likely(!is_dictionary_single_threaded(dict))) {
        netdata_rwlock_destroy(&dict->index.rwlock);
        netdata_rwlock_destroy(&dict->items.rwlock);
        return 0;
    }
    return 0;
}

static inline void ll_recursive_lock_set_thread_as_writer(DICTIONARY *dict) {
    pid_t expected = 0, desired = gettid();
    if(!__atomic_compare_exchange_n(&dict->items.writer_pid, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST))
        fatal("DICTIONARY: Cannot set thread %d as exclusive writer, expected %d, desired %d, found %d.", gettid(), expected, desired, __atomic_load_n(&dict->items.writer_pid, __ATOMIC_SEQ_CST));
}

static inline void ll_recursive_unlock_unset_thread_writer(DICTIONARY *dict) {
    pid_t expected = gettid(), desired = 0;
    if(!__atomic_compare_exchange_n(&dict->items.writer_pid, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST))
        fatal("DICTIONARY: Cannot unset thread %d as exclusive writer, expected %d, desired %d, found %d.", gettid(), expected, desired, __atomic_load_n(&dict->items.writer_pid, __ATOMIC_SEQ_CST));
}

static inline bool ll_recursive_lock_is_thread_the_writer(DICTIONARY *dict) {
    pid_t tid = gettid();
    return tid > 0 && tid == __atomic_load_n(&dict->items.writer_pid, __ATOMIC_SEQ_CST);
}

static void ll_recursive_lock(DICTIONARY *dict, char rw) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    if(ll_recursive_lock_is_thread_the_writer(dict)) {
        dict->items.writer_depth++;
        return;
    }

    if(rw == DICTIONARY_LOCK_READ || rw == DICTIONARY_LOCK_REENTRANT || rw == 'R') {
        // read lock
        netdata_rwlock_rdlock(&dict->items.rwlock);
    }
    else {
        // write lock
        netdata_rwlock_wrlock(&dict->items.rwlock);
        ll_recursive_lock_set_thread_as_writer(dict);
    }
}

static void ll_recursive_unlock(DICTIONARY *dict, char rw) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    if(ll_recursive_lock_is_thread_the_writer(dict) && dict->items.writer_depth > 0) {
        dict->items.writer_depth--;
        return;
    }

    if(rw == DICTIONARY_LOCK_READ || rw == DICTIONARY_LOCK_REENTRANT || rw == 'R') {
        // read unlock

        netdata_rwlock_unlock(&dict->items.rwlock);
    }
    else {
        // write unlock

        ll_recursive_unlock_unset_thread_writer(dict);

        netdata_rwlock_unlock(&dict->items.rwlock);
    }
}


static inline void dictionary_index_lock_rdlock(DICTIONARY *dict) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    netdata_rwlock_rdlock(&dict->index.rwlock);
}
static inline void dictionary_index_lock_wrlock(DICTIONARY *dict) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    netdata_rwlock_wrlock(&dict->index.rwlock);
}
static inline void dictionary_index_lock_unlock(DICTIONARY *dict) {
    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    netdata_rwlock_unlock(&dict->index.rwlock);
}

// ----------------------------------------------------------------------------
// items garbage collector

static void garbage_collect_pending_deletes(DICTIONARY *dict) {
    usec_t last_master_deletion_us = dict->hooks?__atomic_load_n(&dict->hooks->last_master_deletion_us, __ATOMIC_SEQ_CST):0;
    usec_t last_gc_run_us = __atomic_load_n(&dict->last_gc_run_us, __ATOMIC_SEQ_CST);

    bool is_view = is_view_dictionary(dict);

    if(likely(!(
            DICTIONARY_PENDING_DELETES_GET(dict) > 0 ||
            (is_view && last_master_deletion_us > last_gc_run_us)
            )))
        return;

    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);

    __atomic_store_n(&dict->last_gc_run_us, now_realtime_usec(), __ATOMIC_SEQ_CST);

    if(is_view)
        dictionary_index_lock_wrlock(dict);

    DICTIONARY_STATS_GARBAGE_COLLECTIONS_PLUS1(dict);

    size_t deleted = 0, pending = 0, examined = 0;
    DICTIONARY_ITEM *item = dict->items.list, *item_next;
    while(item) {
        examined++;

        // this will cleanup
        item_next = item->next;
        int rc = item_check_and_acquire_advanced(dict, item, is_view);

        if(rc == ITEM_MARKED_FOR_DELETION) {
            // we don't have got a reference

            if(item_is_not_referenced_and_can_be_removed(dict, item)) {
                DOUBLE_LINKED_LIST_REMOVE_UNSAFE(dict->items.list, item, prev, next);
                item_free_with_hooks(dict, item);
                deleted++;

                pending = DICTIONARY_PENDING_DELETES_MINUS1(dict);
                if (!pending)
                    break;
            }
        }
        else if(rc == ITEM_IS_CURRENTLY_BEING_DELETED)
            ; // do not touch this item (we haven't got a reference)

        else if(rc == ITEM_OK)
            item_release(dict, item);

        item = item_next;
    }

    if(is_view)
        dictionary_index_lock_unlock(dict);

    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);

    (void)deleted;
    (void)examined;

    internal_error(false, "DICTIONARY: garbage collected dictionary created by %s (%zu@%s), examined %zu items, deleted %zu items, still pending %zu items",
                   dict->creation_function, dict->creation_line, dict->creation_file, examined, deleted, pending);

}

// ----------------------------------------------------------------------------
// reference counters

static inline size_t reference_counter_init(DICTIONARY *dict) {
    (void)dict;

    // allocate memory required for reference counters
    // return number of bytes
    return 0;
}

static inline size_t reference_counter_free(DICTIONARY *dict) {
    (void)dict;

    // free memory required for reference counters
    // return number of bytes
    return 0;
}

static void item_acquire(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    REFCOUNT refcount;

    if(unlikely(is_dictionary_single_threaded(dict))) {
        refcount = ++item->refcount;
    }
    else {
        // increment the refcount
        refcount = __atomic_add_fetch(&item->refcount, 1, __ATOMIC_SEQ_CST);
    }

    if(refcount <= 0) {
        internal_error(
            true,
            "DICTIONARY: attempted to acquire item which is deleted (refcount = %d): "
            "'%s' on dictionary created by %s() (%zu@%s)",
            refcount - 1,
            item_get_name(item),
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);

        fatal(
            "DICTIONARY: request to acquire item '%s', which is deleted (refcount = %d)!",
            item_get_name(item),
            refcount - 1);
    }

    if(refcount == 1) {
        // referenced items counts number of unique items referenced
        // so, we increase it only when refcount == 1
        DICTIONARY_REFERENCED_ITEMS_PLUS1(dict);

        // if this is a deleted item, but the counter increased to 1
        // we need to remove it from the pending items to delete
        if(item_flag_check(item, ITEM_FLAG_DELETED))
            DICTIONARY_PENDING_DELETES_MINUS1(dict);
    }
}

static void item_release(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    // this function may be called without any lock on the dictionary
    // or even when someone else has write lock on the dictionary

    bool is_deleted;
    REFCOUNT refcount;

    if(unlikely(is_dictionary_single_threaded(dict))) {
        is_deleted = item->flags & ITEM_FLAG_DELETED;
        refcount = --item->refcount;
    }
    else {
        // get the flags before decrementing any reference counters
        // (the other way around may lead to use-after-free)
        is_deleted = item_flag_check(item, ITEM_FLAG_DELETED);

        // decrement the refcount
        refcount = __atomic_sub_fetch(&item->refcount, 1, __ATOMIC_SEQ_CST);
    }

    if(refcount < 0) {
        internal_error(
            true,
            "DICTIONARY: attempted to release item without references (refcount = %d): "
            "'%s' on dictionary created by %s() (%zu@%s)",
            refcount + 1,
            item_get_name(item),
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);

        fatal(
            "DICTIONARY: attempted to release item '%s' without references (refcount = %d)",
            item_get_name(item),
            refcount + 1);
    }

    if(refcount == 0) {

        if(is_deleted)
            DICTIONARY_PENDING_DELETES_PLUS1(dict);

        // referenced items counts number of unique items referenced
        // so, we decrease it only when refcount == 0
        DICTIONARY_REFERENCED_ITEMS_MINUS1(dict);
    }
}

static int item_check_and_acquire_advanced(DICTIONARY *dict, DICTIONARY_ITEM *item, bool having_index_lock) {
    size_t spins = 0;
    REFCOUNT refcount, desired;

    do {
        spins++;

        refcount = DICTIONARY_ITEM_REFCOUNT_GET(dict, item);

        if(refcount < 0) {
            // we can't use this item
            return ITEM_IS_CURRENTLY_BEING_DELETED;
        }

        if(item_flag_check(item, ITEM_FLAG_DELETED)) {
            // we can't use this item
            return ITEM_MARKED_FOR_DELETION;
        }

        desired = refcount + 1;

    } while(!__atomic_compare_exchange_n(&item->refcount, &refcount, desired,
                                          false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST));

    // we acquired the item

    if(is_view_dictionary(dict) && item_shared_flag_check(item, ITEM_FLAG_DELETED) && !item_flag_check(item, ITEM_FLAG_DELETED)) {
        // but, we can't use this item

        if(having_index_lock) {
            // delete it from the hashtable
            hashtable_delete_unsafe(dict, item_get_name(item), item->key_len, item);

            // mark it in our dictionary as deleted too
            // this is safe to be done here, because we have got
            // a reference counter on item
            item_flag_set(item, ITEM_FLAG_DELETED);

            DICTIONARY_ENTRIES_MINUS1(dict);

            // decrement the refcount we incremented above
            if (__atomic_sub_fetch(&item->refcount, 1, __ATOMIC_SEQ_CST) == 0) {
                // this is a deleted item, and we are the last one
                DICTIONARY_PENDING_DELETES_PLUS1(dict);
            }

            // do not touch the item below this point
        }
        else {
            // this is traversal / walkthrough
            // decrement the refcount we incremented above
            __atomic_sub_fetch(&item->refcount, 1, __ATOMIC_SEQ_CST);
        }

        return ITEM_MARKED_FOR_DELETION;
    }

    if(desired == 1)
        DICTIONARY_REFERENCED_ITEMS_PLUS1(dict);

    if(unlikely(spins > 2 && dict->stats))
        DICTIONARY_STATS_CHECK_SPINS_PLUS(dict, spins - 2);

    return ITEM_OK; // we can use this item
}

// if a dictionary item can be deleted, return true, otherwise return false
// we use the private reference counter
static inline bool item_is_not_referenced_and_can_be_removed(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    // if we can set refcount to REFCOUNT_DELETING, we can delete this item

    REFCOUNT expected = DICTIONARY_ITEM_REFCOUNT_GET(dict, item);
    if(expected == 0 && __atomic_compare_exchange_n(&item->refcount, &expected, REFCOUNT_DELETING,
                                                     false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST)) {

        // we are going to delete it
        return true;
    }

    // we can't delete this
    return false;
}

// if a dictionary item can be freed, return true, otherwise return false
// we use the shared reference counter
static inline bool item_shared_release_and_check_if_it_can_be_freed(DICTIONARY *dict __maybe_unused, DICTIONARY_ITEM *item) {
    // if we can set refcount to REFCOUNT_DELETING, we can delete this item

    REFCOUNT links = __atomic_sub_fetch(&item->shared->links, 1, __ATOMIC_SEQ_CST);
    if(links == 0 && __atomic_compare_exchange_n(&item->shared->links, &links, REFCOUNT_DELETING,
                                                     false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST)) {

        // we can delete it
        return true;
    }

    // we can't delete it
    return false;
}


// ----------------------------------------------------------------------------
// hash table operations

static size_t hashtable_init_unsafe(DICTIONARY *dict) {
    dict->index.JudyHSArray = NULL;
    return 0;
}

static size_t hashtable_destroy_unsafe(DICTIONARY *dict) {
    if(unlikely(!dict->index.JudyHSArray)) return 0;

    JError_t J_Error;
    Word_t ret = JudyHSFreeArray(&dict->index.JudyHSArray, &J_Error);
    if(unlikely(ret == (Word_t) JERR)) {
        error("DICTIONARY: Cannot destroy JudyHS, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    debug(D_DICTIONARY, "Dictionary: hash table freed %lu bytes", ret);

    dict->index.JudyHSArray = NULL;
    return (size_t)ret;
}

static inline void **hashtable_insert_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
    JError_t J_Error;
    Pvoid_t *Rc = JudyHSIns(&dict->index.JudyHSArray, (void *)name, name_len, &J_Error);
    if (unlikely(Rc == PJERR)) {
        fatal("DICTIONARY: Cannot insert entry with name '%s' to JudyHS, JU_ERRNO_* == %u, ID == %d",
              name, JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    // if *Rc == 0, new item added to the array
    // otherwise the existing item value is returned in *Rc

    // we return a pointer to a pointer, so that the caller can
    // put anything needed at the value of the index.
    // The pointer to pointer we return has to be used before
    // any other operation that may change the index (insert/delete).
    return Rc;
}

static inline int hashtable_delete_unsafe(DICTIONARY *dict, const char *name, size_t name_len, void *item) {
    (void)item;
    if(unlikely(!dict->index.JudyHSArray)) return 0;

    JError_t J_Error;
    int ret = JudyHSDel(&dict->index.JudyHSArray, (void *)name, name_len, &J_Error);
    if(unlikely(ret == JERR)) {
        error("DICTIONARY: Cannot delete entry with name '%s' from JudyHS, JU_ERRNO_* == %u, ID == %d", name,
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
        return 0;
    }

    // Hey, this is problematic! We need the value back, not just an int with a status!
    // https://sourceforge.net/p/judy/feature-requests/23/

    if(unlikely(ret == 0)) {
        // not found in the dictionary
        return 0;
    }
    else {
        // found and deleted from the dictionary
        return 1;
    }
}

static inline DICTIONARY_ITEM *hashtable_get_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
    if(unlikely(!dict->index.JudyHSArray)) return NULL;

    DICTIONARY_STATS_SEARCHES_PLUS1(dict);

    Pvoid_t *Rc;
    Rc = JudyHSGet(dict->index.JudyHSArray, (void *)name, name_len);
    if(likely(Rc)) {
        // found in the hash table
        return (DICTIONARY_ITEM *)*Rc;
    }
    else {
        // not found in the hash table
        return NULL;
    }
}

static inline void hashtable_inserted_item_unsafe(DICTIONARY *dict, void *item) {
    (void)dict;
    (void)item;

    // this is called just after an item is successfully inserted to the hashtable
    // we don't need this for judy, but we may need it if we integrate more hash tables

    ;
}

// ----------------------------------------------------------------------------
// linked list management

static inline void item_linked_list_add(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);

    if(dict->options & DICT_OPTION_ADD_IN_FRONT)
        DOUBLE_LINKED_LIST_PREPEND_UNSAFE(dict->items.list, item, prev, next);
    else
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(dict->items.list, item, prev, next);

    garbage_collect_pending_deletes(dict);
    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);
}

static inline void item_linked_list_remove(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);

    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(dict->items.list, item, prev, next);

    garbage_collect_pending_deletes(dict);
    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);
}

// ----------------------------------------------------------------------------
// ITEM initialization and updates

static inline size_t item_set_name(DICTIONARY *dict, DICTIONARY_ITEM *item, const char *name, size_t name_len) {
    if(likely(dict->options & DICT_OPTION_NAME_LINK_DONT_CLONE)) {
        item->caller_name = (char *)name;
        item->key_len = name_len;
    }
    else {
        item->string_name = string_strdupz(name);
        item->key_len = string_strlen(item->string_name) + 1;
        item->options |= ITEM_OPTION_ALLOCATED_NAME;
    }

    return item->key_len;
}

static inline size_t item_free_name(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    if(likely(!(dict->options & DICT_OPTION_NAME_LINK_DONT_CLONE)))
        string_freez(item->string_name);

    return item->key_len;
}

static inline const char *item_get_name(const DICTIONARY_ITEM *item) {
    if(item->options & ITEM_OPTION_ALLOCATED_NAME)
        return string2str(item->string_name);
    else
        return item->caller_name;
}

static DICTIONARY_ITEM *item_allocate(DICTIONARY *dict __maybe_unused, size_t *allocated_bytes, DICTIONARY_ITEM *master_item) {
    DICTIONARY_ITEM *item;

    size_t size = sizeof(DICTIONARY_ITEM);
    item = callocz(1, size);
    item->refcount = 1;
    *allocated_bytes += size;

    if(master_item) {
        item->shared = master_item->shared;

        if(unlikely(__atomic_add_fetch(&item->shared->links, 1, __ATOMIC_SEQ_CST) <= 1))
            fatal("DICTIONARY: attempted to link to a shared item structure that had zero references");
    }
    else {
        size = sizeof(DICTIONARY_ITEM_SHARED);
        item->shared = callocz(1, size);
        item->shared->links = 1;
        *allocated_bytes += size;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    item->dict = dict;
#endif
    return item;
}

static DICTIONARY_ITEM *item_create_with_hooks(DICTIONARY *dict, const char *name, size_t name_len, void *value, size_t value_len, void *constructor_data, DICTIONARY_ITEM *master_item) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(name_len > KEY_LEN_MAX))
        fatal("DICTIONARY: tried to index a key of size %zu, but the maximum acceptable is %zu", name_len, (size_t)KEY_LEN_MAX);

    if(unlikely(value_len > VALUE_LEN_MAX))
        fatal("DICTIONARY: tried to add an item of size %zu, but the maximum acceptable is %zu", value_len, (size_t)VALUE_LEN_MAX);
#endif

    size_t item_size = 0, key_size = 0, value_size = 0;

    DICTIONARY_ITEM *item = item_allocate(dict, &item_size, master_item);
    key_size += item_set_name(dict, item, name, name_len);

    if(unlikely(is_view_dictionary(dict))) {
        // we are on a view dictionary
        // do not touch the value
        ;

#ifdef NETDATA_INTERNAL_CHECKS
        if(unlikely(!master_item))
            fatal("DICTIONARY: cannot add an item to a view without a master item.");
#endif
    }
    else {
        // we are on the master dictionary

        if(likely(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE))
            item->shared->value = value;
        else {
            if(likely(value_len)) {
                if(value) {
                    // a value has been supplied
                    // copy it
                    item->shared->value = mallocz(value_len);
                    memcpy(item->shared->value, value, value_len);
                }
                else {
                    // no value has been supplied
                    // allocate a clear memory block
                    item->shared->value = callocz(1, value_len);
                }

            }
            else {
                // the caller wants an item without any value
                item->shared->value = NULL;
            }
        }
        item->shared->value_len = value_len;
        value_size += value_len;

        dictionary_execute_insert_callback(dict, item, constructor_data);
    }

    DICTIONARY_ENTRIES_PLUS1(dict);
    DICTIONARY_STATS_PLUS_MEMORY(dict, key_size, item_size, value_size);

    return item;
}

static void item_reset_value_with_hooks(DICTIONARY *dict, DICTIONARY_ITEM *item, void *value, size_t value_len, void *constructor_data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: %s() should never be called on views.", __FUNCTION__ );

    debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", item_get_name(item));

    DICTIONARY_VALUE_RESETS_PLUS1(dict);

    if(item->shared->value_len != value_len) {
        DICTIONARY_STATS_PLUS_MEMORY(dict, 0, 0, value_len);
        DICTIONARY_STATS_MINUS_MEMORY(dict, 0, 0, item->shared->value_len);
    }

    dictionary_execute_delete_callback(dict, item);

    if(likely(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE)) {
        debug(D_DICTIONARY, "Dictionary: linking value to '%s'", item_get_name(item));
        item->shared->value = value;
        item->shared->value_len = value_len;
    }
    else {
        debug(D_DICTIONARY, "Dictionary: cloning value to '%s'", item_get_name(item));

        void *old_value = item->shared->value;
        void *new_value = NULL;
        if(value_len) {
            new_value = mallocz(value_len);
            if(value) memcpy(new_value, value, value_len);
            else memset(new_value, 0, value_len);
        }
        item->shared->value = new_value;
        item->shared->value_len = value_len;

        debug(D_DICTIONARY, "Dictionary: freeing old value of '%s'", item_get_name(item));
        freez(old_value);
    }

    dictionary_execute_insert_callback(dict, item, constructor_data);
}

static size_t item_free_with_hooks(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", item_get_name(item));

    size_t item_size = 0, key_size = 0, value_size = 0;

    key_size += item->key_len;
    if(unlikely(!(dict->options & DICT_OPTION_NAME_LINK_DONT_CLONE)))
        item_free_name(dict, item);

    if(item_shared_release_and_check_if_it_can_be_freed(dict, item)) {
        dictionary_execute_delete_callback(dict, item);

        if(unlikely(!(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE))) {
            debug(D_DICTIONARY, "Dictionary freeing value of '%s'", item_get_name(item));
            freez(item->shared->value);
            item->shared->value = NULL;
        }
        value_size += item->shared->value_len;

        freez(item->shared);
        item->shared = NULL;
        item_size += sizeof(DICTIONARY_ITEM_SHARED);
    }

    freez(item);
    item_size += sizeof(DICTIONARY_ITEM);

    DICTIONARY_STATS_MINUS_MEMORY(dict, key_size, item_size, value_size);

    // we return the memory we actually freed
    return item_size + (dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE)?0:value_size;
}

// ----------------------------------------------------------------------------
// item operations

static void item_shared_set_deleted(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    if(is_master_dictionary(dict)) {
        item_shared_flag_set(item, ITEM_FLAG_DELETED);

        if(dict->hooks)
            __atomic_store_n(&dict->hooks->last_master_deletion_us, now_realtime_usec(), __ATOMIC_SEQ_CST);
    }
}

static inline void item_free_or_mark_deleted(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    if(item_is_not_referenced_and_can_be_removed(dict, item)) {
        item_shared_set_deleted(dict, item);
        item_linked_list_remove(dict, item);
        item_free_with_hooks(dict, item);
    }
    else {
        item_shared_set_deleted(dict, item);
        item_flag_set(item, ITEM_FLAG_DELETED);
        // after this point do not touch the item
    }

    // the item is not available anymore
    DICTIONARY_ENTRIES_MINUS1(dict);
}

// this is used by traversal functions to remove the current item
// if it is deleted and it has zero references. This will eliminate
// the need for the garbage collector to kick-in later.
// Most deletions happen during traversal, so this is a nice hack
// to speed up everything!
static inline void item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(DICTIONARY *dict, DICTIONARY_ITEM *item, char rw) {
    if(rw == DICTIONARY_LOCK_WRITE) {
        bool should_be_deleted = item_flag_check(item, ITEM_FLAG_DELETED);

        item_release(dict, item);

        if(should_be_deleted && item_is_not_referenced_and_can_be_removed(dict, item)) {
            // this has to be before removing from the linked list,
            // otherwise the garbage collector will also kick in!
            DICTIONARY_PENDING_DELETES_MINUS1(dict);

            item_linked_list_remove(dict, item);
            item_free_with_hooks(dict, item);
        }
    }
    else {
        // we can't do anything under this mode
        item_release(dict, item);
    }
}

static bool item_del(DICTIONARY *dict, const char *name, ssize_t name_len) {
    if(unlikely(!name || !*name)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without a name on a dictionary created from %s() %zu@%s.",
            __FUNCTION__,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);
        return false;
    }

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_del() on a destroyed dictionary");
        return false;
    }

    if(name_len == -1)
        name_len = (ssize_t)strlen(name) + 1; // we need the terminating null too

    debug(D_DICTIONARY, "DEL dictionary entry with name '%s'.", name);

    // Unfortunately, the JudyHSDel() does not return the value of the
    // item that was deleted, so we have to find it before we delete it,
    // since we need to release our structures too.

    dictionary_index_lock_wrlock(dict);

    int ret;
    DICTIONARY_ITEM *item = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(!item)) {
        dictionary_index_lock_unlock(dict);
        ret = false;
    }
    else {
        if(hashtable_delete_unsafe(dict, name, name_len, item) == 0)
            error("DICTIONARY: INTERNAL ERROR: tried to delete item with name '%s' that is not in the index", name);

        dictionary_index_lock_unlock(dict);

        item_free_or_mark_deleted(dict, item);
        ret = true;
    }

    return ret;
}

static DICTIONARY_ITEM *item_add_or_reset_value_and_acquire(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data, DICTIONARY_ITEM *master_item) {
    if(unlikely(!name || !*name)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without a name on a dictionary created from %s() %zu@%s.",
            __FUNCTION__,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);
        return NULL;
    }

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_set() on a destroyed dictionary");
        return NULL;
    }

    if(name_len == -1)
        name_len = (ssize_t)strlen(name) + 1; // we need the terminating null too

    debug(D_DICTIONARY, "SET dictionary entry with name '%s'.", name);

    // DISCUSSION:
    // Is it better to gain a read-lock and do a hashtable_get_unsafe()
    // before we write lock to do hashtable_insert_unsafe()?
    //
    // Probably this depends on the use case.
    // For statsd for example that does dictionary_set() to update received values,
    // it could be beneficial to do a get() before we insert().
    //
    // But the caller has the option to do this on his/her own.
    // So, let's do the fastest here and let the caller decide the flow of calls.

    dictionary_index_lock_wrlock(dict);

    bool added_or_updated = false;
    size_t spins = 0;
    DICTIONARY_ITEM *item = NULL;
    do {
        DICTIONARY_ITEM **item_pptr = (DICTIONARY_ITEM **)hashtable_insert_unsafe(dict, name, name_len);
        if (likely(*item_pptr == 0)) {
            // a new item added to the index

            // create the dictionary item
            item = *item_pptr = item_create_with_hooks(dict, name, name_len, value, value_len, constructor_data, master_item);

            // call the hashtable react
            hashtable_inserted_item_unsafe(dict, item);

            // unlock the index lock, before we add it to the linked list
            // DONT DO IT THE OTHER WAY AROUND - DO NOT CROSS THE LOCKS!
            dictionary_index_lock_unlock(dict);

            item_linked_list_add(dict, item);
            added_or_updated = true;
        }
        else {
            if(item_check_and_acquire_advanced(dict, *item_pptr, true) != ITEM_OK) {
                spins++;
                continue;
            }

            // the item is already in the index
            // so, either we will return the old one
            // or overwrite the value, depending on dictionary flags

            // We should not compare the values here!
            // even if they are the same, we have to do the whole job
            // so that the callbacks will be called.

            item = *item_pptr;

            if(is_view_dictionary(dict)) {
                // view dictionary
                // the item is already there and can be used
                if(item->shared != master_item->shared)
                    error("DICTIONARY: changing the master item on a view is not supported. The previous item will remain. To change the key of an item in a view, delete it and add it again.");
            }
            else {
                // master dictionary
                // the user wants to reset its value

                if (!(dict->options & DICT_OPTION_DONT_OVERWRITE_VALUE)) {
                    item_reset_value_with_hooks(dict, item, value, value_len, constructor_data);
                    added_or_updated = true;
                }

                else if (dictionary_execute_conflict_callback(dict, item, value, constructor_data)) {
                    dictionary_version_increment(dict);
                    added_or_updated = true;
                }

                else {
                    // we did really nothing!
                    ;
                }
            }

            dictionary_index_lock_unlock(dict);
        }
    } while(!item);


    if(unlikely(spins > 0 && dict->stats))
        DICTIONARY_STATS_INSERT_SPINS_PLUS(dict, spins);

    if(is_master_dictionary(dict) && added_or_updated)
        dictionary_execute_react_callback(dict, item, constructor_data);

    return item;
}

static DICTIONARY_ITEM *item_find_and_acquire(DICTIONARY *dict, const char *name, ssize_t name_len) {
    if(unlikely(!name || !*name)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without a name on a dictionary created from %s() %zu@%s.",
            __FUNCTION__,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);
        return NULL;
    }

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_get() on a destroyed dictionary");
        return NULL;
    }

    if(name_len == -1)
        name_len = (ssize_t)strlen(name) + 1; // we need the terminating null too

    debug(D_DICTIONARY, "GET dictionary entry with name '%s'.", name);

    dictionary_index_lock_rdlock(dict);

    DICTIONARY_ITEM *item = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(item && !item_check_and_acquire(dict, item))) {
        item = NULL;
        DICTIONARY_STATS_SEARCH_IGNORES_PLUS1(dict);
    }

    dictionary_index_lock_unlock(dict);

    return item;
}

// ----------------------------------------------------------------------------
// delayed destruction of dictionaries

static bool dictionary_free_all_resources(DICTIONARY *dict, size_t *mem, bool force) {
    if(mem)
        *mem = 0;

    if(!force && dictionary_referenced_items(dict))
        return false;

    size_t dict_size = 0, counted_items = 0, item_size = 0, index_size = 0;
    (void)counted_items;

#ifdef NETDATA_INTERNAL_CHECKS
    long int entries = dict->entries;
    long int referenced_items = dict->referenced_items;
    long int pending_deletion_items = dict->pending_deletion_items;
    const char *creation_function = dict->creation_function;
    const char *creation_file = dict->creation_file;
    size_t creation_line = dict->creation_line;
#endif

    // destroy the index
    index_size += hashtable_destroy_unsafe(dict);

    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);
    DICTIONARY_ITEM *item = dict->items.list;
    while (item) {
        // cache item->next
        // because we are going to free item
        DICTIONARY_ITEM *item_next = item->next;
        item_size += item_free_with_hooks(dict, item);
        item = item_next;

        DICTIONARY_ENTRIES_MINUS1(dict);

        // to speed up destruction, we don't
        // unlink item from the linked-list here

        counted_items++;
    }
    dict->items.list = NULL;
    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);

    dict_size += dictionary_locks_destroy(dict);
    dict_size += reference_counter_free(dict);
    dict_size += dictionary_hooks_free(dict);
    dict_size += sizeof(DICTIONARY);
    DICTIONARY_STATS_MINUS_MEMORY(dict, 0, sizeof(DICTIONARY), 0);

    freez(dict);

    internal_error(
        false,
        "DICTIONARY: Freed dictionary created from %s() %zu@%s, having %ld (counted %zu) entries, %ld referenced, %ld pending deletion, total freed memory: %zu bytes (sizeof(dict) = %zu, sizeof(item) = %zu).",
        creation_function,
        creation_line,
        creation_file,
        entries, counted_items, referenced_items, pending_deletion_items,
        dict_size + item_size, sizeof(DICTIONARY), sizeof(DICTIONARY_ITEM) + sizeof(DICTIONARY_ITEM_SHARED));

    if(mem)
        *mem = dict_size + item_size + index_size;

    return true;
}

netdata_mutex_t dictionaries_waiting_to_be_destroyed_mutex = NETDATA_MUTEX_INITIALIZER;
static DICTIONARY *dictionaries_waiting_to_be_destroyed = NULL;

void dictionary_queue_for_destruction(DICTIONARY *dict) {
    if(is_dictionary_destroyed(dict))
        return;

    DICTIONARY_STATS_DICT_DESTROY_QUEUED_PLUS1(dict);
    dict_flag_set(dict, DICT_FLAG_DESTROYED);

    netdata_mutex_lock(&dictionaries_waiting_to_be_destroyed_mutex);

    dict->next = dictionaries_waiting_to_be_destroyed;
    dictionaries_waiting_to_be_destroyed = dict;

    netdata_mutex_unlock(&dictionaries_waiting_to_be_destroyed_mutex);
}

void cleanup_destroyed_dictionaries(void) {
    if(!dictionaries_waiting_to_be_destroyed)
        return;

    netdata_mutex_lock(&dictionaries_waiting_to_be_destroyed_mutex);

    DICTIONARY *dict, *last = NULL, *next = NULL;
    for(dict = dictionaries_waiting_to_be_destroyed; dict ; dict = next) {
        next = dict->next;

#ifdef NETDATA_INTERNAL_CHECKS
        size_t line = dict->creation_line;
        const char *file = dict->creation_file;
        const char *function = dict->creation_function;
#endif

        DICTIONARY_STATS_DICT_DESTROY_QUEUED_MINUS1(dict);
        if(dictionary_free_all_resources(dict, NULL, false)) {

            internal_error(
                true,
                "DICTIONARY: freed dictionary with delayed destruction, created from %s() %zu@%s.",
                function, line, file);

            if(last) last->next = next;
            else dictionaries_waiting_to_be_destroyed = next;
        }
        else {
            DICTIONARY_STATS_DICT_DESTROY_QUEUED_PLUS1(dict);
            last = dict;
        }
    }

    netdata_mutex_unlock(&dictionaries_waiting_to_be_destroyed_mutex);
}

// ----------------------------------------------------------------------------
// API internal checks

#ifdef NETDATA_INTERNAL_CHECKS
#define api_internal_check(dict, item, allow_null_dict, allow_null_item) api_internal_check_with_trace(dict, item, __FUNCTION__, allow_null_dict, allow_null_item)
static inline void api_internal_check_with_trace(DICTIONARY *dict, DICTIONARY_ITEM *item, const char *function, bool allow_null_dict, bool allow_null_item) {
    if(!allow_null_dict && !dict) {
        internal_error(
            item,
            "DICTIONARY: attempted to %s() with a NULL dictionary, passing an item created from %s() %zu@%s.",
            function,
            item->dict->creation_function,
            item->dict->creation_line,
            item->dict->creation_file);
        fatal("DICTIONARY: attempted to %s() but item is NULL", function);
    }

    if(!allow_null_item && !item) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without an item on a dictionary created from %s() %zu@%s.",
            function,
            dict?dict->creation_function:"unknown",
            dict?dict->creation_line:0,
            dict?dict->creation_file:"unknown");
        fatal("DICTIONARY: attempted to %s() but item is NULL", function);
    }

    if(dict && item && dict != item->dict) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() an item on a dictionary created from %s() %zu@%s, but the item belongs to the dictionary created from %s() %zu@%s.",
            function,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file,
            item->dict->creation_function,
            item->dict->creation_line,
            item->dict->creation_file
        );
        fatal("DICTIONARY: %s(): item does not belong to this dictionary.", function);
    }

    if(item) {
        REFCOUNT refcount = DICTIONARY_ITEM_REFCOUNT_GET(dict, item);
        if (unlikely(refcount <= 0)) {
            internal_error(
                true,
                "DICTIONARY: attempted to %s() of an item with reference counter = %d on a dictionary created from %s() %zu@%s",
                function,
                refcount,
                item->dict->creation_function,
                item->dict->creation_line,
                item->dict->creation_file);
            fatal("DICTIONARY: attempted to %s but item is having refcount = %d", function, refcount);
        }
    }
}
#else
#define api_internal_check(dict, item, allow_null_dict, allow_null_item) debug_dummy()
#endif

#define api_is_name_good(dict, name, name_len) api_is_name_good_with_trace(dict, name, name_len, __FUNCTION__)
static bool api_is_name_good_with_trace(DICTIONARY *dict __maybe_unused, const char *name, ssize_t name_len __maybe_unused, const char *function __maybe_unused) {
    if(unlikely(!name)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() with name = NULL on a dictionary created from %s() %zu@%s.",
            function,
            dict?dict->creation_function:"unknown",
            dict?dict->creation_line:0,
            dict?dict->creation_file:"unknown");
        return false;
    }

    if(unlikely(!*name)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() with empty name on a dictionary created from %s() %zu@%s.",
            function,
            dict?dict->creation_function:"unknown",
            dict?dict->creation_line:0,
            dict?dict->creation_file:"unknown");
        return false;
    }

    internal_error(
        name_len > 0 && name_len != (ssize_t)(strlen(name) + 1),
        "DICTIONARY: attempted to %s() with a name of '%s', having length of %zu (incl. '\\0'), but the supplied name_len = %ld, on a dictionary created from %s() %zu@%s.",
        function,
        name,
        strlen(name) + 1,
        name_len,
        dict?dict->creation_function:"unknown",
        dict?dict->creation_line:0,
        dict?dict->creation_file:"unknown");

    internal_error(
        name_len <= 0 && name_len != -1,
        "DICTIONARY: attempted to %s() with a name of '%s', having length of %zu (incl. '\\0'), but the supplied name_len = %ld, on a dictionary created from %s() %zu@%s.",
        function,
        name,
        strlen(name) + 1,
        name_len,
        dict?dict->creation_function:"unknown",
        dict?dict->creation_line:0,
        dict?dict->creation_file:"unknown");

    return true;
}

// ----------------------------------------------------------------------------
// API - dictionary management

static DICTIONARY *dictionary_create_internal(DICT_OPTIONS options, struct dictionary_stats *stats) {
    cleanup_destroyed_dictionaries();

    DICTIONARY *dict = callocz(1, sizeof(DICTIONARY));
    dict->options = options;
    dict->stats = stats;

    size_t dict_size = 0;
    dict_size += sizeof(DICTIONARY);
    dict_size += dictionary_locks_init(dict);
    dict_size += reference_counter_init(dict);
    dict_size += hashtable_init_unsafe(dict);

    DICTIONARY_STATS_PLUS_MEMORY(dict, 0, dict_size, 0);

    return dict;
}

#ifdef NETDATA_INTERNAL_CHECKS
DICTIONARY *dictionary_create_advanced_with_trace(DICT_OPTIONS options, struct dictionary_stats *stats, const char *function, size_t line, const char *file) {
#else
DICTIONARY *dictionary_create_advanced(DICT_OPTIONS options, struct dictionary_stats *stats) {
#endif

    DICTIONARY *dict = dictionary_create_internal(options, stats?stats:&dictionary_stats_category_other);

#ifdef NETDATA_INTERNAL_CHECKS
    dict->creation_function = function;
    dict->creation_file = file;
    dict->creation_line = line;
#endif

    DICTIONARY_STATS_DICT_CREATIONS_PLUS1(dict);
    return dict;
}

#ifdef NETDATA_INTERNAL_CHECKS
DICTIONARY *dictionary_create_view_with_trace(DICTIONARY *master, const char *function, size_t line, const char *file) {
#else
DICTIONARY *dictionary_create_view(DICTIONARY *master) {
#endif

    DICTIONARY *dict = dictionary_create_internal(master->options, master->stats);
    dict->master = master;

    dictionary_hooks_allocate(master);

    if(unlikely(__atomic_load_n(&master->hooks->links, __ATOMIC_SEQ_CST)) < 1)
        fatal("DICTIONARY: attempted to create a view that has %d links", master->hooks->links);

    dict->hooks = master->hooks;
    __atomic_add_fetch(&master->hooks->links, 1, __ATOMIC_SEQ_CST);

#ifdef NETDATA_INTERNAL_CHECKS
    dict->creation_function = function;
    dict->creation_file = file;
    dict->creation_line = line;
#endif

    DICTIONARY_STATS_DICT_CREATIONS_PLUS1(dict);
    return dict;
}

void dictionary_flush(DICTIONARY *dict) {
    if(unlikely(!dict))
        return;

    // delete the index
    dictionary_index_lock_wrlock(dict);
    hashtable_destroy_unsafe(dict);
    dictionary_index_lock_unlock(dict);

    // delete all items
    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE); // get write lock here, to speed it up (it is recursive)
    DICTIONARY_ITEM *item, *item_next;
    for (item = dict->items.list; item; item = item_next) {
        item_next = item->next;

        if(!item_flag_check(item, ITEM_FLAG_DELETED))
            item_free_or_mark_deleted(dict, item);
    }
    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);

    DICTIONARY_STATS_DICT_FLUSHES_PLUS1(dict);
}

size_t dictionary_destroy(DICTIONARY *dict) {
    cleanup_destroyed_dictionaries();

    if(!dict) return 0;

    DICTIONARY_STATS_DICT_DESTRUCTIONS_PLUS1(dict);

    size_t referenced_items = dictionary_referenced_items(dict);
    if(referenced_items) {
        dictionary_flush(dict);
        dictionary_queue_for_destruction(dict);

        internal_error(
            true,
            "DICTIONARY: delaying destruction of dictionary created from %s() %zu@%s, because it has %ld referenced items in it (%ld total).",
            dict->creation_function,
            dict->creation_line,
            dict->creation_file,
            dict->referenced_items,
            dict->entries);

        return 0;
    }

    size_t freed;
    dictionary_free_all_resources(dict, &freed, true);

    return freed;
}

// ----------------------------------------------------------------------------
// SET an item to the dictionary

DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_set_and_acquire_item_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data) {
    if(unlikely(!api_is_name_good(dict, name, name_len)))
        return NULL;

    api_internal_check(dict, NULL, false, true);

    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: this dictionary is a view, you cannot add items other than the ones from the master dictionary.");

    DICTIONARY_ITEM *item = item_add_or_reset_value_and_acquire(dict, name, name_len, value, value_len, constructor_data, NULL);
    api_internal_check(dict, item, false, false);
    return item;
}

void *dictionary_set_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data) {
    DICTIONARY_ITEM *item = dictionary_set_and_acquire_item_advanced(dict, name, name_len, value, value_len, constructor_data);

    if(likely(item)) {
        void *v = item->shared->value;
        item_release(dict, item);
        return v;
    }

    return NULL;
}

DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_view_set_and_acquire_item_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, DICTIONARY_ITEM *master_item) {
    if(unlikely(!api_is_name_good(dict, name, name_len)))
        return NULL;

    api_internal_check(dict, NULL, false, true);

    if(unlikely(is_master_dictionary(dict)))
        fatal("DICTIONARY: this dictionary is a master, you cannot add items from other dictionaries.");

    dictionary_acquired_item_dup(dict->master, master_item);
    DICTIONARY_ITEM *item = item_add_or_reset_value_and_acquire(dict, name, name_len, NULL, 0, NULL, master_item);
    dictionary_acquired_item_release(dict->master, master_item);

    api_internal_check(dict, item, false, false);
    return item;
}

void *dictionary_view_set_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, DICTIONARY_ITEM *master_item) {
    DICTIONARY_ITEM *item = dictionary_view_set_and_acquire_item_advanced(dict, name, name_len, master_item);

    if(likely(item)) {
        void *v = item->shared->value;
        item_release(dict, item);
        return v;
    }

    return NULL;
}

// ----------------------------------------------------------------------------
// GET an item from the dictionary

DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_get_and_acquire_item_advanced(DICTIONARY *dict, const char *name, ssize_t name_len) {
    if(unlikely(!api_is_name_good(dict, name, name_len)))
        return NULL;

    api_internal_check(dict, NULL, false, true);
    DICTIONARY_ITEM *item = item_find_and_acquire(dict, name, name_len);
    api_internal_check(dict, item, false, true);
    return item;
}

void *dictionary_get_advanced(DICTIONARY *dict, const char *name, ssize_t name_len) {
    DICTIONARY_ITEM *item = dictionary_get_and_acquire_item_advanced(dict, name, name_len);

    if(likely(item)) {
        void *v = item->shared->value;
        item_release(dict, item);
        return v;
    }

    return NULL;
}

// ----------------------------------------------------------------------------
// DUP/REL an item (increase/decrease its reference counter)

DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_acquired_item_dup(DICTIONARY *dict, DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    // we allow the item to be NULL here
    api_internal_check(dict, item, false, true);

    if(likely(item)) {
        item_acquire(dict, item);
        api_internal_check(dict, item, false, false);
    }

    return item;
}

void dictionary_acquired_item_release(DICTIONARY *dict, DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    // we allow the item to be NULL here
    api_internal_check(dict, item, false, true);

    // no need to get a lock here
    // we pass the last parameter to reference_counter_release() as true
    // so that the release may get a write-lock if required to clean up

    if(likely(item))
        item_release(dict, item);
}

// ----------------------------------------------------------------------------
// get the name/value of an item

const char *dictionary_acquired_item_name(DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    api_internal_check(NULL, item, true, false);
    return item_get_name(item);
}

void *dictionary_acquired_item_value(DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    // we allow the item to be NULL here
    api_internal_check(NULL, item, true, true);

    if(likely(item))
        return item->shared->value;

    return NULL;
}

// ----------------------------------------------------------------------------
// DEL an item

bool dictionary_del_advanced(DICTIONARY *dict, const char *name, ssize_t name_len) {
    if(unlikely(!api_is_name_good(dict, name, name_len)))
        return false;

    api_internal_check(dict, NULL, false, true);
    return item_del(dict, name, name_len);
}

// ----------------------------------------------------------------------------
// traversal with loop

void *dictionary_foreach_start_rw(DICTFE *dfe, DICTIONARY *dict, char rw) {
    if(unlikely(!dfe || !dict)) return NULL;

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_foreach_start_rw() on a destroyed dictionary");
        dfe->counter = 0;
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
        return NULL;
    }

    dfe->counter = 0;
    dfe->dict = dict;
    dfe->rw = rw;

    ll_recursive_lock(dict, dfe->rw);

    DICTIONARY_STATS_TRAVERSALS_PLUS1(dict);

    // get the first item from the list
    DICTIONARY_ITEM *item = dict->items.list;

    // skip all the deleted items
    while(item && !item_check_and_acquire(dict, item))
        item = item->next;

    if(likely(item)) {
        dfe->item = item;
        dfe->name = (char *)item_get_name(item);
        dfe->value = item->shared->value;
    }
    else {
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
    }

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT))
        ll_recursive_unlock(dfe->dict, dfe->rw);

    return dfe->value;
}

void *dictionary_foreach_next(DICTFE *dfe) {
    if(unlikely(!dfe || !dfe->dict)) return NULL;

    if(unlikely(is_dictionary_destroyed(dfe->dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_foreach_next() on a destroyed dictionary");
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
        return NULL;
    }

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT))
        ll_recursive_lock(dfe->dict, dfe->rw);

    // the item we just did
    DICTIONARY_ITEM *item = dfe->item;

    // get the next item from the list
    DICTIONARY_ITEM *item_next = (item) ? item->next : NULL;

    // skip all the deleted items until one that can be acquired is found
    while(item_next && !item_check_and_acquire(dfe->dict, item_next))
        item_next = item_next->next;

    if(likely(item)) {
        item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dfe->dict, item, dfe->rw);
        // item_release(dfe->dict, item);
    }

    item = item_next;
    if(likely(item)) {
        dfe->item = item;
        dfe->name = (char *)item_get_name(item);
        dfe->value = item->shared->value;
        dfe->counter++;
    }
    else {
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
    }

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT))
        ll_recursive_unlock(dfe->dict, dfe->rw);

    return dfe->value;
}

void dictionary_foreach_done(DICTFE *dfe) {
    if(unlikely(!dfe || !dfe->dict)) return;

    if(unlikely(is_dictionary_destroyed(dfe->dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_foreach_next() on a destroyed dictionary");
        return;
    }

    // the item we just did
    DICTIONARY_ITEM *item = dfe->item;

    // release it, so that it can possibly be deleted
    if(likely(item)) {
        item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dfe->dict, item, dfe->rw);
        // item_release(dfe->dict, item);
    }

    if(likely(dfe->rw != DICTIONARY_LOCK_REENTRANT))
        ll_recursive_unlock(dfe->dict, dfe->rw);

    dfe->dict = NULL;
    dfe->item = NULL;
    dfe->name = NULL;
    dfe->value = NULL;
    dfe->counter = 0;
}

// ----------------------------------------------------------------------------
// API - walk through the dictionary.
// The dictionary is locked for reading while this happens
// do not use other dictionary calls while walking the dictionary - deadlock!

int dictionary_walkthrough_rw(DICTIONARY *dict, char rw, int (*callback)(const DICTIONARY_ITEM *item, void *entry, void *data), void *data) {
    if(unlikely(!dict || !callback)) return 0;

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_walkthrough_rw() on a destroyed dictionary");
        return 0;
    }

    ll_recursive_lock(dict, rw);

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    // written in such a way, that the callback can delete the active element

    int ret = 0;
    DICTIONARY_ITEM *item = dict->items.list, *item_next;
    while(item) {

        // skip the deleted items
        if(unlikely(!item_check_and_acquire(dict, item))) {
            item = item->next;
            continue;
        }

        if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
            ll_recursive_unlock(dict, rw);

        int r = callback(item, item->shared->value, data);

        if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
            ll_recursive_lock(dict, rw);

        // since we have a reference counter, this item cannot be deleted
        // until we release the reference counter, so the pointers are there
        item_next = item->next;

        item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dict, item, rw);
        // item_release(dict, item);

        if(unlikely(r < 0)) {
            ret = r;
            break;
        }

        ret += r;

        item = item_next;
    }

    ll_recursive_unlock(dict, rw);

    return ret;
}

// ----------------------------------------------------------------------------
// sorted walkthrough

static int dictionary_sort_compar(const void *item1, const void *item2) {
    return strcmp(item_get_name((*(DICTIONARY_ITEM **)item1)), item_get_name((*(DICTIONARY_ITEM **)item2)));
}

int dictionary_sorted_walkthrough_rw(DICTIONARY *dict, char rw, int (*callback)(const DICTIONARY_ITEM *item, void *entry, void *data), void *data) {
    if(unlikely(!dict || !callback)) return 0;

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_sorted_walkthrough_rw() on a destroyed dictionary");
        return 0;
    }

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    ll_recursive_lock(dict, rw);
    size_t entries = __atomic_load_n(&dict->entries, __ATOMIC_SEQ_CST);
    DICTIONARY_ITEM **array = mallocz(sizeof(DICTIONARY_ITEM *) * entries);

    size_t i;
    DICTIONARY_ITEM *item;
    for(item = dict->items.list, i = 0; item && i < entries; item = item->next) {
        if(likely(item_check_and_acquire(dict, item)))
            array[i++] = item;
    }
    ll_recursive_unlock(dict, rw);

    if(unlikely(i != entries))
        entries = i;

    qsort(array, entries, sizeof(DICTIONARY_ITEM *), dictionary_sort_compar);

    bool callit = true;
    int ret = 0, r;
    for(i = 0; i < entries ;i++) {
        item = array[i];

        if(callit)
            r = callback(item, item->shared->value, data);

        item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dict, item, rw);
        // item_release(dict, item);

        if(r < 0) {
            ret = r;
            r = 0;

            // stop calling the callback,
            // but we have to continue, to release all the reference counters
            callit = false;
        }
        else
            ret += r;
    }

    freez(array);

    return ret;
}

// ----------------------------------------------------------------------------
// THREAD_CACHE

static __thread Pvoid_t thread_cache_judy_array = NULL;

void *thread_cache_entry_get_or_set(void *key,
                                    ssize_t key_length,
                                    void *value,
                                    void *(*transform_the_value_before_insert)(void *key, size_t key_length, void *value)
                                    ) {
    if(unlikely(!key || !key_length)) return NULL;

    if(key_length == -1)
        key_length = (ssize_t)strlen((char *)key) + 1;

    JError_t J_Error;
    Pvoid_t *Rc = JudyHSIns(&thread_cache_judy_array, key, key_length, &J_Error);
    if (unlikely(Rc == PJERR)) {
        fatal("THREAD_CACHE: Cannot insert entry to JudyHS, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    if(*Rc == 0) {
        // new item added

        *Rc = (transform_the_value_before_insert) ? transform_the_value_before_insert(key, key_length, value) : value;
    }

    return *Rc;
}

void thread_cache_destroy(void) {
    if(unlikely(!thread_cache_judy_array)) return;

    JError_t J_Error;
    Word_t ret = JudyHSFreeArray(&thread_cache_judy_array, &J_Error);
    if(unlikely(ret == (Word_t) JERR)) {
        error("THREAD_CACHE: Cannot destroy JudyHS, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    internal_error(true, "THREAD_CACHE: hash table freed %lu bytes", ret);

    thread_cache_judy_array = NULL;
}

// ----------------------------------------------------------------------------
// unit test

static void dictionary_unittest_free_char_pp(char **pp, size_t entries) {
    for(size_t i = 0; i < entries ;i++)
        freez(pp[i]);

    freez(pp);
}

static char **dictionary_unittest_generate_names(size_t entries) {
    char **names = mallocz(sizeof(char *) * entries);
    for(size_t i = 0; i < entries ;i++) {
        char buf[25 + 1] = "";
        snprintfz(buf, 25, "name.%zu.0123456789.%zu \t !@#$%%^&*(),./[]{}\\|~`", i, entries / 2 + i);
        names[i] = strdupz(buf);
    }
    return names;
}

static char **dictionary_unittest_generate_values(size_t entries) {
    char **values = mallocz(sizeof(char *) * entries);
    for(size_t i = 0; i < entries ;i++) {
        char buf[25 + 1] = "";
        snprintfz(buf, 25, "value-%zu-0987654321.%zu%%^&*(),. \t !@#$/[]{}\\|~`", i, entries / 2 + i);
        values[i] = strdupz(buf);
    }
    return values;
}

static size_t dictionary_unittest_set_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], values[i], vallen);
        if(val == values[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, values[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_set_null(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    size_t errors = 0;
    size_t i = 0;
    for(; i < entries ;i++) {
        void *val = dictionary_set(dict, names[i], NULL, 0);
        if(val != NULL) { fprintf(stderr, ">>> %s() returns a non NULL value\n", __FUNCTION__); errors++; }
    }
    if(dictionary_entries(dict) != i) {
        fprintf(stderr, ">>> %s() dictionary items do not match\n", __FUNCTION__);
        errors++;
    }
    return errors;
}


static size_t dictionary_unittest_set_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], values[i], vallen);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]) + 1;
        char *val = (char *)dictionary_get(dict, names[i]);
        if(val == values[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, values[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        char *val = (char *)dictionary_get(dict, names[i]);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_nonexisting(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        char *val = (char *)dictionary_get(dict, values[i]);
        if(val) { fprintf(stderr, ">>> %s() returns non-existing item\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_del_nonexisting(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        bool ret = dictionary_del(dict, values[i]);
        if(ret) { fprintf(stderr, ">>> %s() deleted non-existing item\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_del_existing(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    size_t errors = 0;

    size_t forward_from = 0, forward_to = entries / 3;
    size_t middle_from = forward_to, middle_to = entries * 2 / 3;
    size_t backward_from = middle_to, backward_to = entries;

    for(size_t i = forward_from; i < forward_to ;i++) {
        bool ret = dictionary_del(dict, names[i]);
        if(!ret) { fprintf(stderr, ">>> %s() didn't delete (forward) existing item\n", __FUNCTION__); errors++; }
    }

    for(size_t i = middle_to - 1; i >= middle_from ;i--) {
        bool ret = dictionary_del(dict, names[i]);
        if(!ret) { fprintf(stderr, ">>> %s() didn't delete (middle) existing item\n", __FUNCTION__); errors++; }
    }

    for(size_t i = backward_to - 1; i >= backward_from ;i--) {
        bool ret = dictionary_del(dict, names[i]);
        if(!ret) { fprintf(stderr, ">>> %s() didn't delete (backward) existing item\n", __FUNCTION__); errors++; }
    }

    return errors;
}

static size_t dictionary_unittest_reset_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val == names[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, names[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_reset_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val != names[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
        if(!val)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_reset_dont_overwrite_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static int dictionary_unittest_walkthrough_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    return 1;
}

static size_t dictionary_unittest_walkthrough(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    int sum = dictionary_walkthrough_read(dict, dictionary_unittest_walkthrough_callback, NULL);
    if(sum < (int)entries) return entries - sum;
    else return sum - entries;
}

static int dictionary_unittest_walkthrough_delete_this_callback(const DICTIONARY_ITEM *item, void *value __maybe_unused, void *data) {
    const char *name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    if(!dictionary_del((DICTIONARY *)data, name))
        return 0;

    return 1;
}

static size_t dictionary_unittest_walkthrough_delete_this(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    int sum = dictionary_walkthrough_write(dict, dictionary_unittest_walkthrough_delete_this_callback, dict);
    if(sum < (int)entries) return entries - sum;
    else return sum - entries;
}

static int dictionary_unittest_walkthrough_stop_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    return -1;
}

static size_t dictionary_unittest_walkthrough_stop(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    int sum = dictionary_walkthrough_read(dict, dictionary_unittest_walkthrough_stop_callback, NULL);
    if(sum != -1) return 1;
    return 0;
}

static size_t dictionary_unittest_foreach(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t count = 0;
    char *item;
    dfe_start_read(dict, item)
        count++;
    dfe_done(item);

    if(count > entries) return count - entries;
    return entries - count;
}

static size_t dictionary_unittest_foreach_delete_this(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t count = 0;
    char *item;
    dfe_start_write(dict, item)
        if(dictionary_del(dict, item_dfe.name)) count++;
    dfe_done(item);

    if(count > entries) return count - entries;
    return entries - count;
}

static size_t dictionary_unittest_destroy(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t bytes = dictionary_destroy(dict);
    fprintf(stderr, " %s() freed %zu bytes,", __FUNCTION__, bytes);
    return 0;
}

static usec_t dictionary_unittest_run_and_measure_time(DICTIONARY *dict, char *message, char **names, char **values, size_t entries, size_t *errors, size_t (*callback)(DICTIONARY *dict, char **names, char **values, size_t entries)) {
    fprintf(stderr, "%40s ... ", message);

    usec_t started = now_realtime_usec();
    size_t errs = callback(dict, names, values, entries);
    usec_t ended = now_realtime_usec();
    usec_t dt = ended - started;

    if(callback == dictionary_unittest_destroy) dict = NULL;

    long int found_ok = 0, found_deleted = 0, found_referenced = 0;
    if(dict) {
        DICTIONARY_ITEM *item;
        DOUBLE_LINKED_LIST_FOREACH_FORWARD(dict->items.list, item, prev, next) {
            if(item->refcount >= 0 && !(item ->flags & ITEM_FLAG_DELETED))
                found_ok++;
            else
                found_deleted++;

            if(item->refcount > 0)
                found_referenced++;
        }
    }

    fprintf(stderr, " %zu errors, %ld (found %ld) items in dictionary, %ld (found %ld) referenced, %ld (found %ld) deleted, %llu usec \n",
            errs, dict?dict->entries:0, found_ok, dict?dict->referenced_items:0, found_referenced, dict?dict->pending_deletion_items:0, found_deleted, dt);
    *errors += errs;
    return dt;
}

static void dictionary_unittest_clone(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_clone);
    dictionary_unittest_run_and_measure_time(dict, "getting entries", names, values, entries, errors, dictionary_unittest_get_clone);
    dictionary_unittest_run_and_measure_time(dict, "getting non-existing entries", names, values, entries, errors, dictionary_unittest_get_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "resetting entries", names, values, entries, errors, dictionary_unittest_reset_clone);
    dictionary_unittest_run_and_measure_time(dict, "deleting non-existing entries", names, values, entries, errors, dictionary_unittest_del_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "deleting existing entries", names, values, entries, errors, dictionary_unittest_del_existing);
    dictionary_unittest_run_and_measure_time(dict, "walking through empty", names, values, 0, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach empty", names, values, 0, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, errors, dictionary_unittest_destroy);
}

static void dictionary_unittest_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "getting entries", names, values, entries, errors, dictionary_unittest_get_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "getting non-existing entries", names, values, entries, errors, dictionary_unittest_get_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "resetting entries", names, values, entries, errors, dictionary_unittest_reset_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "deleting non-existing entries", names, values, entries, errors, dictionary_unittest_del_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "deleting existing entries", names, values, entries, errors, dictionary_unittest_del_existing);
    dictionary_unittest_run_and_measure_time(dict, "walking through empty", names, values, 0, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach empty", names, values, 0, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, errors, dictionary_unittest_destroy);
}

struct dictionary_unittest_sorting {
    const char *old_name;
    const char *old_value;
    size_t count;
};

static int dictionary_unittest_sorting_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    struct dictionary_unittest_sorting *t = (struct dictionary_unittest_sorting *)data;
    const char *v = (const char *)value;

    int ret = 0;
    if(t->old_name && strcmp(t->old_name, name) > 0) {
        fprintf(stderr, "name '%s' should be after '%s'\n", t->old_name, name);
        ret = 1;
    }
    t->count++;
    t->old_name = name;
    t->old_value = v;

    return ret;
}

static size_t dictionary_unittest_sorted_walkthrough(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    struct dictionary_unittest_sorting tmp = { .old_name = NULL, .old_value = NULL, .count = 0 };
    size_t errors;
    errors = dictionary_sorted_walkthrough_read(dict, dictionary_unittest_sorting_callback, &tmp);

    if(tmp.count != entries) {
        fprintf(stderr, "Expected %zu entries, counted %zu\n", entries, tmp.count);
        errors++;
    }
    return errors;
}

static void dictionary_unittest_sorting(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_clone);
    dictionary_unittest_run_and_measure_time(dict, "sorted walkthrough", names, values, entries, errors, dictionary_unittest_sorted_walkthrough);
}

static void dictionary_unittest_null_dfe(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding null value entries", names, values, entries, errors, dictionary_unittest_set_null);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
}


static int unittest_check_dictionary_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    return 1;
}

static size_t unittest_check_dictionary(const char *label, DICTIONARY *dict, size_t traversable, size_t active_items, size_t deleted_items, size_t referenced_items, size_t pending_deletion) {
    size_t errors = 0;

    size_t ll = 0;
    void *t;
    dfe_start_read(dict, t)
        ll++;
    dfe_done(t);

    fprintf(stderr, "DICT %-20s: dictionary foreach entries %zu, expected %zu...\t\t\t\t\t",
            label, ll, traversable);
    if(ll != traversable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    ll = dictionary_walkthrough_read(dict, unittest_check_dictionary_callback, NULL);
    fprintf(stderr, "DICT %-20s: dictionary walkthrough entries %zu, expected %zu...\t\t\t\t",
            label, ll, traversable);
    if(ll != traversable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    ll = dictionary_sorted_walkthrough_read(dict, unittest_check_dictionary_callback, NULL);
    fprintf(stderr, "DICT %-20s: dictionary sorted walkthrough entries %zu, expected %zu...\t\t\t",
            label, ll, traversable);
    if(ll != traversable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    DICTIONARY_ITEM *item;
    size_t active = 0, deleted = 0, referenced = 0, pending = 0;
    for(item = dict->items.list; item; item = item->next) {
        if(!(item->flags & ITEM_FLAG_DELETED) && !(item->shared->flags & ITEM_FLAG_DELETED))
            active++;
        else {
            deleted++;

            if(item->refcount == 0)
                pending++;
        }

        if(item->refcount > 0)
            referenced++;
    }

    fprintf(stderr, "DICT %-20s: dictionary active items reported %ld, counted %zu, expected %zu...\t\t\t",
            label, dict->entries, active, active_items);
    if(active != active_items || active != (size_t)dict->entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "DICT %-20s: dictionary deleted items counted %zu, expected %zu...\t\t\t\t",
            label, deleted, deleted_items);
    if(deleted != deleted_items) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "DICT %-20s: dictionary referenced items reported %ld, counted %zu, expected %zu...\t\t",
            label, dict->referenced_items, referenced, referenced_items);
    if(referenced != referenced_items || dict->referenced_items != (long int)referenced) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "DICT %-20s: dictionary pending deletion items reported %ld, counted %zu, expected %zu...\t",
            label, dict->pending_deletion_items, pending, pending_deletion);
    if(pending != pending_deletion || pending != (size_t)dict->pending_deletion_items) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    return errors;
}

static int check_item_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    return value == data;
}

static size_t unittest_check_item(const char *label, DICTIONARY *dict,
    DICTIONARY_ITEM *item, const char *name, const char *value, int refcount,
    ITEM_FLAGS deleted_flags, bool searchable, bool browsable, bool linked) {
    size_t errors = 0;

    fprintf(stderr, "ITEM %-20s: name is '%s', expected '%s'...\t\t\t\t\t\t", label, item_get_name(item), name);
    if(strcmp(item_get_name(item), name) != 0) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "ITEM %-20s: value is '%s', expected '%s'...\t\t\t\t\t", label, (const char *)item->shared->value, value);
    if(strcmp((const char *)item->shared->value, value) != 0) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "ITEM %-20s: refcount is %d, expected %d...\t\t\t\t\t\t\t", label, item->refcount, refcount);
    if (item->refcount != refcount) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "ITEM %-20s: deleted flag is %s, expected %s...\t\t\t\t\t", label,
            (item->flags & ITEM_FLAG_DELETED || item->shared->flags & ITEM_FLAG_DELETED)?"true":"false",
            (deleted_flags & ITEM_FLAG_DELETED)?"true":"false");

    if ((item->flags & ITEM_FLAG_DELETED || item->shared->flags & ITEM_FLAG_DELETED) != (deleted_flags & ITEM_FLAG_DELETED)) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    void *v = dictionary_get(dict, name);
    bool found = v == item->shared->value;
    fprintf(stderr, "ITEM %-20s: searchable %5s, expected %5s...\t\t\t\t\t\t", label,
            found?"true":"false", searchable?"true":"false");
    if(found != searchable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = false;
    void *t;
    dfe_start_read(dict, t) {
        if(t == item->shared->value) found = true;
    }
    dfe_done(t);

    fprintf(stderr, "ITEM %-20s: dfe browsable %5s, expected %5s...\t\t\t\t\t", label,
            found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = dictionary_walkthrough_read(dict, check_item_callback, item->shared->value);
    fprintf(stderr, "ITEM %-20s: walkthrough browsable %5s, expected %5s...\t\t\t\t", label,
            found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = dictionary_sorted_walkthrough_read(dict, check_item_callback, item->shared->value);
    fprintf(stderr, "ITEM %-20s: sorted walkthrough browsable %5s, expected %5s...\t\t\t", label,
            found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = false;
    DICTIONARY_ITEM *n;
    for(n = dict->items.list; n ;n = n->next)
        if(n == item) found = true;

    fprintf(stderr, "ITEM %-20s: linked %5s, expected %5s...\t\t\t\t\t\t", label,
            found?"true":"false", linked?"true":"false");
    if(found != linked) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    return errors;
}

struct thread_unittest {
    int join;
    DICTIONARY *dict;
    int dups;
};

static void *unittest_dict_thread(void *arg) {
    struct thread_unittest *tu = arg;
    for(; 1 ;) {
        if(__atomic_load_n(&tu->join, __ATOMIC_RELAXED))
            break;

        DICT_ITEM_CONST DICTIONARY_ITEM *item =
            dictionary_set_and_acquire_item_advanced(tu->dict, "dict thread checking 1234567890",
                                                     -1, NULL, 0, NULL);


        dictionary_get(tu->dict, dictionary_acquired_item_name(item));

        void *t1;
        dfe_start_write(tu->dict, t1) {

            // this should delete the referenced item
            dictionary_del(tu->dict, t1_dfe.name);

            void *t2;
            dfe_start_write(tu->dict, t2) {
                // this should add another
                dictionary_set(tu->dict, t2_dfe.name, NULL, 0);

                dictionary_get(tu->dict, dictionary_acquired_item_name(item));

                // and this should delete it again
                dictionary_del(tu->dict, t2_dfe.name);
            }
            dfe_done(t2);

            // this should fail to add it
            dictionary_set(tu->dict, t1_dfe.name, NULL, 0);
            dictionary_del(tu->dict, t1_dfe.name);
        }
        dfe_done(t1);

        for(int i = 0; i < tu->dups ; i++) {
            dictionary_acquired_item_dup(tu->dict, item);
            dictionary_get(tu->dict, dictionary_acquired_item_name(item));
        }

        for(int i = 0; i < tu->dups ; i++) {
            dictionary_acquired_item_release(tu->dict, item);
            dictionary_del(tu->dict, dictionary_acquired_item_name(item));
        }

        dictionary_acquired_item_release(tu->dict, item);

        dictionary_del(tu->dict, "dict thread checking 1234567890");
    }

    return arg;
}

static int dictionary_unittest_threads() {

    struct thread_unittest tu = {
        .join = 0,
        .dict = NULL,
        .dups = 1,
    };

    // threads testing of dictionary
    tu.dict = dictionary_create(DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE);
    time_t seconds_to_run = 5;
    int threads_to_create = 2;
    fprintf(
        stderr,
        "\nChecking dictionary concurrency with %d threads for %ld seconds...\n",
        threads_to_create,
        seconds_to_run);

    netdata_thread_t threads[threads_to_create];
    tu.join = 0;
    for (int i = 0; i < threads_to_create; i++) {
        char buf[100 + 1];
        snprintf(buf, 100, "dict%d", i);
        netdata_thread_create(
            &threads[i],
            buf,
            NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
            unittest_dict_thread,
            &tu);
    }
    sleep_usec(seconds_to_run * USEC_PER_SEC);

    __atomic_store_n(&tu.join, 1, __ATOMIC_RELAXED);
    for (int i = 0; i < threads_to_create; i++) {
        void *retval;
        netdata_thread_join(threads[i], &retval);
    }

    fprintf(stderr,
            "inserts %zu"
            ", deletes %zu"
            ", searches %zu"
            ", resets %zu"
            ", entries %ld"
            ", referenced_items %ld"
            ", pending deletions %ld"
            ", check spins %zu"
            ", insert spins %zu"
            ", search ignores %zu"
            "\n",
            tu.dict->stats->ops.inserts,
            tu.dict->stats->ops.deletes,
            tu.dict->stats->ops.searches,
            tu.dict->stats->ops.resets,
            tu.dict->entries,
            tu.dict->referenced_items,
            tu.dict->pending_deletion_items,
            tu.dict->stats->spin_locks.use,
            tu.dict->stats->spin_locks.insert,
            tu.dict->stats->spin_locks.search
    );
    dictionary_destroy(tu.dict);
    tu.dict = NULL;

    return 0;
}

struct thread_view_unittest {
    int join;
    DICTIONARY *master;
    DICTIONARY *view;
    DICTIONARY_ITEM *item_master;
    int dups;
};

static void *unittest_dict_master_thread(void *arg) {
    struct thread_view_unittest *tv = arg;

    DICTIONARY_ITEM *item = NULL;
    int loops = 0;
    while(!__atomic_load_n(&tv->join, __ATOMIC_SEQ_CST)) {

        if(!item)
            item = dictionary_set_and_acquire_item(tv->master, "ITEM1", "123", strlen("123") + 1);

        if(__atomic_load_n(&tv->item_master, __ATOMIC_SEQ_CST) != NULL) {
            dictionary_acquired_item_release(tv->master, item);
            dictionary_del(tv->master, "ITEM1");
            item = NULL;
            loops++;
            continue;
        }

        dictionary_acquired_item_dup(tv->master, item); // for the view thread
        __atomic_store_n(&tv->item_master, item, __ATOMIC_SEQ_CST);
        dictionary_del(tv->master, "ITEM1");


        for(int i = 0; i < tv->dups + loops ; i++) {
            dictionary_acquired_item_dup(tv->master, item);
        }

        for(int i = 0; i < tv->dups + loops ; i++) {
            dictionary_acquired_item_release(tv->master, item);
        }

        dictionary_acquired_item_release(tv->master, item);

        item = NULL;
        loops = 0;
    }

    return arg;
}

static void *unittest_dict_view_thread(void *arg) {
    struct thread_view_unittest *tv = arg;

    DICTIONARY_ITEM *m_item = NULL;

    while(!__atomic_load_n(&tv->join, __ATOMIC_SEQ_CST)) {
        if(!(m_item = __atomic_load_n(&tv->item_master, __ATOMIC_SEQ_CST)))
            continue;

        DICTIONARY_ITEM *v_item = dictionary_view_set_and_acquire_item(tv->view, "ITEM2", m_item);
        dictionary_acquired_item_release(tv->master, m_item);
        __atomic_store_n(&tv->item_master, NULL, __ATOMIC_SEQ_CST);

        for(int i = 0; i < tv->dups ; i++) {
            dictionary_acquired_item_dup(tv->view, v_item);
        }

        for(int i = 0; i < tv->dups ; i++) {
            dictionary_acquired_item_release(tv->view, v_item);
        }

        dictionary_del(tv->view, "ITEM2");

        while(!__atomic_load_n(&tv->join, __ATOMIC_SEQ_CST) && !(m_item = __atomic_load_n(&tv->item_master, __ATOMIC_SEQ_CST))) {
            dictionary_acquired_item_dup(tv->view, v_item);
            dictionary_acquired_item_release(tv->view, v_item);
        }

        dictionary_acquired_item_release(tv->view, v_item);
    }

    return arg;
}

static int dictionary_unittest_view_threads() {

    struct thread_view_unittest tv = {
        .join = 0,
        .master = NULL,
        .view = NULL,
        .item_master = NULL,
        .dups = 1,
    };

    // threads testing of dictionary
    struct dictionary_stats stats_master = {};
    struct dictionary_stats stats_view = {};
    tv.master = dictionary_create_advanced(DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE, &stats_master);
    tv.view = dictionary_create_view(tv.master);
    tv.view->stats = &stats_view;

    time_t seconds_to_run = 5;
    fprintf(
        stderr,
        "\nChecking dictionary concurrency with 1 master and 1 view threads for %ld seconds...\n",
        seconds_to_run);

    netdata_thread_t master_thread, view_thread;
    tv.join = 0;

    netdata_thread_create(
        &master_thread,
        "master",
        NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
        unittest_dict_master_thread,
        &tv);

    netdata_thread_create(
        &view_thread,
        "view",
        NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE,
        unittest_dict_view_thread,
        &tv);

    sleep_usec(seconds_to_run * USEC_PER_SEC);

    __atomic_store_n(&tv.join, 1, __ATOMIC_RELAXED);
    void *retval;
    netdata_thread_join(view_thread, &retval);
    netdata_thread_join(master_thread, &retval);

    fprintf(stderr,
            "MASTER: inserts %zu"
            ", deletes %zu"
            ", searches %zu"
            ", resets %zu"
            ", entries %ld"
            ", referenced_items %ld"
            ", pending deletions %ld"
            ", check spins %zu"
            ", insert spins %zu"
            ", search ignores %zu"
            "\n",
            stats_master.ops.inserts,
            stats_master.ops.deletes,
            stats_master.ops.searches,
            stats_master.ops.resets,
            tv.master->entries,
            tv.master->referenced_items,
            tv.master->pending_deletion_items,
            stats_master.spin_locks.use,
            stats_master.spin_locks.insert,
            stats_master.spin_locks.search
    );
    fprintf(stderr,
            "VIEW  : inserts %zu"
            ", deletes %zu"
            ", searches %zu"
            ", resets %zu"
            ", entries %ld"
            ", referenced_items %ld"
            ", pending deletions %ld"
            ", check spins %zu"
            ", insert spins %zu"
            ", search ignores %zu"
            "\n",
            stats_view.ops.inserts,
            stats_view.ops.deletes,
            stats_view.ops.searches,
            stats_view.ops.resets,
            tv.view->entries,
            tv.view->referenced_items,
            tv.view->pending_deletion_items,
            stats_view.spin_locks.use,
            stats_view.spin_locks.insert,
            stats_view.spin_locks.search
    );
    dictionary_destroy(tv.master);
    dictionary_destroy(tv.view);

    return 0;
}

size_t dictionary_unittest_views(void) {
    size_t errors = 0;
    struct dictionary_stats stats = {};
    DICTIONARY *master = dictionary_create_advanced(DICT_OPTION_NONE, &stats);
    DICTIONARY *view = dictionary_create_view(master);

    fprintf(stderr, "\n\nChecking dictionary views...\n");

    // Add an item to both master and view, then remove the view first and the master second
    fprintf(stderr, "\nPASS 1: Adding 1 item to master:\n");
    DICTIONARY_ITEM *item1_on_master = dictionary_set_and_acquire_item(master, "KEY 1", "VALUE1", strlen("VALUE1") + 1);
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 1, 0);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 1: Adding master item to view:\n");
    DICTIONARY_ITEM *item1_on_view = dictionary_view_set_and_acquire_item(view, "KEY 1 ON VIEW", item1_on_master);
    errors += unittest_check_dictionary("view", view, 1, 1, 0, 1, 0);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 1: Deleting view item:\n");
    dictionary_del(view, "KEY 1 ON VIEW");
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 1, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 1, 0);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_DELETED, false, false, true);

    fprintf(stderr, "\nPASS 1: Releasing the deleted view item:\n");
    dictionary_acquired_item_release(view, item1_on_view);
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 1, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 0, 1);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 1: Releasing the acquired master item:\n");
    dictionary_acquired_item_release(master, item1_on_master);
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 0, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 0, 1);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 0, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 1: Deleting the released master item:\n");
    dictionary_del(master, "KEY 1");
    errors += unittest_check_dictionary("master", master, 0, 0, 0, 0, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 0, 1);

    // The other way now:
    // Add an item to both master and view, then remove the master first and verify it is deleted on the view also
    fprintf(stderr, "\nPASS 2: Adding 1 item to master:\n");
    item1_on_master = dictionary_set_and_acquire_item(master, "KEY 1", "VALUE1", strlen("VALUE1") + 1);
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 1, 0);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 2: Adding master item to view:\n");
    item1_on_view = dictionary_view_set_and_acquire_item(view, "KEY 1 ON VIEW", item1_on_master);
    errors += unittest_check_dictionary("view", view, 1, 1, 0, 1, 0);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 2: Deleting master item:\n");
    dictionary_del(master, "KEY 1");
    dictionary_version(view);
    errors += unittest_check_dictionary("master", master, 0, 0, 1, 1, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 1, 0);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_DELETED, false, false, true);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_DELETED, false, false, true);

    fprintf(stderr, "\nPASS 2: Releasing the acquired master item:\n");
    dictionary_acquired_item_release(master, item1_on_master);
    errors += unittest_check_dictionary("master", master, 0, 0, 1, 0, 1);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 1, 0);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_DELETED, false, false, true);

    fprintf(stderr, "\nPASS 2: Releasing the deleted view item:\n");
    dictionary_acquired_item_release(view, item1_on_view);
    errors += unittest_check_dictionary("master", master, 0, 0, 1, 0, 1);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 0, 1);

    dictionary_destroy(master);
    dictionary_destroy(view);
    return errors;
}

int dictionary_unittest(size_t entries) {
    if(entries < 10) entries = 10;

    DICTIONARY *dict;
    size_t errors = 0;

    fprintf(stderr, "Generating %zu names and values...\n", entries);
    char **names = dictionary_unittest_generate_names(entries);
    char **values = dictionary_unittest_generate_values(entries);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_unittest_clone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary multi threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_NONE);
    dictionary_unittest_clone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary single threaded, non-clone, add-in-front options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE |
        DICT_OPTION_ADD_IN_FRONT);
    dictionary_unittest_nonclone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary multi threaded, non-clone, add-in-front options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_ADD_IN_FRONT);
    dictionary_unittest_nonclone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary single-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE |
        DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "resetting non-overwrite entries", names, values, entries, &errors, dictionary_unittest_reset_dont_overwrite_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, &errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, &errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, &errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary multi-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough write delete this", names, values, entries, &errors, dictionary_unittest_walkthrough_delete_this);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary multi-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "foreach write delete this", names, values, entries, &errors, dictionary_unittest_foreach_delete_this);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop empty", names, values, 0, &errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback empty", names, values, 0, &errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_unittest_sorting(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_unittest_null_dfe(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, noclone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_VALUE_LINK_DONT_CLONE);
    dictionary_unittest_null_dfe(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    // check reference counters
    {
        fprintf(stderr, "\nTesting reference counters:\n");
        dict = dictionary_create(DICT_OPTION_NONE | DICT_OPTION_NAME_LINK_DONT_CLONE);
        errors += unittest_check_dictionary("", dict, 0, 0, 0, 0, 0);

        fprintf(stderr, "\nAdding test item to dictionary and acquiring it\n");
        dictionary_set(dict, "test", "ITEM1", 6);
        DICTIONARY_ITEM *item = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");

        errors += unittest_check_dictionary("", dict, 1, 1, 0, 1, 0);
        errors += unittest_check_item("ACQUIRED", dict, item, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nChecking that reference counters are increased:\n");
        void *t;
        dfe_start_read(dict, t) {
            errors += unittest_check_dictionary("", dict, 1, 1, 0, 1, 0);
            errors += unittest_check_item("ACQUIRED TRAVERSAL", dict, item, "test", "ITEM1", 2, ITEM_FLAG_NONE, true, true, true);
        }
        dfe_done(t);

        fprintf(stderr, "\nChecking that reference counters are decreased:\n");
        errors += unittest_check_dictionary("", dict, 1, 1, 0, 1, 0);
        errors += unittest_check_item("ACQUIRED TRAVERSAL 2", dict, item, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDeleting the item we have acquired:\n");
        dictionary_del(dict, "test");

        errors += unittest_check_dictionary("", dict, 0, 0, 1, 1, 0);
        errors += unittest_check_item("DELETED", dict, item, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);

        fprintf(stderr, "\nAdding another item with the same name of the item we deleted, while being acquired:\n");
        dictionary_set(dict, "test", "ITEM2", 6);
        errors += unittest_check_dictionary("", dict, 1, 1, 1, 1, 0);

        fprintf(stderr, "\nAcquiring the second item:\n");
        DICTIONARY_ITEM *item2 = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");
        errors += unittest_check_item("FIRST", dict, item, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);
        errors += unittest_check_item("SECOND", dict, item2, "test", "ITEM2", 1, ITEM_FLAG_NONE, true, true, true);
        errors += unittest_check_dictionary("", dict, 1, 1, 1, 2, 0);

        fprintf(stderr, "\nReleasing the second item (the first is still acquired):\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)item2);
        errors += unittest_check_dictionary("", dict, 1, 1, 1, 1, 0);
        errors += unittest_check_item("FIRST", dict, item, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);
        errors += unittest_check_item("SECOND RELEASED", dict, item2, "test", "ITEM2", 0, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDeleting the second item (the first is still acquired):\n");
        dictionary_del(dict, "test");
        errors += unittest_check_dictionary("", dict, 0, 0, 1, 1, 0);
        errors += unittest_check_item("ACQUIRED DELETED", dict, item, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);

        fprintf(stderr, "\nReleasing the first item (which we have already deleted):\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)item);
        dfe_start_write(dict, item) ; dfe_done(item);
        errors += unittest_check_dictionary("", dict, 0, 0, 1, 0, 1);

        fprintf(stderr, "\nAdding again the test item to dictionary and acquiring it\n");
        dictionary_set(dict, "test", "ITEM1", 6);
        item = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");

        errors += unittest_check_dictionary("", dict, 1, 1, 0, 1, 0);
        errors += unittest_check_item("RE-ADDITION", dict, item, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDestroying the dictionary while we have acquired an item\n");
        dictionary_destroy(dict);

        fprintf(stderr, "Releasing the item (on a destroyed dictionary)\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)item);
        item = NULL;
        dict = NULL;
    }

    dictionary_unittest_free_char_pp(names, entries);
    dictionary_unittest_free_char_pp(values, entries);

    errors += dictionary_unittest_views();
    errors += dictionary_unittest_threads();
    errors += dictionary_unittest_view_threads();

    fprintf(stderr, "\n%zu errors found\n", errors);
    return  errors ? 1 : 0;
}
