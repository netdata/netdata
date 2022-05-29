// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "Judy.h"

/*
 * This version uses JudyHS arrays to index the dictionary
 *
 * The following output is from the unit test, at the end of this file:
 *
 * This is the JudyHS version:
 *
 * 1000000 x dictionary_set() (dictionary size 0 entries, 0 KB)...
 * 1000000 x dictionary_get(existing) (dictionary size 1000000 entries, 74001 KB)...
 * 1000000 x dictionary_get(non-existing) (dictionary size 1000000 entries, 74001 KB)...
 * Walking through the dictionary (dictionary size 1000000 entries, 74001 KB)...
 * 1000000 x dictionary_del(existing) (dictionary size 1000000 entries, 74001 KB)...
 * 1000000 x dictionary_set() (dictionary size 0 entries, 0 KB)...
 * Destroying dictionary (dictionary size 1000000 entries, 74001 KB)...
 *
 * TIMINGS:
 * adding 316027 usec, positive search 156740 usec, negative search 84524, walk through 15036 usec, deleting 361444, destroy 107394 usec
 *
 * This is from the JudySL version:
 *
 * Creating dictionary of 1000000 entries...
 * Checking index of 1000000 entries...
 * Walking 1000000 entries and checking name-value pairs...
 * Created and checked 1000000 entries, found 0 errors - used 58376 KB of memory
 * Destroying dictionary of 1000000 entries...
 * Deleted 1000000 entries
 * create 338975 usec, check 156080 usec, walk 80764 usec, destroy 444569 usec
 *
 * This is the AVL version:
 *
 * Creating dictionary of 1000000 entries...
 * Checking index of 1000000 entries...
 * Walking 1000000 entries and checking name-value pairs...
 * Created and checked 1000000 entries, found 0 errors - used 89626 KB of memory
 * Destroying dictionary of 1000000 entries...
 * create 413892 usec, check 220006 usec, walk 34247 usec, destroy 98062 usec
 *
 * So, the JudySL is a lot slower to WALK and DESTROY (DESTROY does a WALK)
 * It is slower, because for every item, JudySL copies the KEY/NAME to a
 * caller supplied buffer (Index). So, by just walking over 1 million items,
 * JudySL does 1 million strcpy() !!!
 *
 * It also seems that somehow JudySLDel() is unbelievably slow too!
 *
 */


/*
 * Every item in the dictionary has the following structure.
 */
typedef struct name_value {
    struct name_value *next;    // a double linked list to allow fast insertions and deletions
    struct name_value *prev;

    char *name;                 // the name of the dictionary item
    void *value;                // the value of the dictionary item
} NAME_VALUE;

/*
 * When DICTIONARY_FLAG_WITH_STATISTICS is set, we need to keep track of all the memory
 * we allocate and free. So, we need to keep track of the sizes of all names and values.
 * We do this by overloading NAME_VALUE with the following additional fields.
 */
typedef struct name_value_with_stats {
    NAME_VALUE name_value_data_here;    // never used - just to put the lengths at the right position

    size_t name_len;                    // the size of the name, including the terminating zero
    size_t value_len;                   // the size of the value (assumed binary)
} NAME_VALUE_WITH_STATS;

struct dictionary_stats {
    size_t inserts;
    size_t deletes;
    size_t searches;
    size_t resets;
    size_t entries;
    size_t memory;
};

typedef struct dictionary {
    uint8_t flags;                      // the flags of the dictionary

    NAME_VALUE *first_item;             // the double linked list base pointers
    NAME_VALUE *last_item;

    Pvoid_t JudyHSArray;                // the hash table

    netdata_rwlock_t *rwlock;
    struct dictionary_stats *stats;
} DICT;

// ----------------------------------------------------------------------------
// dictionary statistics maintenance

size_t dictionary_allocated_memory(DICTIONARY *ptr) {
    DICT *dict = (DICT *)ptr;

    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS))
        return dict->stats->memory;

    return 0;
}

size_t dictionary_entries(DICTIONARY *ptr) {
    DICT *dict = (DICT *)ptr;

    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS))
        return dict->stats->entries;

    return 0;
}

