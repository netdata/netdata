// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_ITEM_H
#define NETDATA_DICTIONARY_ITEM_H

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// ITEM initialization and updates

static inline size_t item_set_name(DICTIONARY *dict, DICTIONARY_ITEM *item, const char *name, size_t name_len) {
    if(likely(dict->options & DICT_OPTION_NAME_LINK_DONT_CLONE)) {
        item->caller_name = (char *)name;
        item->key_len = name_len;
    }
    else {
        item->string_name = string_strdupz(name);
        item->key_len = string_strlen(item->string_name);
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

static inline size_t item_get_name_len(const DICTIONARY_ITEM *item) {
    if(item->options & ITEM_OPTION_ALLOCATED_NAME)
        return string_strlen(item->string_name);
    else
        return strlen(item->caller_name);
}

// ----------------------------------------------------------------------------

static inline DICTIONARY_ITEM *dict_item_create(DICTIONARY *dict __maybe_unused, size_t *allocated_bytes, DICTIONARY_ITEM *master_item) {
    DICTIONARY_ITEM *item;

    size_t size = sizeof(DICTIONARY_ITEM);
    item = aral_mallocz(dict_items_aral);
    memset(item, 0, sizeof(DICTIONARY_ITEM));

#ifdef NETDATA_INTERNAL_CHECKS
    item->creator_pid = gettid_cached();
#endif

    item->refcount = 1;
    item->flags = ITEM_FLAG_BEING_CREATED;

    *allocated_bytes += size;

    if(master_item) {
        item->shared = master_item->shared;

        if(unlikely(__atomic_add_fetch(&item->shared->links, 1, __ATOMIC_ACQUIRE) <= 1))
            fatal("DICTIONARY: attempted to link to a shared item structure that had zero references");
    }
    else {
        size = sizeof(DICTIONARY_ITEM_SHARED);
        item->shared = aral_mallocz(dict_shared_items_aral);
        memset(item->shared, 0, sizeof(DICTIONARY_ITEM_SHARED));

        item->shared->links = 1;
        *allocated_bytes += size;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    item->dict = dict;
#endif
    return item;
}

static inline void *dict_item_value_mallocz(DICTIONARY *dict, size_t value_len) {
    if(dict->value_aral) {
        internal_fatal(aral_requested_element_size(dict->value_aral) != value_len,
                       "DICTIONARY: item value size %zu does not match the configured fixed one %zu",
                       value_len, aral_requested_element_size(dict->value_aral));
        return aral_mallocz(dict->value_aral);
    }
    else
        return mallocz(value_len);
}

static inline void dict_item_value_freez(DICTIONARY *dict, void *ptr) {
    if(dict->value_aral)
        aral_freez(dict->value_aral, ptr);
    else
        freez(ptr);
}

static inline void *dict_item_value_create(DICTIONARY *dict, void *value, size_t value_len) {
    void *ptr = NULL;

    if(likely(value_len)) {
        if (likely(value)) {
            // a value has been supplied
            // copy it
            ptr =  dict_item_value_mallocz(dict, value_len);
            memcpy(ptr, value, value_len);
        }
        else {
            // no value has been supplied
            // allocate a clear memory block
            ptr = dict_item_value_mallocz(dict, value_len);
            memset(ptr, 0, value_len);
        }
    }
    // else
    // the caller wants an item without any value

    return ptr;
}

static inline DICTIONARY_ITEM *dict_item_create_with_hooks(DICTIONARY *dict, const char *name, size_t name_len, void *value, size_t value_len, void *constructor_data, DICTIONARY_ITEM *master_item) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(name_len > KEY_LEN_MAX))
        fatal("DICTIONARY: tried to index a key of size %zu, but the maximum acceptable is %zu", name_len, (size_t)KEY_LEN_MAX);

    if(unlikely(value_len > VALUE_LEN_MAX))
        fatal("DICTIONARY: tried to add an item of size %zu, but the maximum acceptable is %zu", value_len, (size_t)VALUE_LEN_MAX);
#endif

    size_t item_size = 0, key_size = 0, value_size = 0;

    DICTIONARY_ITEM *item = dict_item_create(dict, &item_size, master_item);
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

        if(unlikely(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE))
            item->shared->value = value;
        else
            item->shared->value = dict_item_value_create(dict, value, value_len);

        item->shared->value_len = value_len;
        value_size += value_len;

        dictionary_execute_insert_callback(dict, item, constructor_data);
    }

    DICTIONARY_ENTRIES_PLUS1(dict);
    DICTIONARY_STATS_PLUS_MEMORY(dict, key_size, item_size, value_size);

    return item;
}

