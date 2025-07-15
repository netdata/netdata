// SPDX-License-Identifier: GPL-3.0-or-later

#include "dictionary-internals.h"

ARAL *dict_items_aral =  NULL;
ARAL *dict_shared_items_aral = NULL;

struct dictionary_stats dictionary_stats_category_other = {
    .name = "other",
};

// ----------------------------------------------------------------------------
// public locks API

inline void dictionary_write_lock(DICTIONARY *dict) {
    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);
}

inline void dictionary_write_unlock(DICTIONARY *dict) {
    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);
}

// ----------------------------------------------------------------------------
// ARAL for dict and hooks

static ARAL *ar_dict = NULL;
static ARAL *ar_hooks = NULL;

static void dictionary_init_aral(void) {
    if(ar_dict && ar_hooks) return;

    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

    if(!ar_dict)
        ar_dict = aral_by_size_acquire(sizeof(DICTIONARY));

    if(!ar_hooks)
        ar_hooks = aral_by_size_acquire(sizeof(struct dictionary_hooks));

    spinlock_unlock(&spinlock);
}

// ----------------------------------------------------------------------------
// callbacks registration

static inline void dictionary_hooks_allocate(DICTIONARY *dict) {
    if(dict->hooks) return;

    dictionary_init_aral();

    dict->hooks = aral_callocz(ar_hooks);
    dict->hooks->links = 1;

    DICTIONARY_STATS_PLUS_MEMORY(dict, 0, sizeof(struct dictionary_hooks), 0);
}

static inline size_t dictionary_hooks_free(DICTIONARY *dict) {
    if(!dict->hooks) return 0;

    REFCOUNT links = __atomic_sub_fetch(&dict->hooks->links, 1, __ATOMIC_ACQUIRE);
    if(links == 0) {
        aral_freez(ar_hooks, dict->hooks);
        dict->hooks = NULL;

        DICTIONARY_STATS_MINUS_MEMORY(dict, 0, sizeof(struct dictionary_hooks), 0);
        return sizeof(struct dictionary_hooks);
    }

    return 0;
}

void dictionary_register_insert_callback(DICTIONARY *dict, dict_cb_insert_t insert_callback, void *data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    dictionary_hooks_allocate(dict);
    dict->hooks->insert_callback = insert_callback;
    dict->hooks->insert_callback_data = data;
}

void dictionary_register_conflict_callback(DICTIONARY *dict, dict_cb_conflict_t conflict_callback, void *data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    internal_error(!(dict->options & DICT_OPTION_DONT_OVERWRITE_VALUE), "DICTIONARY: registering conflict callback without DICT_OPTION_DONT_OVERWRITE_VALUE");
    dict->options |= DICT_OPTION_DONT_OVERWRITE_VALUE;

    dictionary_hooks_allocate(dict);
    dict->hooks->conflict_callback = conflict_callback;
    dict->hooks->conflict_callback_data = data;
}

void dictionary_register_react_callback(DICTIONARY *dict, dict_cb_react_t react_callback, void *data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    dictionary_hooks_allocate(dict);
    dict->hooks->react_callback = react_callback;
    dict->hooks->react_callback_data = data;
}

void dictionary_register_delete_callback(DICTIONARY *dict, dict_cb_delete_t delete_callback,  void *data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    dictionary_hooks_allocate(dict);
    dict->hooks->delete_callback = delete_callback;
    dict->hooks->delelte_callback_data = data;
}

// ----------------------------------------------------------------------------
// dictionary statistics API

ALWAYS_INLINE
size_t dictionary_version(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;

    // this is required for views to return the right number
    // garbage_collect_pending_deletes(dict);

    return __atomic_load_n(&dict->version, __ATOMIC_RELAXED);
}

ALWAYS_INLINE
size_t dictionary_entries(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;

    // this is required for views to return the right number
    // garbage_collect_pending_deletes(dict);

    long int entries = __atomic_load_n(&dict->entries, __ATOMIC_RELAXED);
    internal_fatal(entries < 0, "DICTIONARY: entries is negative: %ld", entries);

    return entries;
}

