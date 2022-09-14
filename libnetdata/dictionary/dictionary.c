// SPDX-License-Identifier: GPL-3.0-or-later

#define DICTIONARY_INTERNALS

#include "../libnetdata.h"
#include <Judy.h>

// runtime options
typedef enum {
    DICT_FLAG_EXCLUSIVE_ACCESS      = (1 << 0), // there is only one thread accessing the dictionary
    DICT_FLAG_DESTROYED             = (1 << 1), // this dictionary has been destroyed
    DICT_FLAG_DEFER_ALL_DELETIONS   = (1 << 2), // defer all deletions of items in the dictionary
    DICT_FLAG_MASTER_DICT           = (1 << 3), // this is the master dictionary
} DICT_FLAGS;

#define dict_flag_check(dict, flag) (__atomic_load_n(&((dict)->flags), __ATOMIC_SEQ_CST) & (flag))
#define dict_flag_set(dict, flag)   __atomic_or_fetch(&((dict)->flags), flag, __ATOMIC_SEQ_CST)
#define dict_flag_clear(dict, flag) __atomic_and_fetch(&((dict)->flags), ~(flag), __ATOMIC_SEQ_CST)


// flags macros
#define has_caller_exclusive_lock(dict) ((dict->options & DICT_OPTION_SINGLE_THREADED) || dict_flag_check(dict, DICT_FLAG_EXCLUSIVE_ACCESS))
#define is_dictionary_destroyed(dict) dict_flag_check(dict, DICT_FLAG_DESTROYED)
#define are_deletions_deferred(dict) dict_flag_check(dict, DICT_FLAG_DEFER_ALL_DELETIONS)
#define is_master_dictionary(dict) dict_flag_check(dict, DICT_FLAG_MASTER_DICT)

// configuration options macros
#define is_dictionary_single_threaded(dict) ((dict)->options & DICT_OPTION_SINGLE_THREADED)

typedef enum item_flags {
    ITEM_FLAG_NONE              = 0,
    ITEM_FLAG_NAME_IS_ALLOCATED = (1 << 0), // the name pointer is a STRING
    ITEM_FLAG_DELETED           = (1 << 1), // this item is deleted, so it is not available for traversal
    ITEM_FLAG_NEW_OR_UPDATED    = (1 << 2), // this item is new or just updated (used by the react callback)

    // IMPORTANT: This is 8-bit
} ITEM_FLAGS;

/*
 * Every item in the dictionary has the following structure.
 */

typedef struct dictionary_item_shared {
    int32_t refcount;       // the reference counter
    void *value;            // the value of the dictionary item
    uint32_t value_len;     // the size of the value (assumed binary)
} DICTIONARY_ITEM_SHARED;

struct dictionary_item {
#ifdef NETDATA_INTERNAL_CHECKS
    DICTIONARY *dict;
#endif

    DICTIONARY_ITEM_SHARED *shared;

    struct dictionary_item *next;    // a double linked list to allow fast insertions and deletions
    struct dictionary_item *prev;

    union {
        STRING *string_name;    // the name of the dictionary item
        char *caller_name;      // the user supplied string pointer
    };

    uint8_t flags;              // the flags for this item
};

typedef struct dictionary_item_master_dict {
    struct dictionary_item item;            // this must be first
    struct dictionary_item_shared shared;
} DICTIONARY_ITEM_MASTER_DICT;

struct dictionary_hooks {
    void (*ins_callback)(const DICTIONARY_ITEM *item, void *value, void *data);
    void *ins_callback_data;

    void (*react_callback)(const DICTIONARY_ITEM *item, void *value, void *data);
    void *react_callback_data;

    void (*del_callback)(const DICTIONARY_ITEM *item, void *value, void *data);
    void *del_callback_data;

    void (*conflict_callback)(const DICTIONARY_ITEM *item, void *old_value, void *new_value, void *data);
    void *conflict_callback_data;
};

struct dictionary_stats {
    size_t inserts;                     // how many index insertions have been performed
    size_t deletes;                     // how many index deletions have been performed
    size_t searches;                    // how many index searches have been performed
    size_t resets;                      // how many times items have reset their values
    long int memory;                    // how much memory the dictionary has currently allocated
    size_t walkthroughs;                // how many walkthroughs have been done
    int readers;                        // how many readers are currently using the dictionary
    int writers;                        // how many writers are currently using the dictionary
};

struct dictionary_shared {
    netdata_rwlock_t rwlock;            // the r/w lock when DICT_OPTION_SINGLE_THREADED is not set
    pid_t writer_pid;                   // the gettid() of the writer
    size_t writer_depth;                 // nesting of write locks
};

struct dictionary {
#ifdef NETDATA_INTERNAL_CHECKS
    const char *creation_function;
    const char *creation_file;
    size_t creation_line;
#endif

    DICT_OPTIONS options;               // the configuration flags of the dictionary
    DICT_FLAGS flags;                   // run time flags for the dictionary

    union {                             // support for multiple indexing engines
        Pvoid_t JudyHSArray;            // the hash table
    } index;

    DICTIONARY_ITEM *items;             // the double linked list of all items in the dictionary

    struct dictionary_shared *shared;   // structures to be shared among multiple dictionaries (views) - exists always
    struct dictionary_hooks *hooks;     // pointer to external function callbacks to be called at certain points
    struct dictionary_stats *stats;     // statistics data, when DICT_OPTION_STATS is set

    DICTIONARY *next;                   // linked list for delayed destruction (garbage collection of whole dictionaries)

    size_t version;                     // the current version of the dictionary
                                        // it is incremented when:
                                        //   - item added
                                        //   - item removed
                                        //   - item value reset
                                        //   - function dictionary_version_increment() is called

    long int entries;                   // how many items are currently in the index (the linked list may have more)
    long int referenced_items;          // how many items of the dictionary are currently being used by 3rd parties
    long int pending_deletion_items;    // how many items of the dictionary have been deleted, but have not been removed yet
};

static inline void item_linked_list_unlink_unsafe(DICTIONARY *dict, DICTIONARY_ITEM *nv);
static size_t item_destroy_unsafe(DICTIONARY *dict, DICTIONARY_ITEM *nv);
static inline const char *item_get_name(const DICTIONARY_ITEM *nv);
static bool item_can_be_deleted(DICTIONARY *dict, DICTIONARY_ITEM *nv);

// ----------------------------------------------------------------------------
// callbacks registration

static inline void dictionary_allocate_hooks(DICTIONARY *dict) {
    if(dict->hooks) return;
    dict->hooks = callocz(1, sizeof(struct dictionary_hooks));
}

void dictionary_register_insert_callback(DICTIONARY *dict, void (*ins_callback)(const DICTIONARY_ITEM *item, void *value, void *data), void *data) {
    dictionary_allocate_hooks(dict);
    dict->hooks->ins_callback = ins_callback;
    dict->hooks->ins_callback_data = data;
}

void dictionary_register_delete_callback(DICTIONARY *dict, void (*del_callback)(const DICTIONARY_ITEM *item, void *value, void *data), void *data) {
    dictionary_allocate_hooks(dict);
    dict->hooks->del_callback = del_callback;
    dict->hooks->del_callback_data = data;
}

void dictionary_register_conflict_callback(DICTIONARY *dict, void (*conflict_callback)(const DICTIONARY_ITEM *item, void *old_value, void *new_value, void *data), void *data) {
    dictionary_allocate_hooks(dict);
    dict->hooks->conflict_callback = conflict_callback;
    dict->hooks->conflict_callback_data = data;
}

void dictionary_register_react_callback(DICTIONARY *dict, void (*react_callback)(const DICTIONARY_ITEM *item, void *value, void *data), void *data) {
    dictionary_allocate_hooks(dict);
    dict->hooks->react_callback = react_callback;
    dict->hooks->react_callback_data = data;
}

// ----------------------------------------------------------------------------
// dictionary statistics maintenance

size_t dictionary_version(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;
    return dict->version;
}
long int dictionary_entries(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;
    return dict->entries;
}
size_t dictionary_referenced_items(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;
    return __atomic_load_n(&dict->referenced_items, __ATOMIC_SEQ_CST);
}

long int dictionary_stats_allocated_memory(DICTIONARY *dict) {
    if(unlikely(!dict || !dict->stats)) return 0;
    return dict->stats->memory;
}
size_t dictionary_stats_searches(DICTIONARY *dict) {
    if(unlikely(!dict || !dict->stats)) return 0;
    return dict->stats->searches;
}
size_t dictionary_stats_inserts(DICTIONARY *dict) {
    if(unlikely(!dict || !dict->stats)) return 0;
    return dict->stats->inserts;
}
size_t dictionary_stats_deletes(DICTIONARY *dict) {
    if(unlikely(!dict || !dict->stats)) return 0;
    return dict->stats->deletes;
}
size_t dictionary_stats_resets(DICTIONARY *dict) {
    if(unlikely(!dict || !dict->stats)) return 0;
    return dict->stats->resets;
}
size_t dictionary_stats_walkthroughs(DICTIONARY *dict) {
    if(unlikely(!dict || !dict->stats)) return 0;
    return dict->stats->walkthroughs;
}

void dictionary_version_increment(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->version, 1, __ATOMIC_SEQ_CST);
}