static inline void DICTIONARY_STATS_SEARCHES_PLUS1(DICT *dict) {
    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS))
        dict->stats->searches++;
}
static inline void DICTIONARY_STATS_ENTRIES_PLUS1(DICT *dict, size_t size) {
    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS)) {
        dict->stats->inserts++;
        dict->stats->entries++;
        dict->stats->memory += size;
    }
}
static inline void DICTIONARY_STATS_ENTRIES_MINUS1(DICT *dict, size_t size) {
    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS)) {
        dict->stats->deletes++;
        dict->stats->entries--;
        dict->stats->memory -= size;
    }
}
static inline void DICTIONARY_STATS_VALUE_RESETS_PLUS1(DICT *dict, size_t oldsize, size_t newsize) {
    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS)) {
        dict->stats->resets++;
        dict->stats->memory += newsize;
        dict->stats->memory -= oldsize;
    }
}


// ----------------------------------------------------------------------------
// dictionary locks

static inline void dictionary_read_lock(DICT *dict) {
    if(likely(dict->rwlock)) {
        // debug(D_DICTIONARY, "Dictionary READ lock");
        netdata_rwlock_rdlock(dict->rwlock);
    }
}

static inline void dictionary_write_lock(DICT *dict) {
    if(likely(dict->rwlock)) {
        // debug(D_DICTIONARY, "Dictionary WRITE lock");
        netdata_rwlock_wrlock(dict->rwlock);
    }
}

static inline void dictionary_unlock(DICT *dict) {
    if(likely(dict->rwlock)) {
        // debug(D_DICTIONARY, "Dictionary UNLOCK lock");
        netdata_rwlock_unlock(dict->rwlock);
    }
}


// ----------------------------------------------------------------------------
// hash table

static void hashtable_init_unsafe(DICT *dict) {
    dict->JudyHSArray = NULL;
}

