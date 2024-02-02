// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_HASHTABLE_H
#define NETDATA_DICTIONARY_HASHTABLE_H

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// hash table operations

static inline size_t hashtable_init_unsafe(DICTIONARY *dict) {
    dict->index.JudyHSArray = NULL;
    return 0;
}

static inline size_t hashtable_destroy_unsafe(DICTIONARY *dict) {
    if(unlikely(!dict->index.JudyHSArray)) return 0;

    pointer_destroy_index(dict);

    JError_t J_Error;
    Word_t ret = JudyHSFreeArray(&dict->index.JudyHSArray, &J_Error);
    if(unlikely(ret == (Word_t) JERR)) {
        netdata_log_error("DICTIONARY: Cannot destroy JudyHS, JU_ERRNO_* == %u, ID == %d",
                          JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    netdata_log_debug(D_DICTIONARY, "Dictionary: hash table freed %lu bytes", ret);

    dict->index.JudyHSArray = NULL;
    return (size_t)ret;
}

static inline void **hashtable_insert_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
    JError_t J_Error;
    Pvoid_t *Rc = JudyHSIns(&dict->index.JudyHSArray, (void *)name, name_len, &J_Error);
    if (unlikely(Rc == PJERR)) {
        netdata_log_error("DICTIONARY: Cannot insert entry with name '%s' to JudyHS, JU_ERRNO_* == %u, ID == %d",
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
        netdata_log_error("DICTIONARY: Cannot delete entry with name '%s' from JudyHS, JU_ERRNO_* == %u, ID == %d",
                          name,
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
        pointer_check(dict, (DICTIONARY_ITEM *)*Rc);
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


#endif //NETDATA_DICTIONARY_HASHTABLE_H
