// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "Judy.h"

struct dictionary_stats {
    size_t inserts;
    size_t deletes;
    size_t searches;
    size_t resets;
    size_t entries;
    size_t memory;
};

typedef struct name_value {
    char *name;
    void *value;

    size_t name_len;
    size_t value_len;
} NAME_VALUE;

typedef struct dictionary {
    Pvoid_t JudyArray;
    size_t JudyArrayMaxNameLen;

    uint8_t flags;

    struct dictionary_stats *stats;
    netdata_rwlock_t *rwlock;
} DICT;

// ----------------------------------------------------------------------------
// dictionary statistics

size_t dictionary_allocated_memory(DICTIONARY *ptr) {
    DICT *dict = (DICT *)ptr;

    if(dict->stats)
        return dict->stats->memory;

    return 0;
}

static inline void DICTIONARY_STATS_SEARCHES_PLUS1(DICT *dict) {
    if(likely(dict->stats))
        dict->stats->searches++;
}
static inline void DICTIONARY_STATS_ENTRIES_PLUS1(DICT *dict, size_t size) {
    if(likely(dict->stats)) {
        dict->stats->inserts++;
        dict->stats->entries++;
        dict->stats->memory += size;
    }
}
static inline void DICTIONARY_STATS_ENTRIES_MINUS1(DICT *dict, size_t size) {
    if(likely(dict->stats)) {
        dict->stats->deletes++;
        dict->stats->entries--;
        dict->stats->memory -= size;
    }
}
static inline void DICTIONARY_STATS_VALUE_RESETS_PLUS1(DICT *dict, size_t oldsize, size_t newsize) {
    if(likely(dict->stats)) {
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
// dictionary index

static void dictionary_index_init_unsafe(DICT *dict) {
    dict->JudyArray = NULL;
    dict->JudyArrayMaxNameLen = 0;
}

static void dictionary_index_destroy_unsafe(DICT *dict) {
    if(unlikely(!dict->JudyArray)) return;

    JError_t J_Error;
    Word_t ret = JudySLFreeArray(&dict->JudyArray, &J_Error);
    if(unlikely(ret == (Word_t) JERR)) {
        error("DICTIONARY: Cannot destroy JudySL, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }
    dict->JudyArray = NULL;
    dict->JudyArrayMaxNameLen = 0;
}

static inline void dictionary_index_insert_unsafe(DICT *dict, NAME_VALUE *nv) {
    JError_t J_Error;
    Pvoid_t *Rc;

    const char *name = nv->name;

    Rc = JudySLIns(&dict->JudyArray, (const uint8_t *)name, &J_Error);
    if(unlikely(Rc == PJERR)) {
        error("DICTIONARY: Cannot insert entry with name '%s' to JudySL, JU_ERRNO_* == %u, ID == %d", name,
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }
    else {
        *Rc = nv;

        size_t name_len = (nv->name_len)?nv->name_len:strlen(name) + 1;
        if(dict->JudyArrayMaxNameLen < name_len) dict->JudyArrayMaxNameLen = name_len;
    }
}

static inline void dictionary_index_delete_unsafe(DICT *dict, const char *name) {
    if(unlikely(!dict->JudyArray)) return;

    JError_t J_Error;
    int ret;

    ret = JudySLDel(&dict->JudyArray, (const uint8_t *)name, &J_Error);
    if(unlikely(ret == JERR)) {
        error("DICTIONARY: Cannot delete entry with name '%s' from JudySL, JU_ERRNO_* == %u, ID == %d", name,
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
        return;
    }

    if(ret == 0)
        error("DICTIONARY: Attempted to delete entry with name '%s', but it was not found in the index", name);
}

static inline NAME_VALUE *dictionary_index_get_unsafe(DICT *dict, const char *name) {
    if(unlikely(!dict->JudyArray)) return NULL;

    JError_t J_Error;
    Pvoid_t *Rc;

    Rc = JudySLGet(dict->JudyArray, (const uint8_t *)name, &J_Error);
    if(unlikely(Rc == PJERR)) {
        error("DICTIONARY: Cannot get entry with name '%s' to JudySL, JU_ERRNO_* == %u, ID == %d", name,
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
        return NULL;
    }
    else if(likely(Rc))
        return (NAME_VALUE *)*Rc;
    else
        return NULL;
}

static inline NAME_VALUE *dictionary_index_get_first_unsafe(DICT *dict) {
    if(unlikely(!dict->JudyArray)) return NULL;

    char Index[dict->JudyArrayMaxNameLen]; Index[0] = '\0';

    JError_t J_Error;
    Pvoid_t *Rc;

    Rc = JudySLFirst(dict->JudyArray, (uint8_t *)&Index, &J_Error);
    if(unlikely(Rc == PJERR)) {
        error("DICTIONARY: Cannot find the first entry of JudySL, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
        return NULL;
    }
    else if(likely(Rc))
        return (NAME_VALUE *)*Rc;
    else
        return NULL;
}

static inline int dictionary_index_walkthrough_unsafe(DICT *dict, int (*callback)(const char *name, void *value, void *data), void *data) {
    if(unlikely(!dict->JudyArray)) return 0;

    char Index[dict->JudyArrayMaxNameLen]; Index[0] = '\0';

    JError_t J_Error;
    Pvoid_t *Rc;

    Rc = JudySLFirst(dict->JudyArray, (uint8_t *)&Index, &J_Error);
    if(unlikely(Rc == PJERR)) {
        error("DICTIONARY: Cannot find the first entry of JudySL, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
        return 0;
    }

    int ret = 0;
    while(Rc && *Rc) {
        NAME_VALUE *nv = *Rc;
        int r = callback(nv->name, nv->value, data);
        if(unlikely(r < 0)) return r;

        ret += r;

        Rc = JudySLNext(dict->JudyArray, (uint8_t *)&Index, &J_Error);
        if(unlikely(Rc == PJERR)) {
            error("DICTIONARY: Cannot find next entry of JudySL, JU_ERRNO_* == %u, ID == %d",
                  JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
            break;
        }
    }
    return ret;
}


// ----------------------------------------------------------------------------
// NAME_VALUE methods

static NAME_VALUE *dictionary_name_value_create_unsafe(DICT *dict, const char *name, void *value, size_t value_len) {
    debug(D_DICTIONARY, "Creating name value entry for name '%s'.", name);

    NAME_VALUE *nv = mallocz(sizeof(NAME_VALUE));

    if(dict->flags & DICTIONARY_FLAG_NAME_LINK_DONT_CLONE) {
        nv->name_len = 0;
        nv->name = (char *)name;
    }
    else {
        nv->name_len = strlen(name) + 1;
        nv->name = mallocz(nv->name_len);
        memcpy(nv->name, name, nv->name_len);
    }

    if(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE) {
        nv->value_len = 0;
        nv->value = value;
    }
    else {
        nv->value_len = value_len;
        nv->value = mallocz(value_len);
        memcpy(nv->value, value, value_len);
    }

    // index it
    dictionary_index_insert_unsafe(dict, nv);
    DICTIONARY_STATS_ENTRIES_PLUS1(dict, sizeof(NAME_VALUE) + nv->name_len + nv->value_len);

    return nv;
}

static void dictionary_name_value_destroy_unsafe(DICT *dict, NAME_VALUE *nv) {
    debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", nv->name);

    dictionary_index_delete_unsafe(dict, nv->name);

    if(!(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE)) {
        debug(D_DICTIONARY, "Dictionary freeing value of '%s'", nv->name);
        freez(nv->value);
    }

    if(!(dict->flags & DICTIONARY_FLAG_NAME_LINK_DONT_CLONE)) {
        debug(D_DICTIONARY, "Dictionary freeing name '%s'", nv->name);
        freez(nv->name);
    }

    DICTIONARY_STATS_ENTRIES_MINUS1(dict, sizeof(NAME_VALUE) + nv->name_len + nv->value_len);

    freez(nv);
}

// ----------------------------------------------------------------------------
// API - basic methods

DICTIONARY *dictionary_create(uint8_t flags) {
    debug(D_DICTIONARY, "Creating dictionary.");

    DICT *dict = mallocz(sizeof(DICT));
    dict->flags = flags;

    if(!(flags & DICTIONARY_FLAG_SINGLE_THREADED)) {
        dict->rwlock = mallocz(sizeof(netdata_rwlock_t));
        netdata_rwlock_init(dict->rwlock);
    }
    else
        dict->rwlock = NULL;

    if(flags & DICTIONARY_FLAG_WITH_STATISTICS) {
        dict->stats = callocz(1, sizeof(struct dictionary_stats));
        dict->stats->memory = sizeof(DICT) + sizeof(struct dictionary_stats) + (flags & DICTIONARY_FLAG_SINGLE_THREADED)?sizeof(netdata_rwlock_t):0;
    }
    else
        dict->stats = NULL;

    dictionary_index_init_unsafe(dict);
    return (DICTIONARY *)dict;
}

void dictionary_destroy(DICTIONARY *ptr) {
    DICT *dict = (DICT *)ptr;

    debug(D_DICTIONARY, "Destroying dictionary.");

    dictionary_write_lock(dict);

    if(dict->stats) {
        freez(dict->stats);
        dict->stats = NULL;
    }

    NAME_VALUE *nv;
    while((nv = dictionary_index_get_first_unsafe(dict)))
        dictionary_name_value_destroy_unsafe(dict, nv);

    dictionary_index_destroy_unsafe(dict);

    dictionary_unlock(dict);

    if(dict->rwlock) {
        netdata_rwlock_destroy(dict->rwlock);
        freez(dict->rwlock);
    }

    freez(dict);
}

// ----------------------------------------------------------------------------

void *dictionary_set_with_name_ptr(DICTIONARY *ptr, const char *name, void *value, size_t value_len, char **name_ptr) {
    DICT *dict = (DICT *)ptr;

    debug(D_DICTIONARY, "SET dictionary entry with name '%s'.", name);

    dictionary_write_lock(dict);

    NAME_VALUE *nv = dictionary_index_get_unsafe(dict, name);
    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Dictionary entry with name '%s' not found. Creating a new one.", name);

        nv = dictionary_name_value_create_unsafe(dict, name, value, value_len);
        if(unlikely(!nv))
            fatal("Cannot create name_value.");
    }
    else if(!(dict->flags & DICTIONARY_FLAG_DONT_OVERWRITE_VALUE)) {
        debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", name);

        if(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE) {
            debug(D_DICTIONARY, "Dictionary: linking value to '%s'", name);
            nv->value = value;
        }
        else {
            DICTIONARY_STATS_VALUE_RESETS_PLUS1(dict, nv->value_len, value_len);

            debug(D_DICTIONARY, "Dictionary: cloning value to '%s'", name);

            // copy the new value without breaking
            // any other thread accessing the same entry
            void *old = nv->value;

            void *new = mallocz(value_len);
            memcpy(new, value, value_len);
            nv->value = new;
            nv->value_len = value_len;

            debug(D_DICTIONARY, "Dictionary: freeing old value of '%s'", name);
            freez(old);
        }
    }

    dictionary_unlock(dict);

    if(name_ptr) *name_ptr = nv->name;
    return nv->value;
}

void *dictionary_get(DICTIONARY *ptr, const char *name) {
    DICT *dict = (DICT *)ptr;

    debug(D_DICTIONARY, "GET dictionary entry with name '%s'.", name);

    dictionary_read_lock(dict);
    NAME_VALUE *nv = dictionary_index_get_unsafe(dict, name);
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

    int ret;

    debug(D_DICTIONARY, "DEL dictionary entry with name '%s'.", name);

    dictionary_write_lock(dict);

    NAME_VALUE *nv = dictionary_index_get_unsafe(dict, name);
    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        ret = -1;
    }
    else {
        debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
        dictionary_name_value_destroy_unsafe(dict, nv);
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
    int ret = dictionary_index_walkthrough_unsafe(dict, drop_name_callback, &mydata);
    dictionary_unlock(dict);

    return ret;
}

int dictionary_walkthrough_with_name(DICTIONARY *ptr, int (*callback)(const char *name, void *entry, void *data), void *data) {
    DICT *dict = (DICT *)ptr;

    dictionary_read_lock(dict);
    int ret = dictionary_index_walkthrough_unsafe(dict, callback, data);
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

    clocks_init();

    usec_t started = now_realtime_usec();
    DICTIONARY *d = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED|DICTIONARY_FLAG_WITH_STATISTICS);
    fprintf(stderr, "Creating dictionary of %zu entries...\n", entries);
    size_t i, errors = 0;
    for(i = 0; i < entries ;i++) {
        size_t len = snprintfz(buf, 1024, "string %zu", i);
        if(len != strlen(buf)) fprintf(stderr, "Expected length %zu, got %zu\n", strlen(buf), len);

        dictionary_set(d, buf, buf, len + 1);
    }

    usec_t checking = now_realtime_usec();

    fprintf(stderr, "Checking index of %zu entries...\n", entries);
    for(i = 0; i < entries ;i++) {
        snprintfz(buf, 1024, "string %zu", i);
        char *s = dictionary_get(d, buf);
        if(strcmp(s, buf) != 0) {
            fprintf(stderr, "ERROR: expected '%s', got '%s'\n", buf, s);
            errors++;
        }
    }

    usec_t walking = now_realtime_usec();

    fprintf(stderr, "Walking %zu entries and checking name-value pairs...\n", entries);
    errors += dictionary_walkthrough_with_name(d, verify_name_and_value_of_cloning_dictionary, NULL);

    fprintf(stderr, "Created and checked %zu entries, found %zu errors - used %zu KB of memory\n", i, errors, dictionary_allocated_memory(d)/ 1024);

    usec_t destroying = now_realtime_usec();
    fprintf(stderr, "Destroying dictionary of %zu entries...\n", entries);
    dictionary_destroy(d);

    fprintf(stderr, "create %llu usec, check %llu usec, walk %llu usec, destroy %llu usec\n",
            checking - started, walking - checking, destroying - walking, now_realtime_usec() - destroying);

    return (int)errors;
}