static void hashtable_destroy_unsafe(DICT *dict) {
    if(unlikely(!dict->JudyHSArray)) return;

    JError_t J_Error;
    Word_t ret = JudyHSFreeArray(&dict->JudyHSArray, &J_Error);
    if(unlikely(ret == (Word_t) JERR)) {
        error("DICTIONARY: Cannot destroy JudyHS, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    debug(D_DICTIONARY, "Dictionary: hash table freed %lu bytes", ret);

    dict->JudyHSArray = NULL;
}

static inline NAME_VALUE **hashtable_insert_unsafe(DICT *dict, const char *name, size_t name_len) {
    Pvoid_t *Rc;
    JError_t J_Error;
    Rc = JudyHSIns(&dict->JudyHSArray, (void *)name, name_len, &J_Error);
    if (unlikely(Rc == PJERR)) {
        fatal("DICTIONARY: Cannot insert entry with name '%s' to JudyHS, JU_ERRNO_* == %u, ID == %d",
              name, JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    return (NAME_VALUE **)Rc;
}

static inline int hashtable_delete_unsafe(DICT *dict, const char *name, size_t name_len) {
    if(unlikely(!dict->JudyHSArray)) return 0;

    JError_t J_Error;
    int ret = JudyHSDel(&dict->JudyHSArray, (void *)name, name_len, &J_Error);
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

static inline NAME_VALUE *hashtable_get_unsafe(DICT *dict, const char *name, size_t name_len) {
    if(unlikely(!dict->JudyHSArray)) return NULL;

    DICTIONARY_STATS_SEARCHES_PLUS1(dict);

    Pvoid_t *Rc;
    Rc = JudyHSGet(dict->JudyHSArray, (void *)name, name_len);
    if(likely(Rc)) {
        // found in the hash table
        return (NAME_VALUE *)*Rc;
    }
    else {
        // not found in the hash table
        return NULL;
    }
}

// ----------------------------------------------------------------------------
// linked list management

static inline void linkedlist_namevalue_link_last(DICT *dict, NAME_VALUE *nv) {
    if(unlikely(!dict->first_item)) {
        // we are the only ones here
        nv->next = NULL;
        nv->prev = NULL;
        dict->first_item = dict->last_item = nv;
    }
    else {
        // add it at the end
        nv->prev = dict->last_item;
        nv->next = NULL;

        if(likely(nv->prev)) nv->prev->next = nv;
        dict->last_item = nv;
    }
}

static inline void linkedlist_namevalue_unlink(DICT *dict, NAME_VALUE *nv) {
    if(nv->next) nv->next->prev = nv->prev;
    if(nv->prev) nv->prev->next = nv->next;
    if(dict->first_item == nv) dict->first_item = nv->next;
    if(dict->last_item == nv) dict->last_item = nv->prev;
}

static inline int linkedlist_namevalue_walkthrough_unsafe(DICT *dict, int (*callback)(const char *name, void *value, void *data), void *data) {
    int ret = 0;
    NAME_VALUE *nv;
    for(nv = dict->first_item; nv ; nv = nv->next) {
        int r = callback(nv->name, nv->value, data);
        if(unlikely(r < 0)) return r;

        ret += r;
    }
    return ret;
}

// ----------------------------------------------------------------------------
// NAME_VALUE methods

static inline size_t namevalue_alloc_size(DICT *dict) {
    return (dict->flags & DICTIONARY_FLAG_WITH_STATISTICS) ? sizeof(NAME_VALUE_WITH_STATS) : sizeof(NAME_VALUE);
}

static inline size_t namevalue_get_namelen(DICT *dict, NAME_VALUE *nv) {
    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS)) {
        NAME_VALUE_WITH_STATS *nvs = (NAME_VALUE_WITH_STATS *)nv;
        return nvs->name_len;
    }
    return 0;
}
static inline size_t namevalue_get_valuelen(DICT *dict, NAME_VALUE *nv) {
    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS)) {
        NAME_VALUE_WITH_STATS *nvs = (NAME_VALUE_WITH_STATS *)nv;
        return nvs->value_len;
    }
    return 0;
}
static inline void namevalue_set_valuelen(DICT *dict, NAME_VALUE *nv, size_t value_len) {
    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS)) {
        NAME_VALUE_WITH_STATS *nvs = (NAME_VALUE_WITH_STATS *)nv;
        nvs->value_len = value_len;
    }
}
static inline void namevalue_set_namevaluelen(DICT *dict, NAME_VALUE *nv, size_t name_len, size_t value_len) {
    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS)) {
        NAME_VALUE_WITH_STATS *nvs = (NAME_VALUE_WITH_STATS *)nv;
        nvs->name_len = name_len;
        nvs->value_len = value_len;
    }
}

static NAME_VALUE *namevalue_create_unsafe(DICT *dict, const char *name, size_t name_len, void *value, size_t value_len) {
    debug(D_DICTIONARY, "Creating name value entry for name '%s'.", name);

    size_t size = namevalue_alloc_size(dict);
    NAME_VALUE *nv = mallocz(size);
    size_t allocated = size;

    namevalue_set_namevaluelen(dict, nv, name_len, value_len);

    if(dict->flags & DICTIONARY_FLAG_NAME_LINK_DONT_CLONE)
        nv->name = (char *)name;
    else {
        nv->name = mallocz(name_len);
        memcpy(nv->name, name, name_len);
        allocated += name_len;
    }

    if(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE)
        nv->value = value;
    else {
        nv->value = mallocz(value_len);
        memcpy(nv->value, value, value_len);
        allocated += value_len;
    }

    DICTIONARY_STATS_ENTRIES_PLUS1(dict, allocated);

    return nv;
}

