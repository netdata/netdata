// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_HASHTABLE_H
#define NETDATA_DICTIONARY_HASHTABLE_H

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// hashtable operations with simple hashtable

//static inline bool compare_keys(void *key1, void *key2) {
//    const char *k1 = key1;
//    const char *k2 = key2;
//    return strcmp(k1, k2) == 0;
//}
//
//static inline void *item_to_key(DICTIONARY_ITEM *item) {
//    return (void *)item_get_name(item);
//}
//
//#define SIMPLE_HASHTABLE_VALUE_TYPE DICTIONARY_ITEM
//#define SIMPLE_HASHTABLE_NAME _DICTIONARY
//#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION item_to_key
//#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION compare_keys
//#include "..//simple_hashtable.h"

//static inline size_t hashtable_init_hashtable(DICTIONARY *dict) {
//    SIMPLE_HASHTABLE_DICTIONARY *ht = callocz(1, sizeof(*ht));
//    simple_hashtable_init_DICTIONARY(ht, 4);
//    dict->index.JudyHSArray = ht;
//    return 0;
//}
//
//static inline size_t hashtable_destroy_hashtable(DICTIONARY *dict) {
//    SIMPLE_HASHTABLE_DICTIONARY *ht = dict->index.JudyHSArray;
//    if(unlikely(!ht)) return 0;
//
//    size_t mem = sizeof(*ht) + ht->size * sizeof(SIMPLE_HASHTABLE_SLOT_DICTIONARY);
//    simple_hashtable_destroy_DICTIONARY(ht);
//    freez(ht);
//    dict->index.JudyHSArray = NULL;
//
//    return mem;
//}
//
//static inline void *hashtable_insert_hashtable(DICTIONARY *dict, const char *name, size_t name_len) {
//    SIMPLE_HASHTABLE_DICTIONARY *ht = dict->index.JudyHSArray;
//
//    char key[name_len+1];
//    memcpy(key, name, name_len);
//    key[name_len] = '\0';
//
//    XXH64_hash_t hash = XXH3_64bits(name, name_len);
//    SIMPLE_HASHTABLE_SLOT_DICTIONARY *sl = simple_hashtable_get_slot_DICTIONARY(ht, hash, key, true);
//    sl->hash = hash; // we will need it in insert later - it is ok to overwrite - it is the same already
//    return sl;
//}
//
//static inline DICTIONARY_ITEM *hashtable_insert_handle_to_item_hashtable(DICTIONARY *dict, void *handle) {
//    (void)dict;
//    SIMPLE_HASHTABLE_SLOT_DICTIONARY *sl = handle;
//    DICTIONARY_ITEM *item = SIMPLE_HASHTABLE_SLOT_DATA(sl);
//    return item;
//}
//
//static inline void hashtable_set_item_hashtable(DICTIONARY *dict, void *handle, DICTIONARY_ITEM *item) {
//    SIMPLE_HASHTABLE_DICTIONARY *ht = dict->index.JudyHSArray;
//    SIMPLE_HASHTABLE_SLOT_DICTIONARY *sl = handle;
//    simple_hashtable_set_slot_DICTIONARY(ht, sl, sl->hash, item);
//}
//
//static inline int hashtable_delete_hashtable(DICTIONARY *dict, const char *name, size_t name_len, DICTIONARY_ITEM *item_to_delete) {
//    (void)item_to_delete;
//    SIMPLE_HASHTABLE_DICTIONARY *ht = dict->index.JudyHSArray;
//
//    char key[name_len+1];
//    memcpy(key, name, name_len);
//    key[name_len] = '\0';
//
//    XXH64_hash_t hash = XXH3_64bits(name, name_len);
//    SIMPLE_HASHTABLE_SLOT_DICTIONARY *sl = simple_hashtable_get_slot_DICTIONARY(ht, hash, key, false);
//    DICTIONARY_ITEM *item = SIMPLE_HASHTABLE_SLOT_DATA(sl);
//    if(!item) return 0; // return not-found
//
//    simple_hashtable_del_slot_DICTIONARY(ht, sl);
//    return 1; // return deleted
//}
//
//static inline DICTIONARY_ITEM *hashtable_get_hashtable(DICTIONARY *dict, const char *name, size_t name_len) {
//    SIMPLE_HASHTABLE_DICTIONARY *ht = dict->index.JudyHSArray;
//    if(unlikely(!ht)) return NULL;
//
//    char key[name_len+1];
//    memcpy(key, name, name_len);
//    key[name_len] = '\0';
//
//    XXH64_hash_t hash = XXH3_64bits(name, name_len);
//    SIMPLE_HASHTABLE_SLOT_DICTIONARY *sl = simple_hashtable_get_slot_DICTIONARY(ht, hash, key, true);
//    return SIMPLE_HASHTABLE_SLOT_DATA(sl);
//}

// ----------------------------------------------------------------------------
// hashtable operations with Judy

static inline size_t hashtable_init_judy(DICTIONARY *dict) {
    dict->index.JudyHSArray = NULL;
    return 0;
}