static inline void DICTIONARY_STATS_SEARCHES_PLUS1(DICTIONARY *dict) {
    if(!dict->stats) return;

    if(has_caller_exclusive_lock(dict)) {
        dict->stats->searches++;
    }
    else {
        __atomic_fetch_add(&dict->stats->searches, 1, __ATOMIC_RELAXED);
    }
}
static inline void DICTIONARY_ENTRIES_PLUS1(DICTIONARY *dict, size_t size) {
    if(has_caller_exclusive_lock(dict)) {
        dict->version++;
        dict->entries++;

        if(dict->stats) {
            dict->stats->inserts++;
            dict->stats->memory += (long)size;
        }
    }
    else {
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_SEQ_CST);
        __atomic_fetch_add(&dict->entries, 1, __ATOMIC_SEQ_CST);

        if(dict->stats) {
            __atomic_fetch_add(&dict->stats->inserts, 1, __ATOMIC_RELAXED);
            __atomic_fetch_add(&dict->stats->memory, (long)size, __ATOMIC_RELAXED);
        }
    }
}
static inline void DICTIONARY_ENTRIES_MINUS1(DICTIONARY *dict) {
    if(has_caller_exclusive_lock(dict)) {
        dict->version++;
        dict->entries--;

        if(dict->stats) {
            dict->stats->deletes++;
        }
    }
    else {
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_SEQ_CST);
        __atomic_fetch_sub(&dict->entries, 1, __ATOMIC_SEQ_CST);

        if(dict->stats) {
            __atomic_fetch_add(&dict->stats->deletes, 1, __ATOMIC_RELAXED);
        }
    }
}
static inline void DICTIONARY_STATS_ENTRIES_MINUS_MEMORY(DICTIONARY *dict, size_t size) {
    if(!dict->stats) return;

    if(has_caller_exclusive_lock(dict)) {
        dict->stats->memory -= (long)size;
    }
    else {
        __atomic_fetch_sub(&dict->stats->memory, (long)size, __ATOMIC_RELAXED);
    }
}
static inline void DICTIONARY_VALUE_RESETS_PLUS1(DICTIONARY *dict, size_t oldsize, size_t newsize) {
    if(has_caller_exclusive_lock(dict)) {
        dict->version++;

        if(dict->stats) {
            dict->stats->resets++;
            dict->stats->memory += (long)newsize;
            dict->stats->memory -= (long)oldsize;
        }
    }
    else {
        __atomic_fetch_add(&dict->version, 1, __ATOMIC_SEQ_CST);

        if(dict->stats) {
            __atomic_fetch_add(&dict->stats->resets, 1, __ATOMIC_RELAXED);
            __atomic_fetch_add(&dict->stats->memory, (long)newsize, __ATOMIC_RELAXED);
            __atomic_fetch_sub(&dict->stats->memory, (long)oldsize, __ATOMIC_RELAXED);
        }
    }
}
static inline void DICTIONARY_STATS_WALKTHROUGHS_PLUS1(DICTIONARY *dict) {
    if(!dict->stats) return;

    if(has_caller_exclusive_lock(dict)) {
        dict->stats->walkthroughs++;
    }
    else {
        __atomic_fetch_add(&dict->stats->walkthroughs, 1, __ATOMIC_RELAXED);
    }
}

static inline size_t DICTIONARY_STATS_REFERENCED_ITEMS_PLUS1(DICTIONARY *dict) {
    return __atomic_add_fetch(&dict->referenced_items, 1, __ATOMIC_SEQ_CST);
}

static inline size_t DICTIONARY_STATS_REFERENCED_ITEMS_MINUS1(DICTIONARY *dict) {
    return __atomic_sub_fetch(&dict->referenced_items, 1, __ATOMIC_SEQ_CST);
}

static inline size_t DICTIONARY_PENDING_DELETES_PLUS1(DICTIONARY *dict) {
    return __atomic_add_fetch(&dict->pending_deletion_items, 1, __ATOMIC_SEQ_CST);
}

static inline size_t DICTIONARY_PENDING_DELETES_MINUS1(DICTIONARY *dict) {
    return __atomic_sub_fetch(&dict->pending_deletion_items, 1, __ATOMIC_SEQ_CST);
}

static inline size_t DICTIONARY_STATS_PENDING_DELETES_GET(DICTIONARY *dict) {
    return __atomic_load_n(&dict->pending_deletion_items, __ATOMIC_SEQ_CST);
}

static inline int DICTIONARY_ITEM_REFCOUNT_GET(DICTIONARY_ITEM *nv) {
    return (int)__atomic_load_n(&nv->shared->refcount, __ATOMIC_SEQ_CST);
}

// ----------------------------------------------------------------------------
// garbage collector
// it is called every time someone gets a write lock to the dictionary

static void garbage_collect_pending_deletes_unsafe(DICTIONARY *dict) {
    if(!has_caller_exclusive_lock(dict)) return;

    if(likely(!DICTIONARY_STATS_PENDING_DELETES_GET(dict))) return;

    DICTIONARY_ITEM *nv = dict->items;
    while(nv) {
        if((nv->flags & ITEM_FLAG_DELETED) && item_can_be_deleted(dict, nv)) {
            DICTIONARY_ITEM *nv_next = nv->next;

            item_linked_list_unlink_unsafe(dict, nv);
            item_destroy_unsafe(dict, nv);

            size_t pending = DICTIONARY_PENDING_DELETES_MINUS1(dict);
            if(!pending) break;

            nv = nv_next;
        }
        else
            nv = nv->next;
    }
}

// ----------------------------------------------------------------------------
// dictionary locks

static inline size_t dictionary_lock_init(DICTIONARY *dict) {
    if(likely(!is_dictionary_single_threaded(dict))) {
        netdata_rwlock_init(&dict->shared->rwlock);

        if(has_caller_exclusive_lock(dict))
            dict_flag_clear(dict, DICT_FLAG_EXCLUSIVE_ACCESS);

        return 0;
    }

    // we are single threaded
    dict_flag_set(dict, DICT_FLAG_EXCLUSIVE_ACCESS);
    return 0;
}

static inline size_t dictionary_lock_free(DICTIONARY *dict) {
    if(likely(!is_dictionary_single_threaded(dict))) {
        netdata_rwlock_destroy(&dict->shared->rwlock);
        return 0;
    }
    return 0;
}

static inline void dictionary_lock_set_thread_as_writer(DICTIONARY *dict) {
    pid_t expected = 0, desired = gettid();
    if(!__atomic_compare_exchange_n(&dict->shared->writer_pid, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST))
        fatal("DICTIONARY: Cannot set thread %d as exclusive writer, expected %d, desired %d, found %d.", gettid(), expected, desired, __atomic_load_n(&dict->shared->writer_pid, __ATOMIC_SEQ_CST));
}

static inline void dictionary_unlock_unset_thread_writer(DICTIONARY *dict) {
    pid_t expected = gettid(), desired = 0;
    if(!__atomic_compare_exchange_n(&dict->shared->writer_pid, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST))
        fatal("DICTIONARY: Cannot unset thread %d as exclusive writer, expected %d, desired %d, found %d.", gettid(), expected, desired, __atomic_load_n(&dict->shared->writer_pid, __ATOMIC_SEQ_CST));
}

static inline bool dictionary_lock_is_thread_the_writer(DICTIONARY *dict) {
    pid_t tid = gettid();
    return tid > 0 && tid == __atomic_load_n(&dict->shared->writer_pid, __ATOMIC_SEQ_CST);
}

static void dictionary_lock(DICTIONARY *dict, char rw) {
    if(dictionary_lock_is_thread_the_writer(dict)) {
        dict->shared->writer_depth++;
        return;
    }

    if(rw == DICTIONARY_LOCK_READ || rw == DICTIONARY_LOCK_REENTRANT || rw == 'R') {
        // read lock

        if(dict->stats)
            __atomic_add_fetch(&dict->stats->readers, 1, __ATOMIC_RELAXED);
    }
    else {
        // write lock

        if(dict->stats)
            __atomic_add_fetch(&dict->stats->writers, 1, __ATOMIC_RELAXED);
    }

    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    if(rw == DICTIONARY_LOCK_READ || rw == DICTIONARY_LOCK_REENTRANT || rw == 'R') {
        // read lock
        netdata_rwlock_rdlock(&dict->shared->rwlock);

        if(has_caller_exclusive_lock(dict)) {
            internal_error(true, "DICTIONARY: left-over exclusive access to dictionary created by %s (%zu@%s) found", dict->creation_function, dict->creation_line, dict->creation_file);
            dict_flag_clear(dict, DICT_FLAG_EXCLUSIVE_ACCESS);
        }
    }
    else {
        // write lock
        netdata_rwlock_wrlock(&dict->shared->rwlock);

        dictionary_lock_set_thread_as_writer(dict);

        dict_flag_set(dict, DICT_FLAG_EXCLUSIVE_ACCESS);
    }
}

static void dictionary_unlock(DICTIONARY *dict, char rw) {
    if(dictionary_lock_is_thread_the_writer(dict) && dict->shared->writer_depth > 0) {
        dict->shared->writer_depth--;
        return;
    }

    if(rw == DICTIONARY_LOCK_READ || rw == DICTIONARY_LOCK_REENTRANT || rw == 'R') {
        // read unlock

        if(dict->stats)
            __atomic_sub_fetch(&dict->stats->readers, 1, __ATOMIC_RELAXED);
    }
    else {
        // write unlock

        garbage_collect_pending_deletes_unsafe(dict);

        if(dict->stats)
            __atomic_sub_fetch(&dict->stats->writers, 1, __ATOMIC_RELAXED);
    }

    if(unlikely(is_dictionary_single_threaded(dict)))
        return;

    if(rw == DICTIONARY_LOCK_READ || rw == DICTIONARY_LOCK_REENTRANT || rw == 'R') {
        // read unlock
        ;
    }
    else {
        // write unlock
        ;
        dictionary_unlock_unset_thread_writer(dict);
    }

    if(unlikely(has_caller_exclusive_lock(dict)))
        dict_flag_clear(dict, DICT_FLAG_EXCLUSIVE_ACCESS);

    netdata_rwlock_unlock(&dict->shared->rwlock);
}

// ----------------------------------------------------------------------------
// deferred deletions

void dictionary_defer_all_deletions_unsafe(DICTIONARY *dict, char rw) {
    if(rw == 'r' || rw == 'R') {
        // read locked - no need to defer deletions
        ;
    }
    else {
        // write locked - defer deletions
        dict_flag_set(dict, DICT_FLAG_DEFER_ALL_DELETIONS);
    }
}

