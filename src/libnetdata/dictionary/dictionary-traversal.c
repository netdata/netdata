// SPDX-License-Identifier: GPL-3.0-or-later

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// RCU-safe pointer helpers for dictionary traversal.
// Under RCU mode, readers load list/next pointers concurrently with writers
// (who hold the write lock but NOT the RCU read-side CS). We must use atomic
// acquire loads so the C11 memory model is satisfied. On x86 these compile
// to plain loads (TSO guarantees), so there is no runtime cost.

static inline DICTIONARY_ITEM *dict_item_list_head(DICTIONARY *dict, bool rcu_mode) {
    if(rcu_mode)
        return __atomic_load_n(&dict->items.list, __ATOMIC_ACQUIRE);
    else
        return dict->items.list;
}

static inline DICTIONARY_ITEM *dict_item_next(DICTIONARY_ITEM *item, bool rcu_mode) {
    if(rcu_mode)
        return __atomic_load_n(&item->next, __ATOMIC_ACQUIRE);
    else
        return item->next;
}

// ----------------------------------------------------------------------------
// traversal with loop

void *dictionary_foreach_start_rw(DICTFE *dfe) {
    if(unlikely(!dfe || !dfe->dict)) return NULL;

    DICTIONARY_STATS_TRAVERSALS_PLUS1(dfe->dict);

    if(unlikely(is_dictionary_destroyed(dfe->dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_foreach_start_rw() on a destroyed dictionary");
        dfe->dict = NULL;
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
        dfe->counter = 0;
        return NULL;
    }

    // Determine if we should use RCU mode for this traversal.
    // RCU mode: no per-item refcount acquisition during traversal.
    // Only for pure READ mode on multi-threaded dictionaries.
    dfe->rcu_mode = ll_lock_is_rcu_mode(dfe->dict, dfe->rw);

    dfe->counter = 0;
    dfe->locked = true;
    ll_recursive_lock(dfe->dict, dfe->rw);

    // Re-check under the lock — dictionary_destroy() sets the flag while
    // holding this lock, so this is the synchronized check.
    if(unlikely(is_dictionary_destroyed(dfe->dict))) {
        ll_recursive_unlock(dfe->dict, dfe->rw);
        dfe->locked = false;
        dfe->dict = NULL;
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
        dfe->counter = 0;
        return NULL;
    }

    // get the first item from the list
    DICTIONARY_ITEM *item = dict_item_list_head(dfe->dict, dfe->rcu_mode);

    if(dfe->rcu_mode) {
        // RCU mode: skip deleted items without acquiring refcount
        while(item && !item_is_available_for_rcu_traversal(item))
            item = dict_item_next(item, true);
    }
    else {
        // Legacy mode: skip deleted items with CAS refcount acquisition
        while(item && !item_check_and_acquire(dfe->dict, item))
            item = item->next;
    }

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

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT)) {
        ll_recursive_unlock(dfe->dict, dfe->rw);
        dfe->locked = false;
    }

    return dfe->value;
}

ALWAYS_INLINE void *dictionary_foreach_next(DICTFE *dfe) {
    if(unlikely(!dfe || !dfe->dict)) return NULL;

    if(unlikely(is_dictionary_destroyed(dfe->dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_foreach_next() on a destroyed dictionary");
        dictionary_foreach_done(dfe);
        return NULL;
    }

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT) || !dfe->locked) {
        ll_recursive_lock(dfe->dict, dfe->rw);
        dfe->locked = true;

        if(unlikely(is_dictionary_destroyed(dfe->dict))) {
            // Unlock before foreach_done — in reentrant mode, foreach_done
            // does not release the lock (it assumes the caller manages it).
            ll_recursive_unlock(dfe->dict, dfe->rw);
            dfe->locked = false;
            dictionary_foreach_done(dfe);
            return NULL;
        }
    }

    // the item we just did
    DICTIONARY_ITEM *item = dfe->item;

    // get the next item from the list
    DICTIONARY_ITEM *item_next = (item) ? dict_item_next(item, dfe->rcu_mode) : NULL;

    if(dfe->rcu_mode) {
        // RCU mode: skip deleted items without acquiring refcount
        while(item_next && !item_is_available_for_rcu_traversal(item_next))
            item_next = dict_item_next(item_next, true);

        // No need to release the previous item — we never acquired it
    }
    else {
        // Legacy mode: skip deleted items with CAS refcount acquisition
        while(item_next && !item_check_and_acquire(dfe->dict, item_next))
            item_next = item_next->next;

        if(likely(item)) {
            dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dfe->dict, item, dfe->rw);
        }
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

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT)) {
        ll_recursive_unlock(dfe->dict, dfe->rw);
        dfe->locked = false;
    }

    return dfe->value;
}

