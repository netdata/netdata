// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_INTERNALS_H
#define NETDATA_DICTIONARY_INTERNALS_H

#define DICTIONARY_INTERNALS
#include "../libnetdata.h"

// runtime flags of the dictionary - must be checked with atomics
typedef enum __attribute__ ((__packed__)) {
    DICT_FLAG_NONE                  = 0,
    DICT_FLAG_DESTROYED             = (1 << 0), // this dictionary has been destroyed
    DICT_FLAG_QUEUED_FOR_DESTRUCTION = (1 << 1), // this dictionary is queued for delayed destruction
} DICT_FLAGS;

// These are __ATOMIC_SEQ_CST, not relaxed: DICT_FLAG_DESTROYED is one half of
// the admission handshake with the in-flight counter below, and that handshake
// is only provable if all four operations involved are sequentially consistent.
// See the comment on dictionary_api_enter().
#define dict_flag_check(dict, flag) (__atomic_load_n(&((dict)->flags), __ATOMIC_SEQ_CST) & (flag))
#define dict_flag_set(dict, flag)   __atomic_or_fetch(&((dict)->flags), flag, __ATOMIC_SEQ_CST)
#define dict_flag_clear(dict, flag) __atomic_and_fetch(&((dict)->flags), ~(flag), __ATOMIC_SEQ_CST)

// flags macros
#define is_dictionary_destroyed(dict) dict_flag_check(dict, DICT_FLAG_DESTROYED)

// ----------------------------------------------------------------------------
// in-flight API callers
//
// referenced_items keeps ITEMS alive; this counter keeps the DICTIONARY OBJECT
// alive. A thread inside the API - or blocked on one of the dictionary's own
// locks - holds no item reference, so without this counter
// dictionary_destroy() can free the DICTIONARY (and the locks inside it) from
// under it. See dictionary_destroy() and dictionary_free_all_resources().
//
// Ordering: this is the store-buffer (Dekker) pattern over two objects, and it
// is made safe by keeping ALL FOUR operations __ATOMIC_SEQ_CST, so that they
// all appear in the single total order S the C memory model defines:
//
//   entering caller     destroy
//   ---------------     -------
//   1. RMW  inflight++  3. RMW  flags |= DESTROYED
//   2. load flags       4. load inflight
//
// Proof that the two cannot miss each other: suppose the caller's load (2)
// does not observe the flag, i.e. (2) precedes (3) in S. Sequentially
// consistent operations respect program order in S, so
// (1) < (2) < (3) < (4) in S. A SEQ_CST load reads the last preceding write in
// S, so (4) observes the increment of (1) and destroy defers. Either the caller
// is counted, or it sees the destroyed flag and bails out.
//
// This is why dict_flag_check() / dict_flag_set() are SEQ_CST and not relaxed:
// with a relaxed flag access the argument above collapses. A SEQ_CST RMW is not
// a fence in the C model - it does not order a later relaxed load of a
// different object - so both sides could legally miss each other and destroy
// would free the dictionary under an entering caller. No explicit
// __atomic_thread_fence() is needed once every operation is SEQ_CST.
//
// Limit: this protects a caller that has already entered the API. It cannot
// protect a caller holding a stale DICTIONARY * that has not entered yet -
// destroy may read inflight == 0 and free the object a moment before that
// caller increments it. That window is unclosable from inside the dictionary
// (it needs a reference held by whoever owns the DICTIONARY *) and remains a
// caller-ownership bug.
//
// Contract: every public function that takes one of the dictionary's own locks,
// or that holds the dictionary object across a blocking point, must be
// bracketed by dictionary_api_enter() / dictionary_api_exit(). That is the
// exact race this closes: pass the destroyed check, then block on a lock inside
// a struct that dictionary_destroy() already freed because it saw
// inflight == 0.
//
// dictionary_destroy() is the one deliberate exception to that rule, and it
// cannot be otherwise: bracketing it would count the destroying thread in
// inflight, so its own recheck below would always observe a non-zero counter
// and it could never take the immediate free path. It is instead SINGLE-OWNER
// (dictionary.h) - exactly one thread ends a dictionary's life, externally
// serialized against any other destroy of the same object. Two concurrent
// destroys can both pass the destroyed-flag check, and one can free the object
// while the other is blocked on ll_recursive_lock(); that is a
// double-free-class caller error, in the same class as the stale-pointer case
// above, and is likewise not detectable from inside the dictionary.
//
// Functions that only perform a single atomic load or add on the object
// (dictionary_version(), dictionary_entries(), dictionary_referenced_items(),
// dictionary_version_increment()) are deliberately not bracketed: they hold
// nothing across a blocking point, so the bracket would close no window that
// caller ownership does not already have to cover, and dictionary_api_enter()
// itself is a plain atomic add of the same weight.
//
// The callback registration functions (dictionary_register_*_callback()) are
// construction-time API; the rule is stated for callers in dictionary.h. They
// write dict->hooks - and dict->options, with a non-atomic read-modify-write -
// so they must be called before the dictionary is published to other threads.
// enter/exit would not make them safe.
//
// The counter is incremented unconditionally, including for single-threaded
// dictionaries. Testing DICT_OPTION_SINGLE_THREADED first would dereference the
// dictionary before the caller is counted, widening the window above by the
// length of that load, and would itself race the non-atomic dict->options
// update in dictionary_register_conflict_callback(). One uncontended atomic RMW
// is not worth either.
#define dictionary_api_enter(dict) do {                                             \
        __atomic_add_fetch(&((dict)->inflight), 1, __ATOMIC_SEQ_CST);               \
    } while(0)