void dictionary_restore_all_deletions_unsafe(DICTIONARY *dict, char rw) {
    if(rw == 'r' || rw == 'R') {
        // read locked - no need to defer deletions
        internal_error(are_deletions_deferred(dict), "DICTIONARY: deletions are deferred on a read lock");
    }
    else {
        // write locked - defer deletions
        if(are_deletions_deferred(dict))
            dict_flag_clear(dict, DICT_FLAG_DEFER_ALL_DELETIONS);
    }
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

static void reference_counter_acquire(DICTIONARY *dict, DICTIONARY_ITEM *nv) {
    int32_t refcount;

    if(likely(is_dictionary_single_threaded(dict)))
        refcount = ++nv->shared->refcount;
    else
        refcount = __atomic_add_fetch(&nv->shared->refcount, 1, __ATOMIC_SEQ_CST);

    if(refcount <= 0)
        fatal("DICTIONARY: request to acquire item '%s', which is deleted!", item_get_name(nv));

    if(refcount == 1) {
        // referenced items counts number of unique items referenced
        // so, we increase it only when refcount == 1
        DICTIONARY_STATS_REFERENCED_ITEMS_PLUS1(dict);

        // if this is a deleted item, but the counter increased to 1
        // we need to remove it from the pending items to delete
        if (nv->flags & ITEM_FLAG_DELETED)
            DICTIONARY_PENDING_DELETES_MINUS1(dict);
    }
}

static void reference_counter_release(DICTIONARY *dict, DICTIONARY_ITEM *nv, bool can_get_write_lock) {
    // this function may be called without any lock on the dictionary
    // or even when someone else has a write lock on the dictionary
    // so, we cannot check for EXCLUSIVE ACCESS

    int32_t refcount;
    if(likely(is_dictionary_single_threaded(dict)))
        refcount = nv->shared->refcount--;
    else
        refcount = __atomic_sub_fetch(&nv->shared->refcount, 1, __ATOMIC_SEQ_CST);

    if(refcount < 0) {
        internal_error(true, "DICTIONARY: attempted to release item without references: '%s' on dictionary created by %s() (%zu@%s)",
            item_get_name(nv), dict->creation_function, dict->creation_line, dict->creation_file);

        fatal("DICTIONARY: attempted to release item without references: '%s'", item_get_name(nv));
    }

    if(refcount == 0) {
        // referenced items counts number of unique items referenced
        // so, we decrease it only when refcount == 0
        DICTIONARY_STATS_REFERENCED_ITEMS_MINUS1(dict);
    }

    if(can_get_write_lock && DICTIONARY_STATS_PENDING_DELETES_GET(dict)) {
        // we can garbage collect now

        dictionary_lock(dict, DICTIONARY_LOCK_WRITE);
        garbage_collect_pending_deletes_unsafe(dict);
        dictionary_unlock(dict, DICTIONARY_LOCK_WRITE);
    }
}

// if a dictionary item can be deleted, return true, otherwise return false
static bool item_can_be_deleted(DICTIONARY *dict, DICTIONARY_ITEM *nv) {
    if(unlikely(are_deletions_deferred(dict)))
        return false;

    int32_t expected = DICTIONARY_ITEM_REFCOUNT_GET(nv);

    if(expected == 0 && __atomic_compare_exchange_n(&nv->shared->refcount, &expected, -1, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST))
        return true;

    return false;
}

static bool item_found_in_hashtable_can_be_used(DICTIONARY_ITEM *nv) {
    return nv && !(nv->flags & ITEM_FLAG_DELETED) && DICTIONARY_ITEM_REFCOUNT_GET(nv) >= 0;
}


// ----------------------------------------------------------------------------
// hash table

static void hashtable_init_unsafe(DICTIONARY *dict) {
    dict->index.JudyHSArray = NULL;
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
    internal_error(!has_caller_exclusive_lock(dict),
                   "DICTIONARY: inserting item to the index without exclusive access to the dictionary created by %s() (%zu@%s)",
                   dict->creation_function, dict->creation_line, dict->creation_file);

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

static inline int hashtable_delete_unsafe(DICTIONARY *dict, const char *name, size_t name_len, void *nv) {
    internal_error(!has_caller_exclusive_lock(dict),
                   "DICTIONARY: deleting item from the index without exclusive access to the dictionary created by %s() (%zu@%s)",
                   dict->creation_function, dict->creation_line, dict->creation_file);

    (void)nv;
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

static inline void hashtable_inserted_item_unsafe(DICTIONARY *dict, void *nv) {
    (void)dict;
    (void)nv;

    // this is called just after an item is successfully inserted to the hashtable
    // we don't need this for judy, but we may need it if we integrate more hash tables

    ;
}

// ----------------------------------------------------------------------------
// linked list management

static inline void item_linked_list_link_unsafe(DICTIONARY *dict, DICTIONARY_ITEM *nv) {
    internal_error(!has_caller_exclusive_lock(dict),
                   "DICTIONARY: adding item to the linked-list without exclusive access to the dictionary created by %s() (%zu@%s)",
                   dict->creation_function, dict->creation_line, dict->creation_file);

    if(dict->options & DICT_OPTION_ADD_IN_FRONT)
        DOUBLE_LINKED_LIST_PREPEND_UNSAFE(dict->items, nv, prev, next);
    else
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(dict->items, nv, prev, next);
}

static inline void item_linked_list_unlink_unsafe(DICTIONARY *dict, DICTIONARY_ITEM *nv) {
    internal_error(!has_caller_exclusive_lock(dict),
                   "DICTIONARY: removing item from the linked-list without exclusive access to the dictionary created by %s() (%zu@%s)",
                   dict->creation_function, dict->creation_line, dict->creation_file);

    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(dict->items, nv, prev, next);
}

// ----------------------------------------------------------------------------
// ITEM methods

static inline size_t item_set_name(DICTIONARY *dict, DICTIONARY_ITEM *nv, const char *name, size_t name_len) {
    if(likely(dict->options & DICT_OPTION_NAME_LINK_DONT_CLONE)) {
        nv->caller_name = (char *)name;
        return 0;
    }

    nv->string_name = string_strdupz(name);
    nv->flags |= ITEM_FLAG_NAME_IS_ALLOCATED;
    return name_len;
}

static inline size_t item_free_name(DICTIONARY *dict, DICTIONARY_ITEM *nv) {
    if(unlikely(!(dict->options & DICT_OPTION_NAME_LINK_DONT_CLONE)))
        string_freez(nv->string_name);

    return 0;
}

static inline const char *item_get_name(const DICTIONARY_ITEM *nv) {
    if(nv->flags & ITEM_FLAG_NAME_IS_ALLOCATED)
        return string2str(nv->string_name);
    else
        return nv->caller_name;
}

static DICTIONARY_ITEM *item_allocate(DICTIONARY *dict, size_t *allocated_bytes) {
    if(is_master_dictionary(dict)) {
        size_t size = sizeof(DICTIONARY_ITEM_MASTER_DICT);
        DICTIONARY_ITEM_MASTER_DICT *nv = mallocz(size);
        nv->item.shared = &nv->shared;
        *allocated_bytes = size;

        return (DICTIONARY_ITEM *)nv;
    }
    else {
        size_t size = sizeof(DICTIONARY_ITEM);
        DICTIONARY_ITEM *nv = mallocz(size);
        *allocated_bytes = size;

        size = sizeof(DICTIONARY_ITEM_SHARED);
        nv->shared = mallocz(size);
        *allocated_bytes += size;
        return nv;
    }
}

static DICTIONARY_ITEM *item_create_unsafe(DICTIONARY *dict, const char *name, size_t name_len, void *value, size_t value_len, void *constructor_data) {
    debug(D_DICTIONARY, "Creating name value entry for name '%s'.", name);

    size_t allocated;
    DICTIONARY_ITEM *nv = item_allocate(dict, &allocated);

#ifdef NETDATA_INTERNAL_CHECKS
    nv->dict = dict;
#endif

    nv->shared->refcount = 0;
    nv->flags = ITEM_FLAG_NONE;
    nv->shared->value_len = value_len;

    allocated += item_set_name(dict, nv, name, name_len);

    if(likely(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE))
        nv->shared->value = value;
    else {
        if(likely(value_len)) {
            if (value) {
                // a value has been supplied
                // copy it
                nv->shared->value = mallocz(value_len);
                memcpy(nv->shared->value, value, value_len);
            }
            else {
                // no value has been supplied
                // allocate a clear memory block
                nv->shared->value = callocz(1, value_len);
            }
        }
        else {
            // the caller wants an item without any value
            nv->shared->value = NULL;
        }

        allocated += value_len;
    }

    DICTIONARY_ENTRIES_PLUS1(dict, allocated);

    if(dict->hooks && dict->hooks->ins_callback)
        dict->hooks->ins_callback(nv, nv->shared->value, constructor_data?constructor_data:dict->hooks->ins_callback_data);

    return nv;
}

static void item_reset_unsafe(DICTIONARY *dict, DICTIONARY_ITEM *nv, void *value, size_t value_len) {
    debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", item_get_name(nv));

    DICTIONARY_VALUE_RESETS_PLUS1(dict, nv->shared->value_len, value_len);

    if(dict->hooks && dict->hooks->del_callback)
        dict->hooks->del_callback(nv, nv->shared->value, dict->hooks->del_callback_data);

    if(likely(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE)) {
        debug(D_DICTIONARY, "Dictionary: linking value to '%s'", item_get_name(nv));
        nv->shared->value = value;
        nv->shared->value_len = value_len;
    }
    else {
        debug(D_DICTIONARY, "Dictionary: cloning value to '%s'", item_get_name(nv));

        void *oldvalue = nv->shared->value;
        void *newvalue = NULL;
        if(value_len) {
            newvalue = mallocz(value_len);
            if(value) memcpy(newvalue, value, value_len);
            else memset(newvalue, 0, value_len);
        }
        nv->shared->value = newvalue;
        nv->shared->value_len = value_len;

        debug(D_DICTIONARY, "Dictionary: freeing old value of '%s'", item_get_name(nv));
        freez(oldvalue);
    }

    if(dict->hooks && dict->hooks->ins_callback)
        dict->hooks->ins_callback(nv, nv->shared->value, dict->hooks->ins_callback_data);
}

static size_t item_destroy_unsafe(DICTIONARY *dict, DICTIONARY_ITEM *nv) {
    debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", item_get_name(nv));

    if(dict->hooks && dict->hooks->del_callback)
        dict->hooks->del_callback(nv, nv->shared->value, dict->hooks->del_callback_data);

    size_t freed = 0;

    if(unlikely(!(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE))) {
        debug(D_DICTIONARY, "Dictionary freeing value of '%s'", item_get_name(nv));
        freez(nv->shared->value);
        freed += nv->shared->value_len;
    }

    if(unlikely(!(dict->options & DICT_OPTION_NAME_LINK_DONT_CLONE))) {
        debug(D_DICTIONARY, "Dictionary freeing name '%s'", item_get_name(nv));
        freed += item_free_name(dict, nv);
    }

    if(!is_master_dictionary(dict)) {
        freez(nv->shared);
        freed += sizeof(DICTIONARY_ITEM_SHARED);

        freez(nv);
        freed += sizeof(DICTIONARY_ITEM);
    }
    else {
        freez(nv);
        freed += sizeof(DICTIONARY_ITEM_MASTER_DICT);
    }

    DICTIONARY_STATS_ENTRIES_MINUS_MEMORY(dict, freed);

    return freed;
}

static inline void item_delete_or_mark_deleted_unsafe(DICTIONARY *dict, DICTIONARY_ITEM *nv) {
    if(item_can_be_deleted(dict, nv)) {
        item_linked_list_unlink_unsafe(dict, nv);
        item_destroy_unsafe(dict, nv);
    }
    else {
        nv->flags |= ITEM_FLAG_DELETED;
        DICTIONARY_PENDING_DELETES_PLUS1(dict);
    }

    DICTIONARY_ENTRIES_MINUS1(dict);
}

// ----------------------------------------------------------------------------
// delayed destruction of dictionaries

static bool dictionary_free_all_resources(DICTIONARY *dict, size_t *mem) {
    if(mem)
        *mem = 0;

    if(dictionary_referenced_items(dict))
        return false;

    dictionary_lock(dict, DICTIONARY_LOCK_WRITE);

    size_t freed = 0;

    // destroy the index
    freed += hashtable_destroy_unsafe(dict);

    if(dict->stats) {
        freed += sizeof(struct dictionary_stats);
        freez(dict->stats);
        dict->stats = NULL;
    }

    if(dict->hooks) {
        freed += sizeof(struct dictionary_hooks);
        freez(dict->hooks);
        dict->hooks = NULL;
    }

    DICTIONARY_ITEM *nv = dict->items;
    while (nv) {
        // cache nv->next
        // because we are going to free nv
        DICTIONARY_ITEM *nv_next = nv->next;
        freed += item_destroy_unsafe(dict, nv);
        nv = nv_next;
        // to speed up destruction, we don't
        // unlink nv from the linked-list here
    }

    dict->items = NULL;

    dictionary_unlock(dict, DICTIONARY_LOCK_WRITE);

    freed += dictionary_lock_free(dict);
    freed += reference_counter_free(dict);

    // this has the rwlock in it, so free it after the dictionary is unlocked
    freed += sizeof(struct dictionary_shared);
    freez(dict->shared);
    dict->shared = NULL;

    freed += sizeof(DICTIONARY);
    freez(dict);

    if(mem) *mem = freed;

    return true;
}

netdata_mutex_t dictionaries_waiting_to_be_destroyed_mutex = NETDATA_MUTEX_INITIALIZER;
static DICTIONARY *dictionaries_waiting_to_be_destroyed = NULL;

void dictionary_queue_for_destruction_unsafe(DICTIONARY *dict) {
    if(is_dictionary_destroyed(dict))
        return;

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

        if(dictionary_free_all_resources(dict, NULL)) {
            internal_error(
                true,
                "DICTIONARY: cleaned up dictionary with delayed destruction, created from %s() %zu@%s.",
                function, line, file);

            if(last) last->next = next;
            else dictionaries_waiting_to_be_destroyed = next;
        }
        else
            last = dict;
    }

    netdata_mutex_unlock(&dictionaries_waiting_to_be_destroyed_mutex);
}

// ----------------------------------------------------------------------------
// API - dictionary management

#ifdef NETDATA_INTERNAL_CHECKS
DICTIONARY *dictionary_create_advanced_with_trace(DICT_OPTIONS options, const char *function, size_t line, const char *file) {
#else
DICTIONARY *dictionary_create_advanced(DICT_OPTIONS options) {
#endif
    cleanup_destroyed_dictionaries();

    debug(D_DICTIONARY, "Creating dictionary.");

    DICTIONARY *dict = callocz(1, sizeof(DICTIONARY));
    size_t allocated = sizeof(DICTIONARY);

    dict->shared = callocz(1, sizeof(struct dictionary_shared));
    allocated += sizeof(struct dictionary_shared);

    dict->options = options;
    dict->flags = DICT_FLAG_MASTER_DICT;
    dict->items = NULL;

    if(dict->options & DICT_OPTION_STATS)
        dict->stats = callocz(1, sizeof(struct dictionary_stats));

    allocated += dictionary_lock_init(dict);
    allocated += reference_counter_init(dict);

    if(dict->stats)
        dict->stats->memory = (long)allocated;

    hashtable_init_unsafe(dict);

#ifdef NETDATA_INTERNAL_CHECKS
    dict->creation_function = function;
    dict->creation_file = file;
    dict->creation_line = line;
#endif

    return dict;
}

static void dictionary_flush_unsafe(DICTIONARY *dict) {
    // delete the index
    hashtable_destroy_unsafe(dict);

    // delete all items
    DICTIONARY_ITEM *nv, *nv_next;
    for (nv = dict->items; nv; nv = nv_next) {
        nv_next = nv->next;

        if(!(nv->flags & ITEM_FLAG_DELETED))
            item_delete_or_mark_deleted_unsafe(dict, nv);
    }
}

void dictionary_flush(DICTIONARY *dict) {
    if(unlikely(!dict)) return;
    dictionary_lock(dict, DICTIONARY_LOCK_WRITE);
    dictionary_flush_unsafe(dict);
    dictionary_unlock(dict, DICTIONARY_LOCK_WRITE);
}

size_t dictionary_destroy(DICTIONARY *dict) {
    cleanup_destroyed_dictionaries();

    if(!dict) return 0;

    debug(D_DICTIONARY, "Destroying dictionary.");

    size_t referenced_items = dictionary_referenced_items(dict);
    if(referenced_items) {
        dictionary_lock(dict, DICTIONARY_LOCK_WRITE);

        dictionary_flush_unsafe(dict);
        dictionary_queue_for_destruction_unsafe(dict);

        internal_error(
            true,
            "DICTIONARY: delaying destruction of dictionary created from %s() %zu@%s, because it has %zu referenced items in it (%ld total).",
            dict->creation_function,
            dict->creation_line,
            dict->creation_file,
            dictionary_referenced_items(dict),
            dictionary_entries(dict));

        dictionary_unlock(dict, DICTIONARY_LOCK_WRITE);
        return 0;
    }

    size_t freed;
    dictionary_free_all_resources(dict, &freed);

    return freed;
}

// ----------------------------------------------------------------------------
// helpers

static DICTIONARY_ITEM *dictionary_set_item_unsafe(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data) {
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

    internal_error(!has_caller_exclusive_lock(dict),
                   "DICTIONARY: inserting dictionary item '%s' without exclusive access to dictionary", name);

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

    DICTIONARY_ITEM *nv = NULL;
    do {
        DICTIONARY_ITEM **pnv = (DICTIONARY_ITEM **)hashtable_insert_unsafe(dict, name, name_len);
        if (likely(*pnv == 0)) {
            // a new item added to the index
            nv = *pnv = item_create_unsafe(dict, name, name_len, value, value_len, constructor_data);
            hashtable_inserted_item_unsafe(dict, nv);
            item_linked_list_link_unsafe(dict, nv);
            nv->flags |= ITEM_FLAG_NEW_OR_UPDATED;
        }
        else {

            if (!item_found_in_hashtable_can_be_used(*pnv))
                continue;

            // the item is already in the index
            // so, either we will return the old one
            // or overwrite the value, depending on dictionary flags

            // We should not compare the values here!
            // even if they are the same, we have to do the whole job
            // so that the callbacks will be called.

            nv = *pnv;

            if (!(dict->options & DICT_OPTION_DONT_OVERWRITE_VALUE)) {
                item_reset_unsafe(dict, nv, value, value_len);
                nv->flags |= ITEM_FLAG_NEW_OR_UPDATED;
            }

            else if (dict->hooks && dict->hooks->conflict_callback) {
                dict->hooks->conflict_callback(
                    nv,
                    nv->shared->value,
                    value,
                    constructor_data ? constructor_data : dict->hooks->conflict_callback_data);

                nv->flags |= ITEM_FLAG_NEW_OR_UPDATED;
            }

            else {
                // we did really nothing!
                // make sure this flag is not set.
                nv->flags &= ~ITEM_FLAG_NEW_OR_UPDATED;
            }
        }
    } while(!nv);

    return nv;
}

static DICTIONARY_ITEM *dictionary_get_item_unsafe(DICTIONARY *dict, const char *name, ssize_t name_len) {
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

    DICTIONARY_ITEM *nv = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(!item_found_in_hashtable_can_be_used(nv))) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        return NULL;
    }

    debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
    return nv;
}

// ----------------------------------------------------------------------------
// API - items management

/*
static void *dictionary_set_advanced_unsafe(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data) {
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

    DICTIONARY_ITEM *nv = dictionary_set_item_unsafe(dict, name, name_len, value, value_len, constructor_data);

    if(unlikely(dict->hooks && dict->hooks->react_callback && nv && (nv->flags & ITEM_FLAG_NEW_OR_UPDATED))) {
        // we need to call the react callback with a reference counter on nv
        reference_counter_acquire(dict, nv);
        dict->hooks->react_callback(nv, nv->shared->value, constructor_data?constructor_data:dict->hooks->react_callback_data);
        reference_counter_release(dict, nv, false);
    }

    return nv ? nv->shared->value : NULL;
}
*/

void *dictionary_set_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data) {
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

    if(name_len == -1)
        name_len = (ssize_t)strlen((const char *)name) + 1;

    dictionary_lock(dict, DICTIONARY_LOCK_WRITE);
    DICTIONARY_ITEM *nv = dictionary_set_item_unsafe(dict, name, name_len, value, value_len, constructor_data);

    // we need to get a reference counter for the react callback
    // before we unlock the dictionary
    if(unlikely(dict->hooks && dict->hooks->react_callback && nv && (nv->flags & ITEM_FLAG_NEW_OR_UPDATED)))
        reference_counter_acquire(dict, nv);

    dictionary_unlock(dict, DICTIONARY_LOCK_WRITE);

    if(unlikely(dict->hooks && dict->hooks->react_callback && nv && (nv->flags & ITEM_FLAG_NEW_OR_UPDATED))) {
        // we got the reference counter we need, above
        dict->hooks->react_callback(nv, nv->shared->value, constructor_data?constructor_data:dict->hooks->react_callback_data);
        reference_counter_release(dict, nv, false);
    }

    return nv ? nv->shared->value : NULL;
}