void dictionary_foreach_unlock(DICTFE *dfe) {
    if(dfe->locked) {
        ll_recursive_unlock(dfe->dict, dfe->rw);
        dfe->locked = false;
    }
}

void dictionary_foreach_done(DICTFE *dfe) {
    if(unlikely(!dfe || !dfe->dict)) return;

    // the item we just did
    DICTIONARY_ITEM *item = dfe->item;

    // release it, so that it can possibly be deleted
    if(likely(item)) {
        if(!dfe->rcu_mode) {
            dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dfe->dict, item, dfe->rw);
        }
        // In RCU mode: no release needed — we never acquired a refcount
    }

    if(likely(dfe->rw != DICTIONARY_LOCK_REENTRANT) && dfe->locked) {
        ll_recursive_unlock(dfe->dict, dfe->rw);
        dfe->locked = false;
    }

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

int dictionary_walkthrough_rw(DICTIONARY *dict, char rw, dict_walkthrough_callback_t walkthrough_callback, void *data) {
    if(unlikely(!dict || !walkthrough_callback)) return 0;

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_walkthrough_rw() on a destroyed dictionary");
        return 0;
    }

    bool rcu_mode = ll_lock_is_rcu_mode(dict, rw);

    ll_recursive_lock(dict, rw);

    if(unlikely(is_dictionary_destroyed(dict))) {
        ll_recursive_unlock(dict, rw);
        return 0;
    }

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    // written in such a way, that the callback can delete the active element

    int ret = 0;
    DICTIONARY_ITEM *item = dict_item_list_head(dict, rcu_mode), *item_next;
    while(item) {

        if(rcu_mode) {
            // RCU mode: skip deleted/unavailable items without refcount
            if(unlikely(!item_is_available_for_rcu_traversal(item))) {
                item = dict_item_next(item, true);
                continue;
            }
        }
        else {
            // Legacy mode: skip deleted items with CAS refcount acquisition
            if(unlikely(!item_check_and_acquire(dict, item))) {
                item = item->next;
                continue;
            }
        }

        if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
            ll_recursive_unlock(dict, rw);

        int r = walkthrough_callback(item, item->shared->value, data);

        if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
            ll_recursive_lock(dict, rw);

        // since we have a reference counter, this item cannot be deleted
        // until we release the reference counter, so the pointers are there
        item_next = dict_item_next(item, rcu_mode);

        if(!rcu_mode) {
            dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dict, item, rw);
        }

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

typedef int (*qsort_compar)(const void *item1, const void *item2);

static int dictionary_sort_compar(const void *item1, const void *item2) {
    return strcmp(item_get_name((*(DICTIONARY_ITEM **)item1)), item_get_name((*(DICTIONARY_ITEM **)item2)));
}

int dictionary_sorted_walkthrough_rw(DICTIONARY *dict, char rw, dict_walkthrough_callback_t walkthrough_callback, void *data, dict_item_comparator_t item_comparator) {
    if(unlikely(!dict || !walkthrough_callback)) return 0;

    if(unlikely(is_dictionary_destroyed(dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_sorted_walkthrough_rw() on a destroyed dictionary");
        return 0;
    }

    // Sorted walkthrough releases the lock before calling callbacks,
    // so items must be kept alive via refcounts. Force non-RCU lock mode
    // ('R') to ensure the rw_spinlock is used instead of RCU — RCU would
    // only protect items during the CS, but we need them after unlock.
    char lock_mode = (rw == DICTIONARY_LOCK_READ) ? 'R' : rw;

    ll_recursive_lock(dict, lock_mode);

    if(unlikely(is_dictionary_destroyed(dict))) {
        ll_recursive_unlock(dict, lock_mode);
        return 0;
    }

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);
    size_t entries = __atomic_load_n(&dict->entries, __ATOMIC_RELAXED);
    DICTIONARY_ITEM **array = mallocz(sizeof(DICTIONARY_ITEM *) * entries);

    size_t i;
    DICTIONARY_ITEM *item;
    for(item = dict->items.list, i = 0; item && i < entries; item = item->next) {
        if(likely(item_check_and_acquire(dict, item)))
            array[i++] = item;
    }
    ll_recursive_unlock(dict, lock_mode);

    if(unlikely(i != entries))
        entries = i;

    if(item_comparator)
        qsort(array, entries, sizeof(DICTIONARY_ITEM *), (qsort_compar) item_comparator);
    else
        qsort(array, entries, sizeof(DICTIONARY_ITEM *), dictionary_sort_compar);

    bool callit = true;
    int ret = 0, r;
    for(i = 0; i < entries ;i++) {
        item = array[i];

        if(callit)
            r = walkthrough_callback(item, item->shared->value, data);

        dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dict, item, lock_mode);
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