size_t dictionary_referenced_items(DICTIONARY *dict) {
    if(unlikely(!dict)) return 0;

    long int referenced_items = __atomic_load_n(&dict->referenced_items, __ATOMIC_RELAXED);
    if(referenced_items < 0)
        fatal("DICTIONARY: referenced items is negative: %ld", referenced_items);

    return referenced_items;
}

void dictionary_version_increment(DICTIONARY *dict) {
    __atomic_fetch_add(&dict->version, 1, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// tracking allocated dictionaries

#include "dictionary-debug.h"

// ----------------------------------------------------------------------------
// items garbage collector

void garbage_collect_pending_deletes(DICTIONARY *dict) {
    usec_t last_master_deletion_us = dict->hooks?__atomic_load_n(&dict->hooks->last_master_deletion_us, __ATOMIC_RELAXED):0;
    usec_t last_gc_run_us = __atomic_load_n(&dict->last_gc_run_us, __ATOMIC_RELAXED);

    bool is_view = is_view_dictionary(dict);

    if(likely(!(
            DICTIONARY_PENDING_DELETES_GET(dict) > 0 ||
            (is_view && last_master_deletion_us > last_gc_run_us)
            )))
        return;

    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);

    __atomic_store_n(&dict->last_gc_run_us, now_realtime_usec(), __ATOMIC_RELAXED);

    if(is_view)
        dictionary_index_lock_wrlock(dict);

    DICTIONARY_STATS_GARBAGE_COLLECTIONS_PLUS1(dict);

    size_t deleted = 0, pending = 0, examined = 0;
    DICTIONARY_ITEM *item = dict->items.list, *item_next;
    while(item) {
        examined++;

        // this will clean up
        item_next = item->next;
        int rc = item_check_and_acquire_advanced(dict, item, is_view);

        if(rc == RC_ITEM_MARKED_FOR_DELETION) {
            // we didn't get a reference

            if(item_is_not_referenced_and_can_be_removed(dict, item)) {
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(dict->items.list, item, prev, next);
                dict_item_free_with_hooks(dict, item);
                deleted++;

                pending = DICTIONARY_PENDING_DELETES_MINUS1(dict);
                if (!pending)
                    break;
            }
        }
        else if(rc == RC_ITEM_IS_CURRENTLY_BEING_DELETED)
            ; // do not touch this item (we didn't get a reference)

        else if(rc == RC_ITEM_OK)
            item_release(dict, item);

        item = item_next;
    }

    if(is_view)
        dictionary_index_wrlock_unlock(dict);

    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);

    (void)deleted;
    (void)examined;

    dictionary_internal_error(false, dict, "DICTIONARY: garbage collected dictionary, "
                          "examined %zu items, deleted %zu items, still pending %zu items",
                          examined, deleted, pending);
}

void dictionary_garbage_collect(DICTIONARY *dict) {
    if(!dict) return;
    garbage_collect_pending_deletes(dict);
}

// ----------------------------------------------------------------------------

void dictionary_static_items_aral_init(void) {
    static SPINLOCK spinlock;

    if(unlikely(!dict_items_aral || !dict_shared_items_aral)) {
        spinlock_lock(&spinlock);

        if(!dict_items_aral)
            dict_items_aral = aral_by_size_acquire(sizeof(DICTIONARY_ITEM));

        if(!dict_shared_items_aral)
            dict_shared_items_aral = aral_by_size_acquire(sizeof(DICTIONARY_ITEM_SHARED));

        spinlock_unlock(&spinlock);
    }
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
#endif

    // destroy the index
    dictionary_index_lock_wrlock(dict);
    index_size += hashtable_destroy_unsafe(dict);
    dictionary_index_wrlock_unlock(dict);

    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);
    DICTIONARY_ITEM *item = dict->items.list;
    while (item) {
        // cache item->next
        // because we are going to free item
        DICTIONARY_ITEM *item_next = item->next;

        item_size += dict_item_free_with_hooks(dict, item);
        item = item_next;

        // to speed up destruction, we don't unlink the item
        // from the linked-list here

        counted_items++;
    }
    dict->items.list = NULL;
    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);

    dict_size += dictionary_locks_destroy(dict);
    dict_size += reference_counter_free(dict);
    dict_size += dictionary_hooks_free(dict);
    dict_size += sizeof(DICTIONARY);
    DICTIONARY_STATS_MINUS_MEMORY(dict, 0, sizeof(DICTIONARY), 0);

    if(dict->value_aral)
        aral_by_size_release(dict->value_aral);

    dictionary_debug_untrack_dict(dict);
    aral_freez(ar_dict, dict);

    internal_error(
        false,
        "DICTIONARY: Freed dictionary having %ld (counted %zu) entries, %ld referenced, %ld pending deletion, total freed memory: %zu bytes (sizeof(dict) = %zu, sizeof(item) = %zu).",
        entries, counted_items, referenced_items, pending_deletion_items,
        dict_size + item_size, sizeof(DICTIONARY), sizeof(DICTIONARY_ITEM) + sizeof(DICTIONARY_ITEM_SHARED));

    if(mem)
        *mem = dict_size + item_size + index_size;

    return true;
}