/*
static DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_set_and_acquire_item_advanced_unsafe(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data) {
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

    DICTIONARY_ITEM *nv = dictionary_set_item_unsafe(dict, name, name_len, value, value_len, constructor_data);

    if(unlikely(!nv))
        return NULL;

    reference_counter_acquire(dict, nv);

    if(unlikely(dict->hooks && dict->hooks->react_callback && (nv->flags & ITEM_FLAG_NEW_OR_UPDATED))) {
        dict->hooks->react_callback(nv, nv->shared->value, constructor_data?constructor_data:dict->hooks->react_callback_data);
    }

    return nv;
}
*/

DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_set_and_acquire_item_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data) {
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

    dictionary_lock(dict, DICTIONARY_LOCK_WRITE);
    DICTIONARY_ITEM *nv = dictionary_set_item_unsafe(dict, name, name_len, value, value_len, constructor_data);

    // we need to get the reference counter before we unlock
    if(nv) reference_counter_acquire(dict, nv);

    dictionary_unlock(dict, DICTIONARY_LOCK_WRITE);

    if(unlikely(dict->hooks && dict->hooks->react_callback && nv && (nv->flags & ITEM_FLAG_NEW_OR_UPDATED))) {
        // we already have a reference counter, for the caller, no need for another one
        dict->hooks->react_callback(nv, nv->shared->value, constructor_data?constructor_data:dict->hooks->react_callback_data);
    }

    return nv;
}

