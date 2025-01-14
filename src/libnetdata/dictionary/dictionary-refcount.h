// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_REFCOUNT_H
#define NETDATA_DICTIONARY_REFCOUNT_H

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// reference counters

static inline size_t reference_counter_init(DICTIONARY *dict __maybe_unused) {
    // allocate memory required for reference counters
    // return number of bytes
    return 0;
}

static inline size_t reference_counter_free(DICTIONARY *dict __maybe_unused) {
    // free memory required for reference counters
    // return number of bytes
    return 0;
}

static inline void item_acquire(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    REFCOUNT refcount;

    if(unlikely(is_dictionary_single_threaded(dict)))
        refcount = ++item->refcount;

    else
        // increment the refcount
        refcount = __atomic_add_fetch(&item->refcount, 1, __ATOMIC_SEQ_CST);


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

static inline void item_release(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    // this function may be called without any lock on the dictionary
    // or even when someone else has 'write' lock on the dictionary

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
        refcount = __atomic_sub_fetch(&item->refcount, 1, __ATOMIC_RELEASE);
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

static inline int item_check_and_acquire_advanced(DICTIONARY *dict, DICTIONARY_ITEM *item, bool having_index_lock) {
    size_t spins = 0;
    REFCOUNT refcount, desired;

    int ret = RC_ITEM_OK;

    refcount = DICTIONARY_ITEM_REFCOUNT_GET(dict, item);

    do {
        spins++;

        if(refcount < 0) {
            // we can't use this item
            ret = RC_ITEM_IS_CURRENTLY_BEING_DELETED;
            break;
        }

        if(item_flag_check(item, ITEM_FLAG_DELETED)) {
            // we can't use this item
            ret = RC_ITEM_MARKED_FOR_DELETION;
            break;
        }

        desired = refcount + 1;

    } while(!__atomic_compare_exchange_n(&item->refcount, &refcount, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    // if ret == ITEM_OK, we acquired the item

    if(ret == RC_ITEM_OK) {
        if (unlikely(is_view_dictionary(dict) &&
                     item_shared_flag_check(item, ITEM_FLAG_DELETED) &&
                     !item_flag_check(item, ITEM_FLAG_DELETED))) {
            // but, we can't use this item

            if (having_index_lock) {
                // delete it from the hashtable
                if(hashtable_delete_unsafe(dict, item_get_name(item), item->key_len, item) == 0)
                    netdata_log_error("DICTIONARY: INTERNAL ERROR VIEW: tried to delete item with name '%s', "
                                      "name_len %u that is not in the index",
                                      item_get_name(item), (KEY_LEN_TYPE)(item->key_len));
                else
                    pointer_del(dict, item);

                // mark it in our dictionary as deleted too,
                // this is safe to be done here, because we have got
                // a reference counter on item
                dict_item_set_deleted(dict, item);

                // decrement the refcount we incremented above
                if (__atomic_sub_fetch(&item->refcount, 1, __ATOMIC_RELEASE) == 0) {
                    // this is a deleted item, and we are the last one
                    DICTIONARY_PENDING_DELETES_PLUS1(dict);
                }

                // do not touch the item below this point
            } else {
                // this is traversal / walkthrough
                // decrement the refcount we incremented above
                __atomic_sub_fetch(&item->refcount, 1, __ATOMIC_RELEASE);
            }

            return RC_ITEM_MARKED_FOR_DELETION;
        }

        if(desired == 1)
            DICTIONARY_REFERENCED_ITEMS_PLUS1(dict);
    }

    if(unlikely(spins > 1))
        DICTIONARY_STATS_CHECK_SPINS_PLUS(dict, spins - 1);

    return ret;
}

// if a dictionary item can be deleted, return true, otherwise return false
// we use the private reference counter
static inline int item_is_not_referenced_and_can_be_removed_advanced(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    // if we can set refcount to REFCOUNT_DELETING, we can delete this item

    size_t spins = 0;
    REFCOUNT refcount, desired = REFCOUNT_DELETED;

    int ret = RC_ITEM_OK;

    refcount = DICTIONARY_ITEM_REFCOUNT_GET(dict, item);

    do {
        spins++;

        if(refcount < 0) {
            // we can't use this item
            ret = RC_ITEM_IS_CURRENTLY_BEING_DELETED;
            break;
        }

        if(refcount > 0) {
            // we can't delete this
            ret = RC_ITEM_IS_REFERENCED;
            break;
        }

        if(item_flag_check(item, ITEM_FLAG_BEING_CREATED)) {
            // we can't use this item
            ret = RC_ITEM_IS_CURRENTLY_BEING_CREATED;
            break;
        }
    } while(!__atomic_compare_exchange_n(&item->refcount, &refcount, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

#ifdef NETDATA_INTERNAL_CHECKS
    if(ret == RC_ITEM_OK)
        item->deleter_pid = gettid_cached();
#endif

    if(unlikely(spins > 1))
        DICTIONARY_STATS_DELETE_SPINS_PLUS(dict, spins - 1);

    return ret;
}

// if a dictionary item can be freed, return true, otherwise return false
// we use the shared reference counter
static inline bool item_shared_release_and_check_if_it_can_be_freed(DICTIONARY *dict __maybe_unused, DICTIONARY_ITEM *item) {
    // if we can set refcount to REFCOUNT_DELETING, we can delete this item

    REFCOUNT links = __atomic_sub_fetch(&item->shared->links, 1, __ATOMIC_RELEASE);
    if(links == 0 && __atomic_compare_exchange_n(&item->shared->links, &links, REFCOUNT_DELETED, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED)) {

        // we can delete it
        return true;
    }

    // we can't delete it
    return false;
}

#endif //NETDATA_DICTIONARY_REFCOUNT_H