static void namevalue_reset_unsafe(DICT *dict, NAME_VALUE *nv, void *value, size_t value_len) {
    debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", nv->name);

    if(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE) {
        debug(D_DICTIONARY, "Dictionary: linking value to '%s'", nv->name);
        nv->value = value;
        namevalue_set_valuelen(dict, nv, value_len);
    }
    else {
        debug(D_DICTIONARY, "Dictionary: cloning value to '%s'", nv->name);
        DICTIONARY_STATS_VALUE_RESETS_PLUS1(dict, namevalue_get_valuelen(dict, nv), value_len);

        // copy the new value without breaking
        // any other thread accessing the same entry
        void *old = nv->value;

        void *new = mallocz(value_len);
        memcpy(new, value, value_len);
        nv->value = new;
        namevalue_set_valuelen(dict, nv, value_len);

        debug(D_DICTIONARY, "Dictionary: freeing old value of '%s'", nv->name);
        freez(old);
    }
}

static void namevalue_destroy_unsafe(DICT *dict, NAME_VALUE *nv) {
    debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", nv->name);

    size_t freed = 0;

    if(!(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE)) {
        debug(D_DICTIONARY, "Dictionary freeing value of '%s'", nv->name);
        freez(nv->value);
        freed += namevalue_get_valuelen(dict, nv);
    }

    if(!(dict->flags & DICTIONARY_FLAG_NAME_LINK_DONT_CLONE)) {
        debug(D_DICTIONARY, "Dictionary freeing name '%s'", nv->name);
        freez(nv->name);
        freed += namevalue_get_namelen(dict, nv);
    }

    freez(nv);
    freed += namevalue_alloc_size(dict);

    DICTIONARY_STATS_ENTRIES_MINUS1(dict, freed);
}

// ----------------------------------------------------------------------------
// API - dictionary management

DICTIONARY *dictionary_create(uint8_t flags) {
    debug(D_DICTIONARY, "Creating dictionary.");

    DICT *dict = mallocz(sizeof(DICT));
    size_t allocated = sizeof(DICT);

    dict->flags = flags;
    dict->first_item = dict->last_item = NULL;

    if(!(flags & DICTIONARY_FLAG_SINGLE_THREADED)) {
        dict->rwlock = mallocz(sizeof(netdata_rwlock_t));
        allocated += sizeof(netdata_rwlock_t);
        netdata_rwlock_init(dict->rwlock);
    }
    else
        dict->rwlock = NULL;

    if(flags & DICTIONARY_FLAG_WITH_STATISTICS) {
        dict->stats = callocz(1, sizeof(struct dictionary_stats));
        allocated += sizeof(struct dictionary_stats);
        dict->stats->memory = allocated;
    }
    else
        dict->stats = NULL;

    hashtable_init_unsafe(dict);
    return (DICTIONARY *)dict;
}

void dictionary_destroy(DICTIONARY *ptr) {
    DICT *dict = (DICT *)ptr;

    debug(D_DICTIONARY, "Destroying dictionary.");

    dictionary_write_lock(dict);

    NAME_VALUE *nv = dict->first_item;
    while (nv) {
        // cache nv->next
        // because we are going to free nv
        NAME_VALUE *nvnext = nv->next;
        namevalue_destroy_unsafe(dict, nv);
        nv = nvnext;
        // to speed up destruction, we don't
        // unlink nv from the linked-list here
    }

    dict->first_item = NULL;
    dict->last_item = NULL;

    // destroy the dictionary
    hashtable_destroy_unsafe(dict);

    dictionary_unlock(dict);

    if(dict->rwlock) {
        netdata_rwlock_destroy(dict->rwlock);
        freez(dict->rwlock);
    }

    if(unlikely(dict->flags & DICTIONARY_FLAG_WITH_STATISTICS)) {
        freez(dict->stats);
        dict->stats = NULL;
    }

    freez(dict);
}

// ----------------------------------------------------------------------------
// API - items management