static void *dictionary_get_advanced_unsafe(DICTIONARY *dict, const char *name, ssize_t name_len) {
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

    DICTIONARY_ITEM *nv = dictionary_get_item_unsafe(dict, name, name_len);

    if(unlikely(!nv))
        return NULL;

    return nv->shared->value;
}

void *dictionary_get_advanced(DICTIONARY *dict, const char *name, ssize_t name_len) {
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

    dictionary_lock(dict, DICTIONARY_LOCK_READ);
    void *ret = dictionary_get_advanced_unsafe(dict, name, name_len);
    dictionary_unlock(dict, DICTIONARY_LOCK_READ);
    return ret;
}

static DICTIONARY_ITEM *dictionary_get_and_acquire_item_advanced_unsafe(DICTIONARY *dict, const char *name, ssize_t name_len) {
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

    DICTIONARY_ITEM *nv = dictionary_get_item_unsafe(dict, name, name_len);

    if(unlikely(!nv))
        return NULL;

    reference_counter_acquire(dict, nv);
    return nv;
}

DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_get_and_acquire_item_advanced(DICTIONARY *dict, const char *name, ssize_t name_len) {
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

    dictionary_lock(dict, DICTIONARY_LOCK_READ);
    DICT_ITEM_CONST DICTIONARY_ITEM *ret = dictionary_get_and_acquire_item_advanced_unsafe(dict, name, name_len);
    dictionary_unlock(dict, DICTIONARY_LOCK_READ);
    return ret;
}

DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_acquired_item_dup(DICTIONARY *dict, DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    if(unlikely(!item)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without an item on a dictionary created from %s() %zu@%s.",
            __FUNCTION__,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);
        return NULL;
    }
    reference_counter_acquire(dict, item);
    return item;
}

const char *dictionary_acquired_item_name(DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    if(unlikely(!item)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without an item on a dictionary.",
            __FUNCTION__);
        return NULL;
    }
    return item_get_name(item);
}

void *dictionary_acquired_item_value(DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    if(unlikely(!item)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without an item on a dictionary.",
            __FUNCTION__);
        return NULL;
    }
    return item->shared->value;
}
/*
void dictionary_acquired_item_release_unsafe(DICTIONARY *dict, DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    if(unlikely(!item)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without an item on a dictionary created from %s() %zu@%s.",
            __FUNCTION__,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);
        return;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(item->dict != dict)
        fatal("DICTIONARY: %s(): item with name '%s' does not belong to this dictionary.", __FUNCTION__, item_get_name(item));
#endif

    reference_counter_release(dict, item, false);
}
*/

void dictionary_acquired_item_release(DICTIONARY *dict, DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    if(unlikely(!item)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without an item on a dictionary created from %s() %zu@%s.",
            __FUNCTION__,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);
        return;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(item->dict != dict)
        fatal("DICTIONARY: %s(): item with name '%s' does not belong to this dictionary.", __FUNCTION__, item_get_name(item));
#endif

    // no need to get a lock here
    // we pass the last parameter to reference_counter_release() as true
    // so that the release may get a write-lock if required to clean up

    reference_counter_release(dict, item, false);
}

static int dictionary_del_advanced_unsafe(DICTIONARY *dict, const char *name, ssize_t name_len) {
    if(unlikely(!name || !*name)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without a name on a dictionary created from %s() %zu@%s.",
            __FUNCTION__,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);
        return -1;
    }

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_del() on a destroyed dictionary");
        return -1;
    }

    internal_error(!has_caller_exclusive_lock(dict),
                   "DICTIONARY: INTERNAL ERROR: deleting dictionary item '%s' without exclusive access to dictionary", name);

    if(name_len == -1)
        name_len = (ssize_t)strlen(name) + 1; // we need the terminating null too

    debug(D_DICTIONARY, "DEL dictionary entry with name '%s'.", name);

    // Unfortunately, the JudyHSDel() does not return the value of the
    // item that was deleted, so we have to find it before we delete it,
    // since we need to release our structures too.

    int ret;
    DICTIONARY_ITEM *nv = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        ret = -1;
    }
    else {
        debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);

        if(hashtable_delete_unsafe(dict, name, name_len, nv) == 0)
            error("DICTIONARY: INTERNAL ERROR: tried to delete item with name '%s' that is not in the index", name);

        item_delete_or_mark_deleted_unsafe(dict, nv);

        ret = 0;
    }
    return ret;
}

int dictionary_del_advanced(DICTIONARY *dict, const char *name, ssize_t name_len) {
    if(unlikely(!name || !*name)) {
        internal_error(
            true,
            "DICTIONARY: attempted to %s() without a name on a dictionary created from %s() %zu@%s.",
            __FUNCTION__,
            dict->creation_function,
            dict->creation_line,
            dict->creation_file);
        return -1;
    }

    dictionary_lock(dict, DICTIONARY_LOCK_WRITE);
    int ret = dictionary_del_advanced_unsafe(dict, name, name_len);
    dictionary_unlock(dict, DICTIONARY_LOCK_WRITE);
    return ret;
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

    dictionary_lock(dict, dfe->rw);

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    // get the first item from the list
    DICTIONARY_ITEM *nv = dict->items;

    // skip all the deleted items
    while(nv && (nv->flags & ITEM_FLAG_DELETED))
        nv = nv->next;

    if(likely(nv)) {
        dfe->item = nv;
        dfe->name = (char *)item_get_name(nv);
        dfe->value = nv->shared->value;
        reference_counter_acquire(dict, nv);
    }
    else {
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
    }

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT))
        dictionary_unlock(dfe->dict, dfe->rw);

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
        dictionary_lock(dfe->dict, dfe->rw);

    // the item we just did
    DICTIONARY_ITEM *nv = dfe->item;

    // get the next item from the list
    DICTIONARY_ITEM *nv_next = (nv) ? nv->next : NULL;

    // skip all the deleted items
    while(nv_next && (nv_next->flags & ITEM_FLAG_DELETED))
        nv_next = nv_next->next;

    // release the old, so that it can possibly be deleted
    if(likely(nv))
        reference_counter_release(dfe->dict, nv, false);

    if(likely(nv = nv_next)) {
        dfe->item = nv;
        dfe->name = (char *)item_get_name(nv);
        dfe->value = nv->shared->value;
        reference_counter_acquire(dfe->dict, nv);
        dfe->counter++;
    }
    else {
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
    }

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT))
        dictionary_unlock(dfe->dict, dfe->rw);

    return dfe->value;
}

void dictionary_foreach_done(DICTFE *dfe) {
    if(unlikely(!dfe || !dfe->dict)) return;

    if(unlikely(is_dictionary_destroyed(dfe->dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_foreach_next() on a destroyed dictionary");
        return;
    }

    // the item we just did
    DICTIONARY_ITEM *nv = dfe->item;

    // release it, so that it can possibly be deleted
    if(likely(nv))
        reference_counter_release(dfe->dict, nv, false);

    if(likely(dfe->rw != DICTIONARY_LOCK_REENTRANT))
        dictionary_unlock(dfe->dict, dfe->rw);

    dfe->dict = NULL;
    dfe->item = NULL;
    dfe->name = NULL;
    dfe->value = NULL;
    dfe->counter = 0;
}

// ----------------------------------------------------------------------------
// API - walk through the dictionary
// the dictionary is locked for reading while this happens
// do not use other dictionary calls while walking the dictionary - deadlock!

int dictionary_walkthrough_rw(DICTIONARY *dict, char rw, int (*callback)(const DICTIONARY_ITEM *item, void *entry, void *data), void *data) {
    if(unlikely(!dict || !callback)) return 0;

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_walkthrough_rw() on a destroyed dictionary");
        return 0;
    }

    dictionary_lock(dict, rw);

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    // written in such a way, that the callback can delete the active element

    int ret = 0;
    DICTIONARY_ITEM *nv = dict->items, *nv_next;
    while(nv) {

        // skip the deleted items
        if(unlikely(nv->flags & ITEM_FLAG_DELETED)) {
            nv = nv->next;
            continue;
        }

        // get a reference counter, so that our item will not be deleted
        // while we are using it
        reference_counter_acquire(dict, nv);

        if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
            dictionary_unlock(dict, rw);

        int r = callback(nv, nv->shared->value, data);

        if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
            dictionary_lock(dict, rw);

        // since we have a reference counter, this item cannot be deleted
        // until we release the reference counter, so the pointers are there
        nv_next = nv->next;
        reference_counter_release(dict, nv, false);

        if(unlikely(r < 0)) {
            ret = r;
            break;
        }

        ret += r;

        nv = nv_next;
    }

    dictionary_unlock(dict, rw);

    return ret;
}

// ----------------------------------------------------------------------------
// sorted walkthrough

static int dictionary_sort_compar(const void *nv1, const void *nv2) {
    return strcmp(item_get_name((*(DICTIONARY_ITEM **)nv1)), item_get_name((*(DICTIONARY_ITEM **)nv2)));
}

int dictionary_sorted_walkthrough_rw(DICTIONARY *dict, char rw, int (*callback)(const DICTIONARY_ITEM *item, void *entry, void *data), void *data) {
    if(unlikely(!dict || !callback || !dict->entries)) return 0;

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_sorted_walkthrough_rw() on a destroyed dictionary");
        return 0;
    }

    dictionary_lock(dict, rw);
    dictionary_defer_all_deletions_unsafe(dict, rw);

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    size_t count = dict->entries;
    DICTIONARY_ITEM **array = mallocz(sizeof(DICTIONARY_ITEM *) * count);

    size_t i;
    DICTIONARY_ITEM *nv;
    for(nv = dict->items, i = 0; nv && i < count ;nv = nv->next) {
        if(likely(!(nv->flags & ITEM_FLAG_DELETED)))
            array[i++] = nv;
    }

    internal_error(nv != NULL, "DICTIONARY: during sorting expected to have %zu items in dictionary, but there are more. Sorted results may be incomplete. Dictionary fails to maintain an accurate number of the number of entries it has.", count);

    if(unlikely(i != count)) {
        internal_error(true, "DICTIONARY: during sorting expected to have %zu items in dictionary, but there are %zu. Sorted results may be incomplete. Dictionary fails to maintain an accurate number of the number of entries it has.", count, i);
        count = i;
    }

    qsort(array, count, sizeof(DICTIONARY_ITEM *), dictionary_sort_compar);

    int ret = 0;
    for(i = 0; i < count ;i++) {
        nv = array[i];
        if(likely(!(nv->flags & ITEM_FLAG_DELETED))) {
            reference_counter_acquire(dict, nv);

            if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
                dictionary_unlock(dict, rw);

            int r = callback(nv, nv->shared->value, data);

            if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
                dictionary_lock(dict, rw);

            reference_counter_release(dict, nv, false);
            if (r < 0) {
                ret = r;
                break;
            }
            ret += r;
        }
    }

    dictionary_restore_all_deletions_unsafe(dict, rw);
    dictionary_unlock(dict, rw);
    freez(array);

    return ret;
}