static inline void dict_item_reset_value_with_hooks(DICTIONARY *dict, DICTIONARY_ITEM *item, void *value, size_t value_len, void *constructor_data) {
    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: %s() should never be called on views.", __FUNCTION__ );

    netdata_log_debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", item_get_name(item));

    DICTIONARY_VALUE_RESETS_PLUS1(dict);

    if(item->shared->value_len != value_len) {
        DICTIONARY_STATS_PLUS_MEMORY(dict, 0, 0, value_len);
        DICTIONARY_STATS_MINUS_MEMORY(dict, 0, 0, item->shared->value_len);
    }

    dictionary_execute_delete_callback(dict, item);

    if(likely(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE)) {
        netdata_log_debug(D_DICTIONARY, "Dictionary: linking value to '%s'", item_get_name(item));
        item->shared->value = value;
        item->shared->value_len = value_len;
    }
    else {
        netdata_log_debug(D_DICTIONARY, "Dictionary: cloning value to '%s'", item_get_name(item));

        void *old_value = item->shared->value;
        void *new_value = NULL;
        if(value_len) {
            new_value = dict_item_value_mallocz(dict, value_len);
            if(value) memcpy(new_value, value, value_len);
            else memset(new_value, 0, value_len);
        }
        item->shared->value = new_value;
        item->shared->value_len = value_len;

        netdata_log_debug(D_DICTIONARY, "Dictionary: freeing old value of '%s'", item_get_name(item));
        dict_item_value_freez(dict, old_value);
    }

    dictionary_execute_insert_callback(dict, item, constructor_data);
}

static inline size_t dict_item_free_with_hooks(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    netdata_log_debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", item_get_name(item));

    if(!item_flag_check(item, ITEM_FLAG_DELETED))
        DICTIONARY_ENTRIES_MINUS1(dict);

    size_t item_size = 0, key_size = 0, value_size = 0;

    key_size += item->key_len;

    if(item_shared_release_and_check_if_it_can_be_freed(dict, item)) {
        dictionary_execute_delete_callback(dict, item);

        if(unlikely(!(dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE))) {
            netdata_log_debug(D_DICTIONARY, "Dictionary freeing value of '%s'", item_get_name(item));
            dict_item_value_freez(dict, item->shared->value);
            item->shared->value = NULL;
        }
        value_size += item->shared->value_len;

        aral_freez(dict_shared_items_aral, item->shared);
        item->shared = NULL;
        item_size += sizeof(DICTIONARY_ITEM_SHARED);
    }

    // free the name after calling the delete callback
    if(unlikely(!(dict->options & DICT_OPTION_NAME_LINK_DONT_CLONE)))
        item_free_name(dict, item);

    aral_freez(dict_items_aral, item);

    item_size += sizeof(DICTIONARY_ITEM);

    DICTIONARY_STATS_MINUS_MEMORY(dict, key_size, item_size, value_size);

    // we return the memory we actually freed
    return item_size + ((dict->options & DICT_OPTION_VALUE_LINK_DONT_CLONE) ? 0 : value_size);
}

// ----------------------------------------------------------------------------
// linked list management

static inline void item_linked_list_add(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);

    if(dict->options & DICT_OPTION_ADD_IN_FRONT)
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(dict->items.list, item, prev, next);
    else
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(dict->items.list, item, prev, next);