netdata_mutex_t dictionaries_waiting_to_be_destroyed_mutex = NETDATA_MUTEX_INITIALIZER;
static DICTIONARY *dictionaries_waiting_to_be_destroyed = NULL;

#ifdef FSANITIZE_ADDRESS
DEFINE_JUDYL_TYPED(STACKTRACE, size_t);
#endif

static void dictionary_queue_for_destruction(DICTIONARY *dict) {
    if(is_dictionary_destroyed(dict))
        return;

    DICTIONARY_STATS_DICT_DESTROY_QUEUED_PLUS1(dict);
    dict_flag_set(dict, DICT_FLAG_DESTROYED);

    netdata_mutex_lock(&dictionaries_waiting_to_be_destroyed_mutex);

    dict->next = dictionaries_waiting_to_be_destroyed;
    dictionaries_waiting_to_be_destroyed = dict;

    netdata_mutex_unlock(&dictionaries_waiting_to_be_destroyed_mutex);
}

size_t dictionary_destroy_delayed_count(void) {
    netdata_mutex_lock(&dictionaries_waiting_to_be_destroyed_mutex);

    size_t count = 0;
    for(DICTIONARY *dict = dictionaries_waiting_to_be_destroyed, *next; dict ; dict = next) {
        next = dict->next;
        count++;
    }

    netdata_mutex_unlock(&dictionaries_waiting_to_be_destroyed_mutex);
    return count;
}