// ----------------------------------------------------------------------------
// STRING implementation - dedup all STRINGs

struct netdata_string {
    uint32_t length;    // the string length including the terminating '\0'

    int32_t refcount;   // how many times this string is used
                        // We use a signed number to be able to detect duplicate frees of a string.
                        // If at any point this goes below zero, we have a duplicate free.

    const char str[];   // the string itself, is appended to this structure
};

static struct string_hashtable {
    Pvoid_t JudyHSArray;        // the Judy array - hashtable
    netdata_rwlock_t rwlock;    // the R/W lock to protect the Judy array

    long int entries;           // the number of entries in the index
    long int active_references; // the number of active references alive
    long int memory;            // the memory used, without the JudyHS index

    size_t inserts;             // the number of successful inserts to the index
    size_t deletes;             // the number of successful deleted from the index
    size_t searches;            // the number of successful searches in the index
    size_t duplications;        // when a string is referenced
    size_t releases;            // when a string is unreferenced

#ifdef NETDATA_INTERNAL_CHECKS
    // internal statistics
    size_t found_deleted_on_search;
    size_t found_available_on_search;
    size_t found_deleted_on_insert;
    size_t found_available_on_insert;
    size_t spins;
#endif

} string_base = {
    .JudyHSArray = NULL,
    .rwlock = NETDATA_RWLOCK_INITIALIZER,
};

#ifdef NETDATA_INTERNAL_CHECKS
#define string_internal_stats_add(var, val) __atomic_add_fetch(&string_base.var, val, __ATOMIC_RELAXED)
#else
#define string_internal_stats_add(var, val) do {;} while(0)
#endif

#define string_stats_atomic_increment(var) __atomic_add_fetch(&string_base.var, 1, __ATOMIC_RELAXED)
#define string_stats_atomic_decrement(var) __atomic_sub_fetch(&string_base.var, 1, __ATOMIC_RELAXED)

void string_statistics(size_t *inserts, size_t *deletes, size_t *searches, size_t *entries, size_t *references, size_t *memory, size_t *duplications, size_t *releases) {
    *inserts = string_base.inserts;
    *deletes = string_base.deletes;
    *searches = string_base.searches;
    *entries = (size_t)string_base.entries;
    *references = (size_t)string_base.active_references;
    *memory = (size_t)string_base.memory;
    *duplications = string_base.duplications;
    *releases = string_base.releases;
}

#define string_entry_acquire(se) __atomic_add_fetch(&((se)->refcount), 1, __ATOMIC_SEQ_CST);
#define string_entry_release(se) __atomic_sub_fetch(&((se)->refcount), 1, __ATOMIC_SEQ_CST);

static inline bool string_entry_check_and_acquire(STRING *se) {
    int32_t expected, desired, count = 0;
    do {
        count++;

        expected = __atomic_load_n(&se->refcount, __ATOMIC_SEQ_CST);

        if(expected <= 0) {
            // We cannot use this.
            // The reference counter reached value zero,
            // so another thread is deleting this.
            string_internal_stats_add(spins, count - 1);
            return false;
        }

        desired = expected + 1;
    }
    while(!__atomic_compare_exchange_n(&se->refcount, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST));

    string_internal_stats_add(spins, count - 1);

    // statistics
    // string_base.active_references is altered at the in string_strdupz() and string_freez()
    string_stats_atomic_increment(duplications);

    return true;
}

STRING *string_dup(STRING *string) {
    if(unlikely(!string)) return NULL;

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(__atomic_load_n(&string->refcount, __ATOMIC_SEQ_CST) <= 0))
        fatal("STRING: tried to %s() a string that is freed (it has %d references).", __FUNCTION__, string->refcount);
#endif

    string_entry_acquire(string);

    // statistics
    string_stats_atomic_increment(active_references);
    string_stats_atomic_increment(duplications);

    return string;
}

// Search the index and return an ACQUIRED string entry, or NULL
static inline STRING *string_index_search(const char *str, size_t length) {
    STRING *string;

    // Find the string in the index
    // With a read-lock so that multiple readers can use the index concurrently.

    netdata_rwlock_rdlock(&string_base.rwlock);

    Pvoid_t *Rc;
    Rc = JudyHSGet(string_base.JudyHSArray, (void *)str, length);
    if(likely(Rc)) {
        // found in the hash table
        string = *Rc;

        if(string_entry_check_and_acquire(string)) {
            // we can use this entry
            string_internal_stats_add(found_available_on_search, 1);
        }
        else {
            // this entry is about to be deleted by another thread
            // do not touch it, let it go...
            string = NULL;
            string_internal_stats_add(found_deleted_on_search, 1);
        }
    }
    else {
        // not found in the hash table
        string = NULL;
    }

    string_stats_atomic_increment(searches);
    netdata_rwlock_unlock(&string_base.rwlock);

    return string;
}

// Insert a string to the index and return an ACQUIRED string entry,
// or NULL if the call needs to be retried (a deleted entry with the same key is still in the index)
// The returned entry is ACQUIRED and it can either be:
//   1. a new item inserted, or
//   2. an item found in the index that is not currently deleted
static inline STRING *string_index_insert(const char *str, size_t length) {
    STRING *string;

    netdata_rwlock_wrlock(&string_base.rwlock);

    STRING **ptr;
    {
        JError_t J_Error;
        Pvoid_t *Rc = JudyHSIns(&string_base.JudyHSArray, (void *)str, length, &J_Error);
        if (unlikely(Rc == PJERR)) {
            fatal(
                "STRING: Cannot insert entry with name '%s' to JudyHS, JU_ERRNO_* == %u, ID == %d",
                str,
                JU_ERRNO(&J_Error),
                JU_ERRID(&J_Error));
        }
        ptr = (STRING **)Rc;
    }

    if (likely(*ptr == 0)) {
        // a new item added to the index
        size_t mem_size = sizeof(STRING) + length;
        string = mallocz(mem_size);
        strcpy((char *)string->str, str);
        string->length = length;
        string->refcount = 1;
        *ptr = string;
        string_base.inserts++;
        string_base.entries++;
        string_base.memory += (long)mem_size;
    }
    else {
        // the item is already in the index
        string = *ptr;

        if(string_entry_check_and_acquire(string)) {
            // we can use this entry
            string_internal_stats_add(found_available_on_insert, 1);
        }
        else {
            // this entry is about to be deleted by another thread
            // do not touch it, let it go...
            string = NULL;
            string_internal_stats_add(found_deleted_on_insert, 1);
        }

        string_stats_atomic_increment(searches);
    }

    netdata_rwlock_unlock(&string_base.rwlock);
    return string;
}

// delete an entry from the index
static inline void string_index_delete(STRING *string) {
    netdata_rwlock_wrlock(&string_base.rwlock);

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(__atomic_load_n(&string->refcount, __ATOMIC_SEQ_CST) != 0))
        fatal("STRING: tried to delete a string at %s() that is already freed (it has %d references).", __FUNCTION__, string->refcount);
#endif

    bool deleted = false;

    if (likely(string_base.JudyHSArray)) {
        JError_t J_Error;
        int ret = JudyHSDel(&string_base.JudyHSArray, (void *)string->str, string->length, &J_Error);
        if (unlikely(ret == JERR)) {
            error(
                "STRING: Cannot delete entry with name '%s' from JudyHS, JU_ERRNO_* == %u, ID == %d",
                string->str,
                JU_ERRNO(&J_Error),
                JU_ERRID(&J_Error));
        } else
            deleted = true;
    }

    if (unlikely(!deleted))
        error("STRING: tried to delete '%s' that is not in the index. Ignoring it.", string->str);
    else {
        size_t mem_size = sizeof(STRING) + string->length;
        string_base.deletes++;
        string_base.entries--;
        string_base.memory -= (long)mem_size;
        freez(string);
    }

    netdata_rwlock_unlock(&string_base.rwlock);
}

STRING *string_strdupz(const char *str) {
    if(unlikely(!str || !*str)) return NULL;

    size_t length = strlen(str) + 1;
    STRING *string = string_index_search(str, length);

    while(!string) {
        // The search above did not find anything,
        // We loop here, because during insert we may find an entry that is being deleted by another thread.
        // So, we have to let it go and retry to insert it again.

        string = string_index_insert(str, length);
    }

    // statistics
    string_stats_atomic_increment(active_references);

    return string;
}

void string_freez(STRING *string) {
    if(unlikely(!string)) return;

    int32_t refcount = string_entry_release(string);

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(refcount < 0))
        fatal("STRING: tried to %s() a string that is already freed (it has %d references).", __FUNCTION__, string->refcount);
#endif

    if(unlikely(refcount == 0))
        string_index_delete(string);

    // statistics
    string_stats_atomic_decrement(active_references);
    string_stats_atomic_increment(releases);
}

size_t string_strlen(STRING *string) {
    if(unlikely(!string)) return 0;
    return string->length - 1;
}

const char *string2str(STRING *string) {
    if(unlikely(!string)) return "";
    return string->str;
}