void *dictionary_set_with_name_ptr(DICTIONARY *ptr, const char *name, void *value, size_t value_len, char **name_ptr) {
    DICT *dict = (DICT *)ptr;

    debug(D_DICTIONARY, "SET dictionary entry with name '%s'.", name);

    size_t name_len = strlen(name) + 1;
    dictionary_write_lock(dict);

    NAME_VALUE *nv, **pnv = hashtable_insert_unsafe(dict, name, name_len);
    if(likely(*pnv == 0)) {
        // a new item added to the index
        nv = *pnv = namevalue_create_unsafe(dict, name, name_len, value, value_len);
        linkedlist_namevalue_link_last(dict, nv);
    }
    else {
        // the item is already in the index
        // so, either we will return the old one
        // or overwrite the value, depending on dictionary flags

        nv = *pnv;
        if(!(dict->flags & DICTIONARY_FLAG_DONT_OVERWRITE_VALUE))
            namevalue_reset_unsafe(dict, nv, value, value_len);
    }

    dictionary_unlock(dict);

    if(name_ptr) *name_ptr = nv->name;
    return nv->value;
}

void *dictionary_get(DICTIONARY *ptr, const char *name) {
    DICT *dict = (DICT *)ptr;

    debug(D_DICTIONARY, "GET dictionary entry with name '%s'.", name);

    dictionary_read_lock(dict);
    NAME_VALUE *nv = hashtable_get_unsafe(dict, name, strlen(name) + 1);
    dictionary_unlock(dict);

    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        return NULL;
    }

    debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
    return nv->value;
}

int dictionary_del(DICTIONARY *ptr, const char *name) {
    DICT *dict = (DICT *)ptr;
    size_t name_len = strlen(name) + 1;

    int ret;

    debug(D_DICTIONARY, "DEL dictionary entry with name '%s'.", name);

    dictionary_write_lock(dict);

    // Unfortunately, the JudyHSDel() does not return the value of the
    // item that was deleted, so we have to find it before we delete it,
    // since we need to release our structures too.

    NAME_VALUE *nv = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        ret = -1;
    }
    else {
        debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
        hashtable_delete_unsafe(dict, name, name_len);
        linkedlist_namevalue_unlink(dict, nv);
        namevalue_destroy_unsafe(dict, nv);
        ret = 0;
    }

    dictionary_unlock(dict);

    return ret;
}


// ----------------------------------------------------------------------------
// API - walk through the dictionary
// the dictionary is locked for reading while this happens
// do not user other dictionary calls while walking the dictionary - deadlock!

struct drop_name_callback_data {
    int (*callback)(void *entry, void *data);
    void *callback_data;
};

int drop_name_callback(const char *name, void *value, void *data) {
    (void)name;
    struct drop_name_callback_data *mydata = (struct drop_name_callback_data *)data;
    return mydata->callback(value, mydata->callback_data);
}

int dictionary_walkthrough(DICTIONARY *ptr, int (*callback)(void *entry, void *data), void *data) {
    DICT *dict = (DICT *)ptr;

    struct drop_name_callback_data mydata = {
        .callback = callback,
        .callback_data = data
    };

    dictionary_read_lock(dict);
    int ret = linkedlist_namevalue_walkthrough_unsafe(dict, drop_name_callback, &mydata);
    dictionary_unlock(dict);

    return ret;
}

int dictionary_walkthrough_with_name(DICTIONARY *ptr, int (*callback)(const char *name, void *entry, void *data), void *data) {
    DICT *dict = (DICT *)ptr;

    dictionary_read_lock(dict);
    int ret = linkedlist_namevalue_walkthrough_unsafe(dict, callback, data);
    dictionary_unlock(dict);

    return ret;
}

// ----------------------------------------------------------------------------
// unit test

int verify_name_and_value_of_cloning_dictionary(const char *name, void *value, void *data) {
    (void)data;

    int ret = 0;

    if(name == value) {
        fprintf(stderr, "ERROR: name and value should not use the same memory\n");
        ret++;
    }

    if(strcmp(name, (char *)value) != 0) {
        fprintf(stderr, "ERROR: expected '%s', got '%s'\n", name, (char *)value);
        ret++;
    }

    return ret;
}