static inline size_t hashtable_destroy_judy(DICTIONARY *dict) {
    if(unlikely(!dict->index.JudyHSArray)) return 0;

    pointer_destroy_index(dict);

    JudyAllocThreadPulseReset();

    JError_t J_Error;
    Word_t ret = JudyHSFreeArray(&dict->index.JudyHSArray, &J_Error);

    __atomic_add_fetch(&dict->stats->memory.index, JudyAllocThreadPulseGetAndReset(), __ATOMIC_RELAXED);

    if(unlikely(ret == (Word_t) JERR)) {
        netdata_log_error("DICTIONARY: Cannot destroy JudyHS, JU_ERRNO_* == %u, ID == %d",
                          JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    netdata_log_debug(D_DICTIONARY, "Dictionary: hash table freed %lu bytes", ret);

    dict->index.JudyHSArray = NULL;
    return (size_t)ret;
}

static inline void *hashtable_insert_judy(DICTIONARY *dict, const char *name, size_t name_len) {
    JudyAllocThreadPulseReset();

    JError_t J_Error;
    Pvoid_t *Rc = JudyHSIns(&dict->index.JudyHSArray, (void *)name, name_len, &J_Error);

    __atomic_add_fetch(&dict->stats->memory.index, JudyAllocThreadPulseGetAndReset(), __ATOMIC_RELAXED);

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
    return (void *)Rc;
}

static inline DICTIONARY_ITEM *hashtable_insert_handle_to_item_judy(DICTIONARY *dict, void *handle) {
    (void)dict;
    DICTIONARY_ITEM **item_pptr = handle;
    return *item_pptr;
}

static inline void hashtable_set_item_judy(DICTIONARY *dict, void *handle, DICTIONARY_ITEM *item) {
    (void)dict;
    DICTIONARY_ITEM **item_pptr = handle;
    *item_pptr = item;
}

static inline int hashtable_delete_judy(DICTIONARY *dict, const char *name, size_t name_len, DICTIONARY_ITEM *item) {
    (void)item;
    if(unlikely(!dict->index.JudyHSArray)) return 0;

    JudyAllocThreadPulseReset();

    JError_t J_Error;
    int ret = JudyHSDel(&dict->index.JudyHSArray, (void *)name, name_len, &J_Error);

    __atomic_add_fetch(&dict->stats->memory.index, JudyAllocThreadPulseGetAndReset(), __ATOMIC_RELAXED);

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

static inline DICTIONARY_ITEM *hashtable_get_judy(DICTIONARY *dict, const char *name, size_t name_len) {
    if(unlikely(!dict->index.JudyHSArray)) return NULL;

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

// --------------------------------------------------------------------------------------------------------------------
// select the right hashtable

static inline size_t hashtable_init_unsafe(DICTIONARY *dict) {
    return hashtable_init_judy(dict);
//    if(dict->options & DICT_OPTION_INDEX_JUDY)
//        return hashtable_init_judy(dict);
//    else
//        return hashtable_init_hashtable(dict);
}

static inline size_t hashtable_destroy_unsafe(DICTIONARY *dict) {
    pointer_destroy_index(dict);

//    if(dict->options & DICT_OPTION_INDEX_JUDY)
    return hashtable_destroy_judy(dict);
//    else
//        return hashtable_destroy_hashtable(dict);
}

static inline void *hashtable_insert_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
    return hashtable_insert_judy(dict, name, name_len);
//    if(dict->options & DICT_OPTION_INDEX_JUDY)
//        return hashtable_insert_judy(dict, name, name_len);
//    else
//        return hashtable_insert_hashtable(dict, name, name_len);
}

static inline DICTIONARY_ITEM *hashtable_insert_handle_to_item_unsafe(DICTIONARY *dict, void *handle) {
    return hashtable_insert_handle_to_item_judy(dict, handle);
//    if(dict->options & DICT_OPTION_INDEX_JUDY)
//        return hashtable_insert_handle_to_item_judy(dict, handle);
//    else
//        return hashtable_insert_handle_to_item_hashtable(dict, handle);
}

static inline int hashtable_delete_unsafe(DICTIONARY *dict, const char *name, size_t name_len, DICTIONARY_ITEM *item) {
    return hashtable_delete_judy(dict, name, name_len, item);
//    if(dict->options & DICT_OPTION_INDEX_JUDY)
//        return hashtable_delete_judy(dict, name, name_len, item);
//    else
//        return hashtable_delete_hashtable(dict, name, name_len, item);
}

static inline DICTIONARY_ITEM *hashtable_get_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
    DICTIONARY_STATS_SEARCHES_PLUS1(dict);

    DICTIONARY_ITEM *item;

    item = hashtable_get_judy(dict, name, name_len);
//    if(dict->options & DICT_OPTION_INDEX_JUDY)
//        item = hashtable_get_judy(dict, name, name_len);
//    else
//        item = hashtable_get_hashtable(dict, name, name_len);

    if(item)
        pointer_check(dict, item);

    return item;
}

static inline void hashtable_set_item_unsafe(DICTIONARY *dict, void *handle, DICTIONARY_ITEM *item) {
    hashtable_set_item_judy(dict, handle, item);
//    if(dict->options & DICT_OPTION_INDEX_JUDY)
//        hashtable_set_item_judy(dict, handle, item);
//    else
//        hashtable_set_item_hashtable(dict, handle, item);
}

#endif //NETDATA_DICTIONARY_HASHTABLE_H