STRING *string_2way_merge(STRING *a, STRING *b) {
    static STRING *X = NULL;

    if(unlikely(!X)) {
        X = string_strdupz("[x]");
    }

    if(unlikely(a == b)) return string_dup(a);
    if(unlikely(a == X)) return string_dup(a);
    if(unlikely(b == X)) return string_dup(b);
    if(unlikely(!a)) return string_dup(X);
    if(unlikely(!b)) return string_dup(X);

    size_t alen = string_strlen(a);
    size_t blen = string_strlen(b);
    size_t length = alen + blen + string_strlen(X) + 1;
    char buf1[length + 1], buf2[length + 1], *dst1;
    const char *s1, *s2;

    s1 = string2str(a);
    s2 = string2str(b);
    dst1 = buf1;
    for( ; *s1 && *s2 && *s1 == *s2 ;s1++, s2++)
        *dst1++ = *s1;

    *dst1 = '\0';

    if(*s1 != '\0' || *s2 != '\0') {
        *dst1++ = '[';
        *dst1++ = 'x';
        *dst1++ = ']';

        s1 = &(string2str(a))[alen - 1];
        s2 = &(string2str(b))[blen - 1];
        char *dst2 = &buf2[length];
        *dst2 = '\0';
        for (; *s1 && *s2 && *s1 == *s2; s1--, s2--)
            *(--dst2) = *s1;

        strcpy(dst1, dst2);
    }

    return string_strdupz(buf1);
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
    long i = 0;
    for(; i < (long)entries ;i++) {
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
        int ret = dictionary_del(dict, values[i]);
        if(ret != -1) { fprintf(stderr, ">>> %s() deleted non-existing item\n", __FUNCTION__); errors++; }
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
        int ret = dictionary_del(dict, names[i]);
        if(ret == -1) { fprintf(stderr, ">>> %s() didn't delete (forward) existing item\n", __FUNCTION__); errors++; }
    }

    for(size_t i = middle_to - 1; i >= middle_from ;i--) {
        int ret = dictionary_del(dict, names[i]);
        if(ret == -1) { fprintf(stderr, ">>> %s() didn't delete (middle) existing item\n", __FUNCTION__); errors++; }
    }

    for(size_t i = backward_to - 1; i >= backward_from ;i--) {
        int ret = dictionary_del(dict, names[i]);
        if(ret == -1) { fprintf(stderr, ">>> %s() didn't delete (backward) existing item\n", __FUNCTION__); errors++; }
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

    if(dictionary_del((DICTIONARY *)data, name) == -1)
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
        if(dictionary_del(dict, item_dfe.name) != -1) count++;
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

    fprintf(stderr, " %zu errors, %ld items in dictionary, %llu usec \n", errs, dict? dictionary_entries(dict):0, dt);
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
    const char *oldname;
    const char *oldvalue;
    size_t count;
};

static int dictionary_unittest_sorting_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    struct dictionary_unittest_sorting *t = (struct dictionary_unittest_sorting *)data;
    const char *v = (const char *)value;

    int ret = 0;
    if(t->oldname && strcmp(t->oldname, name) > 0) {
        fprintf(stderr, "name '%s' should be after '%s'\n", t->oldname, name);
        ret = 1;
    }
    t->count++;
    t->oldname = name;
    t->oldvalue = v;

    return ret;
}

static size_t dictionary_unittest_sorted_walkthrough(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    struct dictionary_unittest_sorting tmp = { .oldname = NULL, .oldvalue = NULL, .count = 0 };
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


static int check_dictionary_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    return 1;
}