int dictionary_unittest(size_t entries) {
    char buf[1024];
    size_t i, errors = 0;

    clocks_init();

    usec_t creating = now_realtime_usec();

    DICTIONARY *d = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED|DICTIONARY_FLAG_WITH_STATISTICS);
    fprintf(stderr, "%zu x dictionary_set() (dictionary size %zu entries, %zu KB)...\n", entries, dictionary_entries(d), dictionary_allocated_memory(d) / 1024);
    for(i = 0; i < entries ;i++) {
        size_t len = snprintfz(buf, 1024, "string %zu", i);
        char *s = dictionary_set(d, buf, buf, len + 1);
        if(strcmp(s, buf) != 0) {
            fprintf(stderr, "ERROR: expected to add '%s', got '%s'\n", buf, s);
            errors++;
        }
    }

    usec_t creating_end = now_realtime_usec();
    usec_t positive_checking = now_realtime_usec();

    fprintf(stderr, "%zu x dictionary_get(existing) (dictionary size %zu entries, %zu KB)...\n", entries, dictionary_entries(d), dictionary_allocated_memory(d) / 1024);
    for(i = 0; i < entries ;i++) {
        snprintfz(buf, 1024, "string %zu", i);
        char *s = dictionary_get(d, buf);
        if(strcmp(s, buf) != 0) {
            fprintf(stderr, "ERROR: expected '%s', got '%s'\n", buf, s);
            errors++;
        }
    }

    usec_t positive_checking_end = now_realtime_usec();
    usec_t negative_checking = now_realtime_usec();

    fprintf(stderr, "%zu x dictionary_get(non-existing) (dictionary size %zu entries, %zu KB)...\n", entries, dictionary_entries(d), dictionary_allocated_memory(d) / 1024);
    for(i = 0; i < entries ;i++) {
        snprintfz(buf, 1024, "string %zu not matching anything", i);
        char *s = dictionary_get(d, buf);
        if(s) {
            fprintf(stderr, "ERROR: found an expected item, searching for '%s', got '%s'\n", buf, s);
            errors++;
        }
    }

    usec_t negative_checking_end = now_realtime_usec();
    usec_t walking = now_realtime_usec();

    fprintf(stderr, "Walking through the dictionary (dictionary size %zu entries, %zu KB)...\n", dictionary_entries(d), dictionary_allocated_memory(d) / 1024);
    errors += dictionary_walkthrough_with_name(d, verify_name_and_value_of_cloning_dictionary, NULL);

    usec_t walking_end = now_realtime_usec();
    usec_t deleting = now_realtime_usec();

    fprintf(stderr, "%zu x dictionary_del(existing) (dictionary size %zu entries, %zu KB)...\n", entries, dictionary_entries(d), dictionary_allocated_memory(d) / 1024);
    for(i = 0; i < entries ;i++) {
        snprintfz(buf, 1024, "string %zu", i);
        int ret = dictionary_del(d, buf);
        if(ret == -1) {
            fprintf(stderr, "ERROR: expected to delete '%s' but it failed\n", buf);
            errors++;
        }
    }

    usec_t deleting_end = now_realtime_usec();

    fprintf(stderr, "%zu x dictionary_set() (dictionary size %zu entries, %zu KB)...\n", entries, dictionary_entries(d), dictionary_allocated_memory(d) / 1024);
    for(i = 0; i < entries ;i++) {
        size_t len = snprintfz(buf, 1024, "string %zu", i);
        char *s = dictionary_set(d, buf, buf, len + 1);
        if(strcmp(s, buf) != 0) {
            fprintf(stderr, "ERROR: expected to add '%s', got '%s'\n", buf, s);
            errors++;
        }
    }

    usec_t destroying = now_realtime_usec();

    fprintf(stderr, "Destroying dictionary (dictionary size %zu entries, %zu KB)...\n", dictionary_entries(d), dictionary_allocated_memory(d) / 1024);
    dictionary_destroy(d);

    usec_t destroying_end = now_realtime_usec();

    fprintf(stderr, "\nTIMINGS:\nadding %llu usec, positive search %llu usec, negative search %llu, walk through %llu usec, deleting %llu, destroy %llu usec\n",
            creating_end - creating,
            positive_checking_end - positive_checking,
            negative_checking_end - negative_checking,
            walking_end - walking,
            deleting_end - deleting,
            destroying_end - destroying);

    return (int)errors;
}