#ifdef NETDATA_INTERNAL_CHECKS
    item->ll_adder_pid = gettid_cached();
#endif

    // clear the BEING created flag,
    // after it has been inserted into the linked list
    item_flag_clear(item, ITEM_FLAG_BEING_CREATED);

    garbage_collect_pending_deletes(dict);
    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);
}

static inline void item_linked_list_remove(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    ll_recursive_lock(dict, DICTIONARY_LOCK_WRITE);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(dict->items.list, item, prev, next);

#ifdef NETDATA_INTERNAL_CHECKS
    item->ll_remover_pid = gettid_cached();
#endif

    garbage_collect_pending_deletes(dict);
    ll_recursive_unlock(dict, DICTIONARY_LOCK_WRITE);
}

// ----------------------------------------------------------------------------
// item operations

static inline void dict_item_shared_set_deleted(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    if(is_master_dictionary(dict)) {
        item_shared_flag_set(item, ITEM_FLAG_DELETED);

        if(dict->hooks)
            __atomic_store_n(&dict->hooks->last_master_deletion_us, now_realtime_usec(), __ATOMIC_RELAXED);
    }
}

// returns true if we set the deleted flag on this item
static inline bool dict_item_set_deleted(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    ITEM_FLAGS expected, desired;

    expected = __atomic_load_n(&item->flags, __ATOMIC_RELAXED);

    do {

        if (expected & ITEM_FLAG_DELETED)
            return false;

        desired = expected | ITEM_FLAG_DELETED;

    } while(!__atomic_compare_exchange_n(&item->flags, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    DICTIONARY_ENTRIES_MINUS1(dict);
    return true;
}

static inline void dict_item_free_or_mark_deleted(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    int rc = item_is_not_referenced_and_can_be_removed_advanced(dict, item);
    switch(rc) {
        case RC_ITEM_OK:
            // the item is ours, refcount set to -100
            dict_item_shared_set_deleted(dict, item);
            item_linked_list_remove(dict, item);
            dict_item_free_with_hooks(dict, item);
            break;

        case RC_ITEM_IS_REFERENCED:
        case RC_ITEM_IS_CURRENTLY_BEING_CREATED:
            // the item is currently referenced by others
            dict_item_shared_set_deleted(dict, item);
            dict_item_set_deleted(dict, item);
            // after this point do not touch the item
            break;

        case RC_ITEM_IS_CURRENTLY_BEING_DELETED:
            // an item that is currently being deleted by someone else - don't touch it
            break;

        default:
            internal_error(true, "Hey dev! You forgot to add the new condition here!");
            break;
    }
}

// this is used by traversal functions to remove the current item
// if it is deleted, and it has zero references. This will eliminate
// the need for the garbage collector to kick-in later.
// Most deletions happen during traversal, so this is a nice hack
// to speed up everything!
static inline void dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(DICTIONARY *dict, DICTIONARY_ITEM *item, char rw) {
    if(rw == DICTIONARY_LOCK_WRITE) {
        bool should_be_deleted = item_flag_check(item, ITEM_FLAG_DELETED);

        item_release(dict, item);

        if(should_be_deleted && item_is_not_referenced_and_can_be_removed(dict, item)) {
            // this has to be before removing from the linked list,
            // otherwise the garbage collector will also kick in!
            DICTIONARY_PENDING_DELETES_MINUS1(dict);

            item_linked_list_remove(dict, item);
            dict_item_free_with_hooks(dict, item);
        }
    }
    else {
        // we can't do anything under this mode
        item_release(dict, item);
    }
}

static inline bool dict_item_del(DICTIONARY *dict, const char *name, ssize_t name_len) {
    if(name_len == -1)
        name_len = (ssize_t)strlen(name);

    netdata_log_debug(D_DICTIONARY, "DEL dictionary entry with name '%s'.", name);

    // Unfortunately, the JudyHSDel() does not return the value of the
    // item that was deleted, so we have to find it before we delete it,
    // since we need to release our structures too.

    dictionary_index_lock_wrlock(dict);

    int ret;
    DICTIONARY_ITEM *item = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(!item)) {
        dictionary_index_wrlock_unlock(dict);
        ret = false;
    }
    else {
        if(hashtable_delete_unsafe(dict, name, name_len, item) == 0)
            netdata_log_error("DICTIONARY: INTERNAL ERROR: tried to delete item with name '%s', "
                              "name_len %zd that is not in the index",
                              name, name_len);
        else
            pointer_del(dict, item);

        dictionary_index_wrlock_unlock(dict);

        dict_item_free_or_mark_deleted(dict, item);
        ret = true;
    }

    return ret;
}

static inline DICTIONARY_ITEM *dict_item_add_or_reset_value_and_acquire(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data, DICTIONARY_ITEM *master_item) {
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
        name_len = (ssize_t)strlen(name);

    netdata_log_debug(D_DICTIONARY, "SET dictionary entry with name '%s'.", name);

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
        void *handle = hashtable_insert_unsafe(dict, name, name_len);
        item = hashtable_insert_handle_to_item_unsafe(dict, handle);
        if (likely(item == NULL)) {
            // a new item added to the index

            // create the dictionary item
            item = dict_item_create_with_hooks(dict, name, name_len, value, value_len, constructor_data, master_item);

            pointer_add(dict, item);

            hashtable_set_item_unsafe(dict, handle, item);

            // unlock the index lock, before we add it to the linked list
            // DON'T DO IT THE OTHER WAY AROUND - DO NOT CROSS THE LOCKS!
            dictionary_index_wrlock_unlock(dict);

            item_linked_list_add(dict, item);

            added_or_updated = true;
        }
        else {
            pointer_check(dict, item);

            if(item_check_and_acquire_advanced(dict, item, true) != RC_ITEM_OK) {
                spins++;
                continue;
            }

            // the item is already in the index
            // so, either we will return the old one
            // or overwrite the value, depending on dictionary flags

            // We should not compare the values here!
            // even if they are the same, we have to do the whole job
            // so that the callbacks will be called.

            if(is_view_dictionary(dict)) {
                // view dictionary
                // the item is already there and can be used
                if(item->shared != master_item->shared)
                    netdata_log_error("DICTIONARY: changing the master item on a view is not supported. The previous item will remain. To change the key of an item in a view, delete it and add it again.");
            }
            else {
                // master dictionary
                // the user wants to reset its value

                if (!(dict->options & DICT_OPTION_DONT_OVERWRITE_VALUE)) {
                    dict_item_reset_value_with_hooks(dict, item, value, value_len, constructor_data);
                    added_or_updated = true;
                }

                else if (dictionary_execute_conflict_callback(dict, item, value, constructor_data)) {
                    dictionary_version_increment(dict);
                    added_or_updated = true;
                }

                else {
                    // conflict callback returned false
                    // we did really nothing!
                    ;
                }
            }

            dictionary_index_wrlock_unlock(dict);
        }
    } while(!item);


    if(unlikely(spins > 0))
        DICTIONARY_STATS_INSERT_SPINS_PLUS(dict, spins);

    if(is_master_dictionary(dict) && added_or_updated)
        dictionary_execute_react_callback(dict, item, constructor_data);

    return item;
}

static inline DICTIONARY_ITEM *dict_item_find_and_acquire(DICTIONARY *dict, const char *name, ssize_t name_len) {
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
        name_len = (ssize_t)strlen(name);

    netdata_log_debug(D_DICTIONARY, "GET dictionary entry with name '%s'.", name);

    dictionary_index_lock_rdlock(dict);

    DICTIONARY_ITEM *item = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(item && !item_check_and_acquire(dict, item))) {
        item = NULL;
        DICTIONARY_STATS_SEARCH_IGNORES_PLUS1(dict);
    }

    dictionary_index_rdlock_unlock(dict);

    return item;
}


#endif //NETDATA_DICTIONARY_ITEM_H