#define dictionary_api_exit(dict) do {                                              \
        __atomic_sub_fetch(&((dict)->inflight), 1, __ATOMIC_SEQ_CST);               \
    } while(0)

#define dictionary_inflight(dict) __atomic_load_n(&((dict)->inflight), __ATOMIC_SEQ_CST)

// configuration options macros
#define is_dictionary_single_threaded(dict) ((dict)->options & DICT_OPTION_SINGLE_THREADED)
#define is_view_dictionary(dict) ((dict)->master)
#define is_master_dictionary(dict) (!is_view_dictionary(dict))

typedef enum __attribute__ ((__packed__)) item_options {
    ITEM_OPTION_NONE            = 0,
    ITEM_OPTION_ALLOCATED_NAME  = (1 << 0), // the name pointer is a STRING

    // IMPORTANT: This is 1-bit - to add more change ITEM_OPTIONS_BITS
} ITEM_OPTIONS;

typedef enum __attribute__ ((__packed__)) item_flags {
    ITEM_FLAG_NONE              = 0,
    ITEM_FLAG_DELETED           = (1 << 0), // this item is marked deleted, so it is not available for traversal (deleted from the index too)
    ITEM_FLAG_BEING_CREATED     = (1 << 1), // this item is currently being created - this flag is removed when construction finishes
    ITEM_FLAG_PENDING_DELETION  = (1 << 2), // this deleted item is counted in pending_deletion_items

    // IMPORTANT: This is 8-bit
} ITEM_FLAGS;

#define item_flag_check(item, flag) (__atomic_load_n(&((item)->flags), __ATOMIC_RELAXED) & (flag))
#define item_flag_set(item, flag)   __atomic_or_fetch(&((item)->flags), flag, __ATOMIC_RELAXED)
#define item_flag_clear(item, flag) __atomic_and_fetch(&((item)->flags), ~(flag), __ATOMIC_RELAXED)

#define item_shared_flag_check(item, flag) (__atomic_load_n(&((item)->shared->flags), __ATOMIC_RELAXED) & (flag))
#define item_shared_flag_set(item, flag)   __atomic_or_fetch(&((item)->shared->flags), flag, __ATOMIC_RELAXED)
#define item_shared_flag_clear(item, flag) __atomic_and_fetch(&((item)->shared->flags), ~(flag), __ATOMIC_RELAXED)

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

