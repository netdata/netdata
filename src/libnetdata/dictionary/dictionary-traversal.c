// SPDX-License-Identifier: GPL-3.0-or-later

#include "dictionary-internals.h"


// ----------------------------------------------------------------------------
// traversal with loop

void *dictionary_foreach_start_rw(DICTFE *dfe, DICTIONARY *dict, char rw) {
    if(unlikely(!dfe || !dict)) return NULL;

    DICTIONARY_STATS_TRAVERSALS_PLUS1(dict);

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
    dfe->locked = true;
    ll_recursive_lock(dict, dfe->rw);

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
        dfe->item = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
        return NULL;
    }

    if(unlikely(dfe->rw == DICTIONARY_LOCK_REENTRANT) || !dfe->locked) {
        ll_recursive_lock(dfe->dict, dfe->rw);
        dfe->locked = true;
    }

    // the item we just did
    DICTIONARY_ITEM *item = dfe->item;

    // get the next item from the list
    DICTIONARY_ITEM *item_next = (item) ? item->next : NULL;

    // skip all the deleted items until one that can be acquired is found
    while(item_next && !item_check_and_acquire(dfe->dict, item_next))
        item_next = item_next->next;

    if(likely(item)) {
        dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dfe->dict, item, dfe->rw);
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

    if(unlikely(is_dictionary_destroyed(dfe->dict))) {
        internal_error(true, "DICTIONARY: attempted to dictionary_foreach_next() on a destroyed dictionary");
        return;
    }

    // the item we just did
    DICTIONARY_ITEM *item = dfe->item;

    // release it, so that it can possibly be deleted
    if(likely(item)) {
        dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dfe->dict, item, dfe->rw);
        // item_release(dfe->dict, item);
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

        int r = walkthrough_callback(item, item->shared->value, data);

        if(unlikely(rw == DICTIONARY_LOCK_REENTRANT))
            ll_recursive_lock(dict, rw);

        // since we have a reference counter, this item cannot be deleted
        // until we release the reference counter, so the pointers are there
        item_next = item->next;

        dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dict, item, rw);
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

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    ll_recursive_lock(dict, rw);
    size_t entries = __atomic_load_n(&dict->entries, __ATOMIC_RELAXED);
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

        dict_item_release_and_check_if_it_is_deleted_and_can_be_removed_under_this_lock_mode(dict, item, rw);
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