size_t cleanup_destroyed_dictionaries(bool shutdown __maybe_unused)
{
    if (netdata_mutex_trylock(&dictionaries_waiting_to_be_destroyed_mutex) != 0)
        return 0;

    if (!dictionaries_waiting_to_be_destroyed) {
        netdata_mutex_unlock(&dictionaries_waiting_to_be_destroyed_mutex);
        return 0;
    }

    size_t remaining = 0;

#ifdef FSANITIZE_ADDRESS
    // Create Judy arrays for tracking stats by stacktrace
    STACKTRACE_JudyLSet dict_counts = { 0 };     // Count of dictionaries per stacktrace
    STACKTRACE_JudyLSet item_counts = { 0 };     // Count of items per stacktrace
    STACKTRACE_JudyLSet item_stacktrace_counts = { 0 }; // Count of items by their creation stacktrace
    
    STACKTRACE_INIT(&dict_counts);
    STACKTRACE_INIT(&item_counts);
    STACKTRACE_INIT(&item_stacktrace_counts);
#endif

    DICTIONARY *dict, *last = NULL, *next = NULL;
    for(dict = dictionaries_waiting_to_be_destroyed; dict ; dict = next) {
        next = dict->next;

        DICTIONARY_STATS_DICT_DESTROY_QUEUED_MINUS1(dict);
        if(dictionary_free_all_resources(dict, NULL, false)) {
            if(last) last->next = next;
            else dictionaries_waiting_to_be_destroyed = next;
        }
        else {
#ifdef FSANITIZE_ADDRESS
            size_t ref_items = dictionary_referenced_items(dict);

            // Track this dictionary for deduplication reporting
            if (shutdown) {
                // Process all stacktraces from this dictionary
                for (int i = 0; i < dict->stacktraces.num_stacktraces; i++) {
                    if (dict->stacktraces.stacktraces[i]) {
                        // Update dictionary count
                        uintptr_t key = (uintptr_t)dict->stacktraces.stacktraces[i];
                        size_t dict_count = STACKTRACE_GET(&dict_counts, key);
                        dict_count++;
                        STACKTRACE_SET(&dict_counts, key, dict_count);

                        // Update item count
                        size_t items_count = STACKTRACE_GET(&item_counts, key);
                        items_count += ref_items;
                        STACKTRACE_SET(&item_counts, key, items_count);
                    }
                }
            }
#endif

            DICTIONARY_STATS_DICT_DESTROY_QUEUED_PLUS1(dict);
            last = dict;
            remaining++;
        }
    }

#ifdef FSANITIZE_ADDRESS
    if (remaining > 0 && shutdown) {
        // Print deduplicated report
        fprintf(stderr, "WARNING: There are %zu dictionaries with references in them, that cannot be destroyed.\n", 
                remaining);
        
        // Buffer for formatting stack traces
        BUFFER *wb = buffer_create(16384, NULL);
        
        // Print each unique stacktrace group
        Word_t stacktrace_idx = 0;
        size_t i = 0;
        
        // Get the first key from dict_counts (both Judy arrays have the same keys)
        for(size_t key_index = STACKTRACE_FIRST(&dict_counts, &stacktrace_idx);
             key_index;
             key_index = STACKTRACE_NEXT(&dict_counts, &stacktrace_idx)) {

            i++;
            STACKTRACE st = (STACKTRACE)stacktrace_idx;
            size_t dict_count = STACKTRACE_GET(&dict_counts, stacktrace_idx);
            size_t item_count = STACKTRACE_GET(&item_counts, stacktrace_idx);
            
            // Format stacktrace to buffer
            buffer_flush(wb);
            stacktrace_to_buffer(st, wb);
            
            fprintf(stderr, "\n > DICTIONARY DELAYED %zu: %zu items in %zu dictionaries accessed from:\n%s\n\n",
                    i, item_count, dict_count, buffer_tostring(wb));
        }
        
        // Clean up
        // Now collect and report information about dictionary items grouped by their creation stacktrace
            fprintf(stderr, "\n========= DICTIONARY ITEMS GROUPED BY CREATION STACKTRACE =========\n");
            
        // Loop through dictionaries to collect item stacktraces
        for(dict = dictionaries_waiting_to_be_destroyed; dict; dict = dict->next) {
            // Iterate through all items and count by stacktrace
            DICTIONARY_ITEM *item;
            for(item = dict->items.list; item; item = item->next) {
                // Count each unique stacktrace in the item's array
                for (int i = 0; i < item->stacktraces.num_stacktraces; i++) {
                    if (item->stacktraces.stacktraces[i]) {
                        uintptr_t item_key = (uintptr_t)item->stacktraces.stacktraces[i];
                        size_t count = STACKTRACE_GET(&item_stacktrace_counts, item_key);
                        count++;
                        STACKTRACE_SET(&item_stacktrace_counts, item_key, count);
                    }
                }
            }
        }

        // Print report of items by stacktrace
        Word_t item_st_idx = 0;
        size_t j = 0;

        for(size_t key_index = STACKTRACE_FIRST(&item_stacktrace_counts, &item_st_idx);
             key_index;
             key_index = STACKTRACE_NEXT(&item_stacktrace_counts, &item_st_idx)) {

            j++;
            STACKTRACE st = (STACKTRACE)item_st_idx;
            size_t count = STACKTRACE_GET(&item_stacktrace_counts, item_st_idx);

            // Format stacktrace to buffer
            buffer_flush(wb);
            stacktrace_to_buffer(st, wb);

            fprintf(stderr, "\n > DICTIONARY ITEMS DELAYED %zu: %zu items accessed from:\n%s\n\n",
                    j, count, buffer_tostring(wb));
        }

        fprintf(stderr, "Total: %zu dictionaries\n", j);
        fprintf(stderr, "==================================================================\n");

        STACKTRACE_FREE(&dict_counts, NULL, NULL);
        STACKTRACE_FREE(&item_counts, NULL, NULL);
        STACKTRACE_FREE(&item_stacktrace_counts, NULL, NULL);
        
        buffer_free(wb);
    }
#endif

    netdata_mutex_unlock(&dictionaries_waiting_to_be_destroyed_mutex);

    return remaining;
}