typedef struct dictionary_item_shared {
    void *value;                            // the value of the dictionary item

    // the order of the following items is important!
    // The total of their storage should be 64-bits

    REFCOUNT links;                         // how many links this item has
    VALUE_LEN_TYPE value_len:VALUE_LEN_BITS; // the size of the value
    ITEM_FLAGS_TYPE flags;                  // shared flags
} DICTIONARY_ITEM_SHARED;

struct dictionary_item {
#ifdef FSANITIZE_ADDRESS
    STACKTRACE_ARRAY stacktraces;   // stack traces from all acquisition points
#endif
#if defined(FSANITIZE_ADDRESS) || defined(NETDATA_INTERNAL_CHECKS)
    DICTIONARY *dict;                     // the dictionary this item belongs to
#endif
#ifdef NETDATA_INTERNAL_CHECKS
    pid_t creator_pid;
    pid_t deleter_pid;
    pid_t ll_adder_pid;
    pid_t ll_remover_pid;
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

    dict_cb_insert_t insert_callback;
    void *insert_callback_data;

    dict_cb_conflict_t conflict_callback;
    void *conflict_callback_data;

    dict_cb_react_t react_callback;
    void *react_callback_data;

    dict_cb_delete_t delete_callback;
    void *delelte_callback_data;
};

struct dictionary {
#ifdef FSANITIZE_ADDRESS
    STACKTRACE_ARRAY stacktraces;   // stack traces from all acquisition points
#endif

    usec_t last_gc_run_us;
    DICT_OPTIONS options;               // the configuration flags of the dictionary (they never change - no atomics)
    DICT_FLAGS flags;                   // run time flags for the dictionary (they change all the time - atomics needed)

    ARAL *value_aral;

    struct {                            // support for multiple indexing engines
        Pvoid_t JudyHSArray;        // the hash table
        RW_SPINLOCK rw_spinlock;        // protect the index
    } index;

    struct {
        DICTIONARY_ITEM *list;          // the double linked list of all items in the dictionary
        RW_SPINLOCK rw_spinlock;        // protect the linked-list
        SPINLOCK pending_deletion_spinlock; // protect delete flag / last-reference accounting
        pid_t writer_pid;               // the gettid() of the writer
        uint32_t writer_depth;          // nesting of write locks
    } items;

    struct dictionary_hooks *hooks;     // pointer to external function callbacks to be called at certain points
    struct dictionary_stats *stats;     // statistics data, when DICT_OPTION_STATS is set

    DICTIONARY *master;                 // the master dictionary
    DICTIONARY *next;                   // linked list for delayed destruction (garbage collection of whole dictionaries)

    uint32_t version;                   // the current version of the dictionary
                      // it is incremented when:
                      //   - item added
                      //   - item removed
                      //   - item value reset
                      //   - conflict callback returns true
                      //   - function dictionary_version_increment() is called

    int32_t entries;                   // how many items are currently in the index (the linked list may have more)
    int32_t referenced_items;          // how many items of the dictionary are currently being used by 3rd parties
    int32_t pending_deletion_items;    // how many items of the dictionary have been deleted, but have not been removed yet
    int32_t inflight;                  // how many threads are currently inside the dictionary API (see dictionary_inflight_enter)

#ifdef NETDATA_DICTIONARY_VALIDATE_POINTERS
    netdata_mutex_t global_pointer_registry_mutex;
    Pvoid_t global_pointer_registry;
#endif
};

// ----------------------------------------------------------------------------
// forward definitions of functions used in reverse order in the code

void garbage_collect_pending_deletes(DICTIONARY *dict);
static inline void item_linked_list_remove(DICTIONARY *dict, DICTIONARY_ITEM *item);
static size_t dict_item_free_with_hooks(DICTIONARY *dict, DICTIONARY_ITEM *item);
static inline const char *item_get_name(const DICTIONARY_ITEM *item);
static inline int hashtable_delete_unsafe(DICTIONARY *dict, const char *name, size_t name_len, DICTIONARY_ITEM *item);
static void item_release(DICTIONARY *dict, DICTIONARY_ITEM *item);
static bool dict_item_set_deleted(DICTIONARY *dict, DICTIONARY_ITEM *item);