static size_t check_dictionary(DICTIONARY *dict, size_t entries, size_t linked_list_members) {
    size_t errors = 0;

    fprintf(stderr, "dictionary entries %ld, expected %zu...\t\t\t\t\t", dictionary_entries(dict), entries);
    if (dictionary_entries(dict) != (long)entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    size_t ll = 0;
    void *t;
    dfe_start_read(dict, t)
        ll++;
    dfe_done(t);

    fprintf(stderr, "dictionary foreach entries %zu, expected %zu...\t\t\t\t", ll, entries);
    if(ll != entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    ll = dictionary_walkthrough_read(dict, check_dictionary_callback, NULL);
    fprintf(stderr, "dictionary walkthrough entries %zu, expected %zu...\t\t\t\t", ll, entries);
    if(ll != entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    ll = dictionary_sorted_walkthrough_read(dict, check_dictionary_callback, NULL);
    fprintf(stderr, "dictionary sorted walkthrough entries %zu, expected %zu...\t\t\t", ll, entries);
    if(ll != entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    DICTIONARY_ITEM *nv;
    for(ll = 0, nv = dict->items; nv ;nv = nv->next)
        ll++;

    fprintf(stderr, "dictionary linked list entries %zu, expected %zu...\t\t\t\t", ll, linked_list_members);
    if(ll != linked_list_members) {
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

static size_t check_item_deleted_flag(DICTIONARY *dict,
    DICTIONARY_ITEM *nv, const char *name, const char *value, int refcount,
    ITEM_FLAGS deleted_flags, bool searchable, bool browsable, bool linked) {
    size_t errors = 0;

    fprintf(stderr, "NAME_VALUE name is '%s', expected '%s'...\t\t\t\t", item_get_name(nv), name);
    if(strcmp(item_get_name(nv), name) != 0) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "NAME_VALUE value is '%s', expected '%s'...\t\t\t", (const char *)nv->shared->value, value);
    if(strcmp((const char *)nv->shared->value, value) != 0) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "NAME_VALUE refcount is %d, expected %d...\t\t\t\t\t", nv->shared->refcount, refcount);
    if (nv->shared->refcount != refcount) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "NAME_VALUE deleted flag is %s, expected %s...\t\t\t", (nv->flags & ITEM_FLAG_DELETED)?"TRUE":"FALSE", (deleted_flags & ITEM_FLAG_DELETED)?"TRUE":"FALSE");
    if ((nv->flags & ITEM_FLAG_DELETED) != (deleted_flags & ITEM_FLAG_DELETED)) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    void *v = dictionary_get(dict, name);
    bool found = v == nv->shared->value;
    fprintf(stderr, "NAME_VALUE searchable %5s, expected %5s...\t\t\t\t", found?"true":"false", searchable?"true":"false");
    if(found != searchable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = false;
    void *t;
    dfe_start_read(dict, t) {
        if(t == nv->shared->value) found = true;
    }
    dfe_done(t);

    fprintf(stderr, "NAME_VALUE dfe browsable %5s, expected %5s...\t\t\t", found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = dictionary_walkthrough_read(dict, check_item_callback, nv->shared->value);
    fprintf(stderr, "NAME_VALUE walkthrough browsable %5s, expected %5s...\t\t", found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = dictionary_sorted_walkthrough_read(dict, check_item_callback, nv->shared->value);
    fprintf(stderr, "NAME_VALUE sorted walkthrough browsable %5s, expected %5s...\t", found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = false;
    DICTIONARY_ITEM *n;
    for(n = dict->items; n ;n = n->next)
        if(n == nv) found = true;

    fprintf(stderr, "NAME_VALUE linked %5s, expected %5s...\t\t\t\t", found?"true":"false", linked?"true":"false");
    if(found != linked) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    return errors;
}

static int string_threads_join = 0;
static void *string_thread(void *arg __maybe_unused) {
    int dups = 1; //(gettid() % 10);
    for(; 1 ;) {
        if(__atomic_load_n(&string_threads_join, __ATOMIC_RELAXED))
            break;

        STRING *s = string_strdupz("string thread checking 1234567890");

        for(int i = 0; i < dups ; i++)
            string_dup(s);

        for(int i = 0; i < dups ; i++)
            string_freez(s);

        string_freez(s);
    }

    return arg;
}

static int dict_threads_join = 0;
static DICTIONARY *dict_test1 = NULL;
static void *dict_thread(void *arg __maybe_unused) {
    int dups = 1; //(gettid() % 10);

    for(; 1 ;) {
        if(__atomic_load_n(&dict_threads_join, __ATOMIC_RELAXED))
            break;

        DICT_ITEM_CONST DICTIONARY_ITEM *item =
            dictionary_set_and_acquire_item_advanced(dict_test1, "dict thread checking 1234567890",
                                                     -1, NULL, 0, NULL);

        void *t1;
        dfe_start_write(dict_test1, t1) {

            // this should delete the referenced item
            dictionary_del(dict_test1, t1_dfe.name);

            void *t2;
            dfe_start_write(dict_test1, t2) {
                // this should add another
                dictionary_set(dict_test1, t2_dfe.name, NULL, 0);

                // and this should delete it again
                dictionary_del(dict_test1, t2_dfe.name);
            }
            dfe_done(t2);

            // this should fail to add it
            dictionary_set(dict_test1, t1_dfe.name, NULL, 0);
            dictionary_del(dict_test1, t1_dfe.name);
        }
        dfe_done(t1);

        for(int i = 0; i < dups ; i++)
            dictionary_acquired_item_dup(dict_test1, item);

        for(int i = 0; i < dups ; i++)
            dictionary_acquired_item_release(dict_test1, item);

        dictionary_acquired_item_release(dict_test1, item);

        dictionary_del(dict_test1, "dict thread checking 1234567890");
    }

    return arg;
}

int dictionary_unittest(size_t entries) {
    if(entries < 10) entries = 10;

    DICTIONARY *dict;
    size_t errors = 0;

    fprintf(stderr, "Generating %zu names and values...\n", entries);
    char **names = dictionary_unittest_generate_names(entries);
    char **values = dictionary_unittest_generate_values(entries);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_STATS);
    dictionary_unittest_clone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary multi threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_NONE | DICT_OPTION_STATS);
    dictionary_unittest_clone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary single threaded, non-clone, add-in-front options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE |
        DICT_OPTION_ADD_IN_FRONT | DICT_OPTION_STATS);
    dictionary_unittest_nonclone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary multi threaded, non-clone, add-in-front options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_ADD_IN_FRONT |
        DICT_OPTION_STATS);
    dictionary_unittest_nonclone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary single-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE |
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_STATS);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "resetting non-overwrite entries", names, values, entries, &errors, dictionary_unittest_reset_dont_overwrite_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, &errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, &errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, &errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary multi-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE |
        DICT_OPTION_STATS);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough write delete this", names, values, entries, &errors, dictionary_unittest_walkthrough_delete_this);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary multi-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE |
        DICT_OPTION_STATS);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "foreach write delete this", names, values, entries, &errors, dictionary_unittest_foreach_delete_this);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop empty", names, values, 0, &errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback empty", names, values, 0, &errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_STATS);
    dictionary_unittest_sorting(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_STATS);
    dictionary_unittest_null_dfe(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, noclone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_STATS);
    dictionary_unittest_null_dfe(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    // check reference counters
    {
        fprintf(stderr, "\nTesting reference counters:\n");
        dict = dictionary_create(DICT_OPTION_NONE | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_STATS);
        errors += check_dictionary(dict, 0, 0);

        fprintf(stderr, "\nAdding test item to dictionary and acquiring it\n");
        dictionary_set(dict, "test", "ITEM1", 6);
        DICTIONARY_ITEM *nv = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");

        errors += check_dictionary(dict, 1, 1);
        errors += check_item_deleted_flag(dict, nv, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nChecking that reference counters are increased:\n");
        void *t;
        dfe_start_read(dict, t) {
            errors += check_dictionary(dict, 1, 1);
            errors += check_item_deleted_flag(dict, nv, "test", "ITEM1", 2, ITEM_FLAG_NONE, true, true, true);
        }
        dfe_done(t);

        fprintf(stderr, "\nChecking that reference counters are decreased:\n");
        errors += check_dictionary(dict, 1, 1);
        errors += check_item_deleted_flag(dict, nv, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDeleting the item we have acquired:\n");
        dictionary_del(dict, "test");

        errors += check_dictionary(dict, 0, 1);
        errors += check_item_deleted_flag(dict, nv, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);

        fprintf(stderr, "\nAdding another item with the same name of the item we deleted, while being acquired:\n");
        dictionary_set(dict, "test", "ITEM2", 6);
        errors += check_dictionary(dict, 1, 2);

        fprintf(stderr, "\nAcquiring the second item:\n");
        DICTIONARY_ITEM *nv2 = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");
        errors += check_item_deleted_flag(dict, nv, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);
        errors += check_item_deleted_flag(dict, nv2, "test", "ITEM2", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nReleasing the second item (the first is still acquired):\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)nv2);
        errors += check_dictionary(dict, 1, 2);
        errors += check_item_deleted_flag(dict, nv, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);
        errors += check_item_deleted_flag(dict, nv2, "test", "ITEM2", 0, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDeleting the second item (the first is still acquired):\n");
        dictionary_del(dict, "test");
        errors += check_dictionary(dict, 0, 1);
        errors += check_item_deleted_flag(dict, nv, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);

        fprintf(stderr, "\nReleasing the first item (which we have already deleted):\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)nv);
        errors += check_dictionary(dict, 0, 0);

        fprintf(stderr, "\nAdding again the test item to dictionary and acquiring it\n");
        dictionary_set(dict, "test", "ITEM1", 6);
        nv = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");

        errors += check_dictionary(dict, 1, 1);
        errors += check_item_deleted_flag(dict, nv, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDestroying the dictionary while we have acquired an item\n");
        dictionary_destroy(dict);

        fprintf(stderr, "Releasing the item (on a destroyed dictionary)\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)nv);
        nv = NULL;
        dict = NULL;
    }

    // check string
    {
        long int string_entries_starting = string_base.entries;

        fprintf(stderr, "\nChecking strings...\n");

        STRING *s1 = string_strdupz("hello unittest");
        STRING *s2 = string_strdupz("hello unittest");
        if(s1 != s2) {
            errors++;
            fprintf(stderr, "ERROR: duplicating strings are not deduplicated\n");
        }
        else
            fprintf(stderr, "OK: duplicating string are deduplicated\n");

        STRING *s3 = string_dup(s1);
        if(s3 != s1) {
            errors++;
            fprintf(stderr, "ERROR: cloning strings are not deduplicated\n");
        }
        else
            fprintf(stderr, "OK: cloning string are deduplicated\n");

        if(s1->refcount != 3) {
            errors++;
            fprintf(stderr, "ERROR: string refcount is not 3\n");
        }
        else
            fprintf(stderr, "OK: string refcount is 3\n");

        STRING *s4 = string_strdupz("world unittest");
        if(s4 == s1) {
            errors++;
            fprintf(stderr, "ERROR: string is sharing pointers on different strings\n");
        }
        else
            fprintf(stderr, "OK: string is properly handling different strings\n");

        usec_t start_ut, end_ut;
        STRING **strings = mallocz(entries * sizeof(STRING *));

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            strings[i] = string_strdupz(names[i]);
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Created %zu strings in %llu usecs\n", entries, end_ut - start_ut);

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            strings[i] = string_dup(strings[i]);
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Cloned %zu strings in %llu usecs\n", entries, end_ut - start_ut);

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            string_freez(strings[i]);
            string_freez(strings[i]);
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Freed %zu strings in %llu usecs\n", entries, end_ut - start_ut);

        freez(strings);

        if(string_base.entries != string_entries_starting + 2) {
            errors++;
            fprintf(stderr, "ERROR: strings dictionary should have %ld items but it has %ld\n", string_entries_starting + 2, string_base.entries);
        }
        else
            fprintf(stderr, "OK: strings dictionary has 2 items\n");
    }

    // check 2-way merge
    {
        struct testcase {
            char *src1; char *src2; char *expected;
        } tests[] = {
            { "", "", ""},
            { "a", "", "[x]"},
            { "", "a", "[x]"},
            { "a", "a", "a"},
            { "abcd", "abcd", "abcd"},
            { "foo_cs", "bar_cs", "[x]_cs"},
            { "cp_UNIQUE_INFIX_cs", "cp_unique_infix_cs", "cp_[x]_cs"},
            { "cp_UNIQUE_INFIX_ci_unique_infix_cs", "cp_unique_infix_ci_UNIQUE_INFIX_cs", "cp_[x]_cs"},
            { "foo[1234]", "foo[4321]", "foo[[x]]"},
            { NULL, NULL, NULL },
        };

        for (struct testcase *tc = &tests[0]; tc->expected != NULL; tc++) {
            STRING *src1 = string_strdupz(tc->src1);
            STRING *src2 = string_strdupz(tc->src2);
            STRING *expected = string_strdupz(tc->expected);

            STRING *result = string_2way_merge(src1, src2);
            if (string_cmp(result, expected) != 0) {
                fprintf(stderr, "string_2way_merge(\"%s\", \"%s\") -> \"%s\" (expected=\"%s\")\n",
                        string2str(src1),
                        string2str(src2),
                        string2str(result),
                        string2str(expected));
                errors++;
            }

            string_freez(src1);
            string_freez(src2);
            string_freez(expected);
            string_freez(result);
        }
    }

    dictionary_unittest_free_char_pp(names, entries);
    dictionary_unittest_free_char_pp(values, entries);

    // threads testing of dictionary
    {
        dict_test1 = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_STATS);
        time_t seconds_to_run = 5;
        int threads_to_create = 2;
        fprintf(
            stderr,
            "Checking dictionary concurrency with %d threads for %ld seconds...\n",
            threads_to_create,
            seconds_to_run);

        netdata_thread_t threads[threads_to_create];
        dict_threads_join = 0;
        for (int i = 0; i < threads_to_create; i++) {
            char buf[100 + 1];
            snprintf(buf, 100, "dict%d", i);
            netdata_thread_create(
                &threads[i], buf, NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE, dict_thread, NULL);
        }
        sleep_usec(seconds_to_run * USEC_PER_SEC);

        __atomic_store_n(&dict_threads_join, 1, __ATOMIC_RELAXED);
        for (int i = 0; i < threads_to_create; i++) {
            void *retval;
            netdata_thread_join(threads[i], &retval);
        }

        fprintf(stderr, "inserts %zu, deletes %zu, searches %zu, resets %zu, entries %ld, referenced_items %ld, pending deletions %ld\n",
                dict_test1->stats->inserts,
                dict_test1->stats->deletes,
                dict_test1->stats->searches,
                dict_test1->stats->resets,
                dict_test1->entries,
                dict_test1->referenced_items,
                dict_test1->pending_deletion_items
                );
        dictionary_destroy(dict_test1);
        dict_test1 = NULL;
    }

    // threads testing of string
    {
#ifdef NETDATA_INTERNAL_CHECKS
        size_t ofound_deleted_on_search = string_base.found_deleted_on_search,
               ofound_available_on_search = string_base.found_available_on_search,
               ofound_deleted_on_insert = string_base.found_deleted_on_insert,
               ofound_available_on_insert = string_base.found_available_on_insert,
               ospins = string_base.spins;
#endif

        size_t oinserts, odeletes, osearches, oentries, oreferences, omemory, oduplications, oreleases;
        string_statistics(&oinserts, &odeletes, &osearches, &oentries, &oreferences, &omemory, &oduplications, &oreleases);

        time_t seconds_to_run = 5;
        int threads_to_create = 2;
        fprintf(
            stderr,
            "Checking string concurrency with %d threads for %ld seconds...\n",
            threads_to_create,
            seconds_to_run);
        // check string concurrency
        netdata_thread_t threads[threads_to_create];
        string_threads_join = 0;
        for (int i = 0; i < threads_to_create; i++) {
            char buf[100 + 1];
            snprintf(buf, 100, "string%d", i);
            netdata_thread_create(
                &threads[i], buf, NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE, string_thread, NULL);
        }
        sleep_usec(seconds_to_run * USEC_PER_SEC);

        __atomic_store_n(&string_threads_join, 1, __ATOMIC_RELAXED);
        for (int i = 0; i < threads_to_create; i++) {
            void *retval;
            netdata_thread_join(threads[i], &retval);
        }

        size_t inserts, deletes, searches, sentries, references, memory, duplications, releases;
        string_statistics(&inserts, &deletes, &searches, &sentries, &references, &memory, &duplications, &releases);

        fprintf(stderr, "inserts %zu, deletes %zu, searches %zu, entries %zu, references %zu, memory %zu, duplications %zu, releases %zu\n",
                inserts - oinserts, deletes - odeletes, searches - osearches, sentries - oentries, references - oreferences, memory - omemory, duplications - oduplications, releases - oreleases);

#ifdef NETDATA_INTERNAL_CHECKS
        size_t found_deleted_on_search = string_base.found_deleted_on_search,
               found_available_on_search = string_base.found_available_on_search,
               found_deleted_on_insert = string_base.found_deleted_on_insert,
               found_available_on_insert = string_base.found_available_on_insert,
               spins = string_base.spins;

        fprintf(stderr, "on insert: %zu ok + %zu deleted\non search: %zu ok + %zu deleted\nspins: %zu\n",
                found_available_on_insert - ofound_available_on_insert,
                found_deleted_on_insert - ofound_deleted_on_insert,
                found_available_on_search - ofound_available_on_search,
                found_deleted_on_search - ofound_deleted_on_search,
                spins - ospins
                );
#endif
    }

    fprintf(stderr, "\n%zu errors found\n", errors);
    return  errors ? 1 : 0;
}