// ----------------------------------------------------------------------------
// API internal checks

// Use the debug version from dictionary-debug.h
#define api_internal_check(dict, item, allow_null_dict, allow_null_item) dictionary_debug_internal_check(dict, item, allow_null_dict, allow_null_item)

#define api_is_name_good(dict, name, name_len) api_is_name_good_with_trace(dict, name, name_len, __FUNCTION__)
static bool api_is_name_good_with_trace(DICTIONARY *dict __maybe_unused, const char *name, ssize_t name_len __maybe_unused, const char *function __maybe_unused) {
    if(unlikely(!name)) {
        dictionary_internal_error(true, dict,
            "DICTIONARY: attempted to %s() with name = NULL on a dictionary",
            function);
        return false;
    }

    if(unlikely(!*name)) {
        dictionary_internal_error(true, dict,
            "DICTIONARY: attempted to %s() with empty name on a dictionary",
            function);
        return false;
    }

    dictionary_internal_error(
        name_len > 0 && name_len != (ssize_t)strlen(name), dict,
        "DICTIONARY: attempted to %s() with a name of '%s', having length of %zu, "
        "but the supplied name_len = %ld",
        function,
        name,
        strlen(name),
        (long int) name_len);

    dictionary_internal_error(
        name_len <= 0 && name_len != -1, dict,
        "DICTIONARY: attempted to %s() with a name of '%s', having length of %zu, "
        "but the supplied name_len = %ld",
        function,
        name,
        strlen(name),
        (long int) name_len);

    return true;
}

// ----------------------------------------------------------------------------
// API - dictionary management

static DICTIONARY *dictionary_create_internal(DICT_OPTIONS options, struct dictionary_stats *stats, size_t fixed_size) {
    dictionary_init_aral();
    cleanup_destroyed_dictionaries(false);

    DICTIONARY *dict = aral_callocz(ar_dict);
    dict->options = options;
    dict->stats = stats;

    if((dict->options & DICT_OPTION_FIXED_SIZE) && !fixed_size) {
        dict->options &= ~DICT_OPTION_FIXED_SIZE;
        internal_fatal(true, "DICTIONARY: requested fixed size dictionary, without setting the size");
    }
    if(!(dict->options & DICT_OPTION_FIXED_SIZE) && fixed_size) {
        dict->options |= DICT_OPTION_FIXED_SIZE;
        internal_fatal(true, "DICTIONARY: set a fixed size for the items, without setting DICT_OPTION_FIXED_SIZE flag");
    }

    if(dict->options & DICT_OPTION_FIXED_SIZE)
        dict->value_aral = aral_by_size_acquire(fixed_size);
    else
        dict->value_aral = NULL;

//    if(!(dict->options & (DICT_OPTION_INDEX_JUDY|DICT_OPTION_INDEX_HASHTABLE)))
    dict->options |= DICT_OPTION_INDEX_JUDY;

    size_t dict_size = 0;
    dict_size += sizeof(DICTIONARY);
    dict_size += dictionary_locks_init(dict);
    dict_size += reference_counter_init(dict);
    dict_size += hashtable_init_unsafe(dict);

    dictionary_static_items_aral_init();
    pointer_index_init(dict);

    DICTIONARY_STATS_PLUS_MEMORY(dict, 0, dict_size, 0);

    return dict;
}

// Helper function to add a stacktrace to a dictionary
#ifdef FSANITIZE_ADDRESS
static inline void dict_add_stacktrace(DICTIONARY *dict) {
    stacktrace_array_add(&dict->stacktraces, 1);
}
#endif

DICTIONARY *dictionary_create_advanced(DICT_OPTIONS options, struct dictionary_stats *stats, size_t fixed_size) {
    DICTIONARY *dict = dictionary_create_internal(options, stats?stats:&dictionary_stats_category_other, fixed_size);

#ifdef FSANITIZE_ADDRESS
    // Initialize stacktrace tracking
    stacktrace_array_init(&dict->stacktraces);
    
    // Add the first stack trace at creation time
    dict_add_stacktrace(dict);
#endif

    DICTIONARY_STATS_DICT_CREATIONS_PLUS1(dict);
    dictionary_debug_track_dict(dict);
    return dict;
}

