// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// dictionary statistics

static inline void NETDATA_DICTIONARY_STATS_INSERTS_PLUS1(DICTIONARY *dict) {
    if(likely(dict->stats))
        dict->stats->inserts++;
}
static inline void NETDATA_DICTIONARY_STATS_DELETES_PLUS1(DICTIONARY *dict) {
    if(likely(dict->stats))
        dict->stats->deletes++;
}
static inline void NETDATA_DICTIONARY_STATS_SEARCHES_PLUS1(DICTIONARY *dict) {
    if(likely(dict->stats))
        dict->stats->searches++;
}
static inline void NETDATA_DICTIONARY_STATS_ENTRIES_PLUS1(DICTIONARY *dict) {
    if(likely(dict->stats))
        dict->stats->entries++;
}
static inline void NETDATA_DICTIONARY_STATS_ENTRIES_MINUS1(DICTIONARY *dict) {
    if(likely(dict->stats))
        dict->stats->entries--;
}


// ----------------------------------------------------------------------------
// dictionary locks

static inline void dictionary_read_lock(DICTIONARY *dict) {
    if(likely(dict->rwlock)) {
        // debug(D_DICTIONARY, "Dictionary READ lock");
        netdata_rwlock_rdlock(dict->rwlock);
    }
}

static inline void dictionary_write_lock(DICTIONARY *dict) {
    if(likely(dict->rwlock)) {
        // debug(D_DICTIONARY, "Dictionary WRITE lock");
        netdata_rwlock_wrlock(dict->rwlock);
    }
}

static inline void dictionary_unlock(DICTIONARY *dict) {
    if(likely(dict->rwlock)) {
        // debug(D_DICTIONARY, "Dictionary UNLOCK lock");
        netdata_rwlock_unlock(dict->rwlock);
    }
}


// ----------------------------------------------------------------------------
// avl index

static int name_value_compare(void* a, void* b) {
    if(((NAME_VALUE *)a)->hash < ((NAME_VALUE *)b)->hash) return -1;
    else if(((NAME_VALUE *)a)->hash > ((NAME_VALUE *)b)->hash) return 1;
    else return strcmp(((NAME_VALUE *)a)->name, ((NAME_VALUE *)b)->name);
}

static inline NAME_VALUE *dictionary_name_value_index_find_nolock(DICTIONARY *dict, const char *name, uint32_t hash) {
    NAME_VALUE tmp;
    tmp.hash = (hash)?hash:simple_hash(name);
    tmp.name = (char *)name;

    NETDATA_DICTIONARY_STATS_SEARCHES_PLUS1(dict);
    return (NAME_VALUE *)avl_search(&(dict->values_index), (avl *) &tmp);
}

// ----------------------------------------------------------------------------
// internal methods

static NAME_VALUE *dictionary_name_value_create_nolock(DICTIONARY *dict, const char *name, void *value, size_t value_len, uint32_t hash) {
    debug(D_DICTIONARY, "Creating name value entry for name '%s'.", name);

    NAME_VALUE *nv = callocz(1, sizeof(NAME_VALUE));

    if(dict->flags & DICTIONARY_FLAG_NAME_LINK_DONT_CLONE)
        nv->name = (char *)name;
    else {
        nv->name = strdupz(name);
    }

    nv->hash = (hash)?hash:simple_hash(nv->name);

    if(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE)
        nv->value = value;
    else {
        nv->value = mallocz(value_len);
        memcpy(nv->value, value, value_len);
    }

    // index it
    NETDATA_DICTIONARY_STATS_INSERTS_PLUS1(dict);
    if(unlikely(avl_insert(&((dict)->values_index), (avl *)(nv)) != (avl *)nv))
        error("dictionary: INTERNAL ERROR: duplicate insertion to dictionary.");

    NETDATA_DICTIONARY_STATS_ENTRIES_PLUS1(dict);

    return nv;
}

static void dictionary_name_value_destroy_nolock(DICTIONARY *dict, NAME_VALUE *nv) {
    debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", nv->name);

    NETDATA_DICTIONARY_STATS_DELETES_PLUS1(dict);
    if(unlikely(avl_remove(&(dict->values_index), (avl *)(nv)) != (avl *)nv))
        error("dictionary: INTERNAL ERROR: dictionary invalid removal of node.");

    NETDATA_DICTIONARY_STATS_ENTRIES_MINUS1(dict);

    if(!(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE)) {
        debug(D_REGISTRY, "Dictionary freeing value of '%s'", nv->name);
        freez(nv->value);
    }

    if(!(dict->flags & DICTIONARY_FLAG_NAME_LINK_DONT_CLONE)) {
        debug(D_REGISTRY, "Dictionary freeing name '%s'", nv->name);
        freez(nv->name);
    }

    freez(nv);
}