#define RC_ITEM_OK                         ( 0)
#define RC_ITEM_MARKED_FOR_DELETION        (-1) // the item is marked for deletion
#define RC_ITEM_IS_CURRENTLY_BEING_DELETED (-2) // the item is currently being deleted
#define RC_ITEM_IS_CURRENTLY_BEING_CREATED (-3) // the item is currently being deleted
#define RC_ITEM_IS_REFERENCED              (-4) // the item is currently referenced
#define item_check_and_acquire(dict, item) (item_check_and_acquire_advanced(dict, item, false) == RC_ITEM_OK)
static int item_check_and_acquire_advanced(DICTIONARY *dict, DICTIONARY_ITEM *item, bool having_index_lock);
#define item_is_not_referenced_and_can_be_removed(dict, item) (item_is_not_referenced_and_can_be_removed_advanced(dict, item) == RC_ITEM_OK)
static inline int item_is_not_referenced_and_can_be_removed_advanced(DICTIONARY *dict, DICTIONARY_ITEM *item);

// ----------------------------------------------------------------------------
// validate each pointer is indexed once - internal checks only

#ifdef NETDATA_DICTIONARY_VALIDATE_POINTERS
static inline void pointer_index_init(DICTIONARY *dict __maybe_unused) {
    netdata_mutex_init(&dict->global_pointer_registry_mutex);
}

static inline void pointer_destroy_index(DICTIONARY *dict __maybe_unused) {
    netdata_mutex_lock(&dict->global_pointer_registry_mutex);
    JudyHSFreeArray(&dict->global_pointer_registry, PJE0);
    netdata_mutex_unlock(&dict->global_pointer_registry_mutex);
}
static inline void pointer_add(DICTIONARY *dict __maybe_unused, DICTIONARY_ITEM *item __maybe_unused) {
    netdata_mutex_lock(&dict->global_pointer_registry_mutex);
    Pvoid_t *PValue = JudyHSIns(&dict->global_pointer_registry, &item, sizeof(void *), PJE0);
    if(*PValue != NULL)
        fatal("pointer already exists in registry");
    *PValue = item;
    netdata_mutex_unlock(&dict->global_pointer_registry_mutex);
}

static inline void pointer_check(DICTIONARY *dict __maybe_unused, DICTIONARY_ITEM *item __maybe_unused) {
    netdata_mutex_lock(&dict->global_pointer_registry_mutex);
    Pvoid_t *PValue = JudyHSGet(dict->global_pointer_registry, &item, sizeof(void *));
    if(PValue == NULL)
        fatal("pointer is not found in registry");
    netdata_mutex_unlock(&dict->global_pointer_registry_mutex);
}

static inline void pointer_del(DICTIONARY *dict __maybe_unused, DICTIONARY_ITEM *item __maybe_unused) {
    netdata_mutex_lock(&dict->global_pointer_registry_mutex);
    int ret = JudyHSDel(&dict->global_pointer_registry, &item, sizeof(void *), PJE0);
    if(!ret)
        fatal("pointer to be deleted does not exist in registry");
    netdata_mutex_unlock(&dict->global_pointer_registry_mutex);
}
#else // !NETDATA_DICTIONARY_VALIDATE_POINTERS
#define pointer_index_init(dict) debug_dummy()
#define pointer_destroy_index(dict) debug_dummy()
#define pointer_add(dict, item) debug_dummy()
#define pointer_check(dict, item) debug_dummy()
#define pointer_del(dict, item) debug_dummy()
#endif // !NETDATA_DICTIONARY_VALIDATE_POINTERS

extern ARAL *dict_items_aral;
extern ARAL *dict_shared_items_aral;

size_t dictionary_destroy_delayed_count(void);

#include "dictionary-debug.h"
#include "dictionary-statistics.h"
#include "dictionary-locks.h"
#include "dictionary-refcount.h"
#include "dictionary-hashtable.h"
#include "dictionary-callbacks.h"
#include "dictionary-item.h"

#endif //NETDATA_DICTIONARY_INTERNALS_H