DICTIONARY *dictionary_create_view(DICTIONARY *master) {
    DICTIONARY *dict = dictionary_create_internal(master->options, master->stats,
                                                  master->value_aral ? aral_requested_element_size(master->value_aral) : 0);

    dict->master = master;

    dictionary_hooks_allocate(master);

    if(unlikely(__atomic_load_n(&master->hooks->links, __ATOMIC_RELAXED)) < 1)
        fatal("DICTIONARY: attempted to create a view that has %d links", master->hooks->links);

    dict->hooks = master->hooks;
    __atomic_add_fetch(&master->hooks->links, 1, __ATOMIC_ACQUIRE);

#ifdef FSANITIZE_ADDRESS
    // Initialize stacktrace tracking
    stacktrace_array_init(&dict->stacktraces);
    
    // Add the first stack trace at creation time
    dict_add_stacktrace(dict);
#endif

    DICTIONARY_STATS_DICT_CREATIONS_PLUS1(dict);
    dictionary_debug_track_dict(dict);
    return dict;
}

void dictionary_flush(DICTIONARY *dict) {
    if(unlikely(!dict))
        return;

    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);

    DICTIONARY_ITEM *item, *next = NULL;
    for(item = dict->items.list; item ;item = next) {
        next = item->next;
        dict_item_del(dict, item_get_name(item), (ssize_t)item_get_name_len(item));
    }

    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);

    DICTIONARY_STATS_DICT_FLUSHES_PLUS1(dict);

    dictionary_garbage_collect(dict);
}

size_t dictionary_destroy(DICTIONARY *dict) {
    cleanup_destroyed_dictionaries(false);

    if(!dict) return 0;

    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);

    DICTIONARY_STATS_DICT_DESTRUCTIONS_PLUS1(dict);

    size_t referenced_items = dictionary_referenced_items(dict);
    if(referenced_items) {
        dictionary_flush(dict);
        dictionary_queue_for_destruction(dict);

        dictionary_internal_error(
            true, dict,
            "DICTIONARY: delaying destruction of dictionary, because it has %d referenced items in it (%d total).",
            dict->referenced_items, dict->entries);

        ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);
        return 0;
    }

    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);

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

    DICTIONARY_ITEM *item =
        dict_item_add_or_reset_value_and_acquire(dict, name, name_len, value, value_len, constructor_data, NULL);
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

    garbage_collect_pending_deletes(dict);

    dictionary_acquired_item_dup(dict->master, master_item);
    DICTIONARY_ITEM *item = dict_item_add_or_reset_value_and_acquire(dict, name, name_len, NULL, 0, NULL, master_item);
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
    DICTIONARY_ITEM *item = dict_item_find_and_acquire(dict, name, name_len);
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

ALWAYS_INLINE
DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_acquired_item_dup(DICTIONARY *dict, DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    // we allow the item to be NULL here
    api_internal_check(dict, item, false, true);

    if(likely(item)) {
        item_acquire(dict, item);
        api_internal_check(dict, item, false, false);
    }

    return item;
}

ALWAYS_INLINE
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

ALWAYS_INLINE
const char *dictionary_acquired_item_name(DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    return item_get_name(item);
}

ALWAYS_INLINE
void *dictionary_acquired_item_value(DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    if(likely(item))
        return item->shared->value;

    return NULL;
}

size_t dictionary_acquired_item_references(DICT_ITEM_CONST DICTIONARY_ITEM *item) {
    if(likely(item))
        return DICTIONARY_ITEM_REFCOUNT_GET_SOLE(item);

    return 0;
}

// ----------------------------------------------------------------------------
// DEL an item

bool dictionary_del_advanced(DICTIONARY *dict, const char *name, ssize_t name_len) {
    if(unlikely(!api_is_name_good(dict, name, name_len)))
        return false;

    api_internal_check(dict, NULL, false, true);

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to delete item on a destroyed dictionary");
        return false;
    }

    return dict_item_del(dict, name, name_len);
}