// ----------------------------------------------------------------------------
// API - basic methods

DICTIONARY *dictionary_create(uint8_t flags) {
    debug(D_DICTIONARY, "Creating dictionary.");

    DICTIONARY *dict = callocz(1, sizeof(DICTIONARY));

    if(flags & DICTIONARY_FLAG_WITH_STATISTICS)
        dict->stats = callocz(1, sizeof(struct dictionary_stats));

    if(!(flags & DICTIONARY_FLAG_SINGLE_THREADED)) {
        dict->rwlock = callocz(1, sizeof(netdata_rwlock_t));
        netdata_rwlock_init(dict->rwlock);
    }

    avl_init(&dict->values_index, name_value_compare);
    dict->flags = flags;

    return dict;
}

void dictionary_destroy(DICTIONARY *dict) {
    debug(D_DICTIONARY, "Destroying dictionary.");

    dictionary_write_lock(dict);

    while(dict->values_index.root)
        dictionary_name_value_destroy_nolock(dict, (NAME_VALUE *)dict->values_index.root);

    dictionary_unlock(dict);

    if(dict->stats)
        freez(dict->stats);

    if(dict->rwlock) {
        netdata_rwlock_destroy(dict->rwlock);
        freez(dict->rwlock);
    }

    freez(dict);
}

// ----------------------------------------------------------------------------

void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len) {
    debug(D_DICTIONARY, "SET dictionary entry with name '%s'.", name);

    uint32_t hash = simple_hash(name);

    dictionary_write_lock(dict);

    NAME_VALUE *nv = dictionary_name_value_index_find_nolock(dict, name, hash);
    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Dictionary entry with name '%s' not found. Creating a new one.", name);

        nv = dictionary_name_value_create_nolock(dict, name, value, value_len, hash);
        if(unlikely(!nv))
            fatal("Cannot create name_value.");
    }
    else {
        debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", name);

        if(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE) {
            debug(D_REGISTRY, "Dictionary: linking value to '%s'", name);
            nv->value = value;
        }
        else {
            debug(D_REGISTRY, "Dictionary: cloning value to '%s'", name);

            // copy the new value without breaking
            // any other thread accessing the same entry
            void *new = mallocz(value_len),
                    *old = nv->value;

            memcpy(new, value, value_len);
            nv->value = new;

            debug(D_REGISTRY, "Dictionary: freeing old value of '%s'", name);
            freez(old);
        }
    }

    dictionary_unlock(dict);

    return nv->value;
}

void *dictionary_get(DICTIONARY *dict, const char *name) {
    debug(D_DICTIONARY, "GET dictionary entry with name '%s'.", name);

    dictionary_read_lock(dict);
    NAME_VALUE *nv = dictionary_name_value_index_find_nolock(dict, name, 0);
    dictionary_unlock(dict);

    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        return NULL;
    }

    debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
    return nv->value;
}

int dictionary_del(DICTIONARY *dict, const char *name) {
    int ret;

    debug(D_DICTIONARY, "DEL dictionary entry with name '%s'.", name);

    dictionary_write_lock(dict);

    NAME_VALUE *nv = dictionary_name_value_index_find_nolock(dict, name, 0);
    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        ret = -1;
    }
    else {
        debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
        dictionary_name_value_destroy_nolock(dict, nv);
        ret = 0;
    }

    dictionary_unlock(dict);

    return ret;
}


// ----------------------------------------------------------------------------
// API - walk through the dictionary
// the dictionary is locked for reading while this happens
// do not user other dictionary calls while walking the dictionary - deadlock!

static int dictionary_walker(avl *a, int (*callback)(void *entry, void *data), void *data) {
    int total = 0, ret = 0;

    if(a->avl_link[0]) {
        ret = dictionary_walker(a->avl_link[0], callback, data);
        if(ret < 0) return ret;
        total += ret;
    }

    ret = callback(((NAME_VALUE *)a)->value, data);
    if(ret < 0) return ret;
    total += ret;

    if(a->avl_link[1]) {
        ret = dictionary_walker(a->avl_link[1], callback, data);
        if (ret < 0) return ret;
        total += ret;
    }

    return total;
}

int dictionary_get_all(DICTIONARY *dict, int (*callback)(void *entry, void *data), void *data) {
    int ret = 0;

    dictionary_read_lock(dict);

    if(likely(dict->values_index.root))
        ret = dictionary_walker(dict->values_index.root, callback, data);

    dictionary_unlock(dict);

    return ret;
}
