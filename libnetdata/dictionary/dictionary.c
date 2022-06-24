// SPDX-License-Identifier: GPL-3.0-or-later

// NOT TO BE USED BY USERS YET
#define DICTIONARY_FLAG_EXCLUSIVE_ACCESS   (1 << 29) // there is only one thread accessing the dictionary

typedef struct dictionary DICTIONARY;
#define DICTIONARY_INTERNALS

#include "../libnetdata.h"

#ifndef ENABLE_DBENGINE
#define DICTIONARY_WITH_AVL
#warning Compiling DICTIONARY with an AVL index
#else
#define DICTIONARY_WITH_JUDYHS
#endif

#ifdef DICTIONARY_WITH_JUDYHS
#include <Judy.h>
#endif

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

typedef enum name_value_flags {
    NAME_VALUE_FLAG_NONE                   = 0,
    NAME_VALUE_FLAG_DELETED                = (1 << 0), // this item is deleted
} NAME_VALUE_FLAGS;


/*
 * Every item in the dictionary has the following structure.
 */
typedef struct name_value {
#ifdef DICTIONARY_WITH_AVL
    avl_t avl_node;
#endif

    struct name_value *next;    // a double linked list to allow fast insertions and deletions
    struct name_value *prev;

    size_t name_len;            // the size of the name, including the terminating zero
    size_t value_len;           // the size of the value (assumed binary)

    void *value;                // the value of the dictionary item
    char *name;                 // the name of the dictionary item

    int refcount;               // the reference counter
    NAME_VALUE_FLAGS flags;     // the flags for this item
} NAME_VALUE;

struct dictionary {
    DICTIONARY_FLAGS flags;             // the flags of the dictionary

    NAME_VALUE *first_item;             // the double linked list base pointers
    NAME_VALUE *last_item;

#ifdef DICTIONARY_WITH_AVL
    avl_tree_type values_index;
    NAME_VALUE *hash_base;
#endif

#ifdef DICTIONARY_WITH_JUDYHS
    Pvoid_t JudyHSArray;                // the hash table
#endif

    netdata_rwlock_t *rwlock;           // the r/w lock when DICTIONARY_FLAG_SINGLE_THREADED is not set

    void (*ins_callback)(const char *name, void *value, void *data);
    void *ins_callback_data;

    void (*del_callback)(const char *name, void *value, void *data);
    void *del_callback_data;

    void (*conflict_callback)(const char *name, void *old_value, void *new_value, void *data);
    void *conflict_callback_data;

    size_t inserts;
    size_t deletes;
    size_t searches;
    size_t resets;
    size_t entries;
    size_t walkthroughs;
    size_t memory;
};

void dictionary_register_insert_callback(DICTIONARY *dict, void (*ins_callback)(const char *name, void *value, void *data), void *data) {
    dict->ins_callback = ins_callback;
    dict->ins_callback_data = data;
}

void dictionary_register_delete_callback(DICTIONARY *dict, void (*del_callback)(const char *name, void *value, void *data), void *data) {
    dict->del_callback = del_callback;
    dict->del_callback_data = data;
}

void dictionary_register_conflict_callback(DICTIONARY *dict, void (*conflict_callback)(const char *name, void *old_value, void *new_value, void *data), void *data) {
    dict->conflict_callback = conflict_callback;
    dict->conflict_callback_data = data;
}

// ----------------------------------------------------------------------------
// dictionary statistics maintenance

size_t dictionary_stats_allocated_memory(DICTIONARY *dict) {
    return dict->memory;
}
size_t dictionary_stats_entries(DICTIONARY *dict) {
    return dict->entries;
}
size_t dictionary_stats_searches(DICTIONARY *dict) {
    return dict->searches;
}
size_t dictionary_stats_inserts(DICTIONARY *dict) {
    return dict->inserts;
}
size_t dictionary_stats_deletes(DICTIONARY *dict) {
    return dict->deletes;
}
size_t dictionary_stats_resets(DICTIONARY *dict) {
    return dict->resets;
}
size_t dictionary_stats_walkthroughs(DICTIONARY *dict) {
    return dict->walkthroughs;
}

static inline void DICTIONARY_STATS_SEARCHES_PLUS1(DICTIONARY *dict) {
    if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS) {
        dict->searches++;
    }
    else {
        __atomic_fetch_add(&dict->searches, 1, __ATOMIC_RELAXED);
    }
}
static inline void DICTIONARY_STATS_ENTRIES_PLUS1(DICTIONARY *dict, size_t size) {
    if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS) {
        dict->inserts++;
        dict->entries++;
        dict->memory += size;
    }
    else {
        __atomic_fetch_add(&dict->inserts, 1, __ATOMIC_RELAXED);
        __atomic_fetch_add(&dict->entries, 1, __ATOMIC_RELAXED);
        __atomic_fetch_add(&dict->memory, size, __ATOMIC_RELAXED);
    }
}
static inline void DICTIONARY_STATS_ENTRIES_MINUS1(DICTIONARY *dict) {
    if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS) {
        dict->deletes++;
        dict->entries--;
    }
    else {
        __atomic_fetch_add(&dict->deletes, 1, __ATOMIC_RELAXED);
        __atomic_fetch_sub(&dict->entries, 1, __ATOMIC_RELAXED);
    }
}
static inline void DICTIONARY_STATS_ENTRIES_MINUS_MEMORY(DICTIONARY *dict, size_t size) {
    if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS) {
        dict->memory -= size;
    }
    else {
        __atomic_fetch_sub(&dict->memory, size, __ATOMIC_RELAXED);
    }
}
static inline void DICTIONARY_STATS_VALUE_RESETS_PLUS1(DICTIONARY *dict, size_t oldsize, size_t newsize) {
    if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS) {
        dict->resets++;
        dict->memory += newsize;
        dict->memory -= oldsize;
    }
    else {
        __atomic_fetch_add(&dict->resets, 1, __ATOMIC_RELAXED);
        __atomic_fetch_add(&dict->memory, newsize, __ATOMIC_RELAXED);
        __atomic_fetch_sub(&dict->memory, oldsize, __ATOMIC_RELAXED);
    }
}

static inline void DICTIONARY_STATS_WALKTHROUGHS_PLUS1(DICTIONARY *dict) {
    if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS) {
        dict->walkthroughs++;
    }
    else {
        __atomic_fetch_add(&dict->walkthroughs, 1, __ATOMIC_RELAXED);
    }
}

// ----------------------------------------------------------------------------
// dictionary locks

static inline size_t dictionary_lock_init(DICTIONARY *dict) {
    if(likely(!(dict->flags & DICTIONARY_FLAG_SINGLE_THREADED))) {
            dict->rwlock = mallocz(sizeof(netdata_rwlock_t));
            netdata_rwlock_init(dict->rwlock);

            if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)
                dict->flags &= ~DICTIONARY_FLAG_EXCLUSIVE_ACCESS;

            return sizeof(netdata_rwlock_t);
    }

    dict->flags |= DICTIONARY_FLAG_EXCLUSIVE_ACCESS;
    dict->rwlock = NULL;
    return 0;
}

static inline size_t dictionary_lock_free(DICTIONARY *dict) {
    if(likely(!(dict->flags & DICTIONARY_FLAG_SINGLE_THREADED))) {
        netdata_rwlock_destroy(dict->rwlock);
        freez(dict->rwlock);
        return sizeof(netdata_rwlock_t);
    }
    return 0;
}

static inline void dictionary_lock_rdlock(DICTIONARY *dict) {
    if(likely(!(dict->flags & DICTIONARY_FLAG_SINGLE_THREADED))) {
        // debug(D_DICTIONARY, "Dictionary READ lock");
        netdata_rwlock_rdlock(dict->rwlock);

        if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)
            dict->flags &= ~DICTIONARY_FLAG_EXCLUSIVE_ACCESS;
    }
}

static inline void dictionary_lock_wrlock(DICTIONARY *dict) {
    if(likely(!(dict->flags & DICTIONARY_FLAG_SINGLE_THREADED))) {
        // debug(D_DICTIONARY, "Dictionary WRITE lock");
        netdata_rwlock_wrlock(dict->rwlock);

        dict->flags |= DICTIONARY_FLAG_EXCLUSIVE_ACCESS;
    }
}

static inline void dictionary_unlock(DICTIONARY *dict) {
    if(likely(!(dict->flags & DICTIONARY_FLAG_SINGLE_THREADED))) {
        // debug(D_DICTIONARY, "Dictionary UNLOCK lock");
        netdata_rwlock_unlock(dict->rwlock);

        if(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)
            dict->flags &= ~DICTIONARY_FLAG_EXCLUSIVE_ACCESS;
    }
}

// ----------------------------------------------------------------------------
// reference counters

static inline size_t reference_counter_init(DICTIONARY *dict) {
    (void)dict;

    // allocate memory required for reference counters
    // return number of bytes
    return 0;
}

static inline size_t reference_counter_free(DICTIONARY *dict) {
    (void)dict;

    // free memory required for reference counters
    // return number of bytes
    return 0;
}

static int reference_counter_acquire(DICTIONARY *dict, NAME_VALUE *nv) {
    (void)dict;
    return __atomic_add_fetch(&nv->refcount, 1, __ATOMIC_SEQ_CST);
}

static inline void linkedlist_namevalue_unlink_unsafe(DICTIONARY *dict, NAME_VALUE *nv);
static size_t namevalue_destroy_unsafe(DICTIONARY *dict, NAME_VALUE *nv);

static int reference_counter_release(DICTIONARY *dict, NAME_VALUE *nv) {
    (void)dict;
    int refcount = __atomic_sub_fetch(&nv->refcount, 1, __ATOMIC_SEQ_CST);

    if((nv->flags & NAME_VALUE_FLAG_DELETED) && !refcount) {
        linkedlist_namevalue_unlink_unsafe(dict, nv);
        namevalue_destroy_unsafe(dict, nv);
    }

    return refcount;
}

static int reference_counter_mark_deleted(DICTIONARY *dict, NAME_VALUE *nv) {
    (void)dict;
    int refcount = __atomic_load_n(&nv->refcount, __ATOMIC_SEQ_CST);
    if(refcount) {
        nv->flags |= NAME_VALUE_FLAG_DELETED;
        return 1;
    }
    return 0;
}

// ----------------------------------------------------------------------------
// hash table

#ifdef DICTIONARY_WITH_AVL
static int name_value_compare(void* a, void* b) {
    return strcmp(((NAME_VALUE *)a)->name, ((NAME_VALUE *)b)->name);
}

static void hashtable_init_unsafe(DICTIONARY *dict) {
    avl_init(&dict->values_index, name_value_compare);
}

static size_t hashtable_destroy_unsafe(DICTIONARY *dict) {
    (void)dict;
    return 0;
}

static inline int hashtable_delete_unsafe(DICTIONARY *dict, const char *name, size_t name_len, NAME_VALUE *nv) {
    (void)name;
    (void)name_len;

    if(unlikely(avl_remove(&(dict->values_index), (avl_t *)(nv)) != (avl_t *)nv))
        return 0;

    return 1;
}

static inline NAME_VALUE *hashtable_get_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
    (void)name_len;

    NAME_VALUE tmp;
    tmp.name = (char *)name;
    return (NAME_VALUE *)avl_search(&(dict->values_index), (avl_t *) &tmp);
}

static inline NAME_VALUE **hashtable_insert_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
    // AVL needs a NAME_VALUE to insert into the dictionary but we don't have it yet.
    // So, the only thing we can do, is return an existing one if it is already there.
    // Returning NULL will make the caller thing we added it, will allocate one
    // and will call hashtable_inserted_name_value_unsafe(), at which we will do
    // the actual indexing.

    dict->hash_base = hashtable_get_unsafe(dict, name, name_len);
    return &dict->hash_base;
}

static inline void hashtable_inserted_name_value_unsafe(DICTIONARY *dict, const char *name, size_t name_len, NAME_VALUE *nv) {
    // we have our new NAME_VALUE object.
    // Let's index it.

    (void)name;
    (void)name_len;

    if(unlikely(avl_insert(&((dict)->values_index), (avl_t *)(nv)) != (avl_t *)nv))
        error("dictionary: INTERNAL ERROR: duplicate insertion to dictionary.");
}
#endif

#ifdef DICTIONARY_WITH_JUDYHS
static void hashtable_init_unsafe(DICTIONARY *dict) {
    dict->JudyHSArray = NULL;
}

static size_t hashtable_destroy_unsafe(DICTIONARY *dict) {
    if(unlikely(!dict->JudyHSArray)) return 0;

    JError_t J_Error;
    Word_t ret = JudyHSFreeArray(&dict->JudyHSArray, &J_Error);
    if(unlikely(ret == (Word_t) JERR)) {
        error("DICTIONARY: Cannot destroy JudyHS, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    debug(D_DICTIONARY, "Dictionary: hash table freed %lu bytes", ret);

    dict->JudyHSArray = NULL;
    return (size_t)ret;
}

static inline NAME_VALUE **hashtable_insert_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(!(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)))
        error("DICTIONARY: INTERNAL ERROR: inserting to the index without exclusive access to the dictionary.");
#endif

    JError_t J_Error;
    Pvoid_t *Rc = JudyHSIns(&dict->JudyHSArray, (void *)name, name_len, &J_Error);
    if (unlikely(Rc == PJERR)) {
        fatal("DICTIONARY: Cannot insert entry with name '%s' to JudyHS, JU_ERRNO_* == %u, ID == %d",
              name, JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    // if *Rc == 0, new item added to the array
    // otherwise the existing item value is returned in *Rc

    // we return a pointer to a pointer, so that the caller can
    // put anything needed at the value of the index.
    // The pointer to pointer we return has to be used before
    // any other operation that may change the index (insert/delete).
    return (NAME_VALUE **)Rc;
}

static inline int hashtable_delete_unsafe(DICTIONARY *dict, const char *name, size_t name_len, NAME_VALUE *nv) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(!(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)))
        error("DICTIONARY: INTERNAL ERROR: deleting from the index without exclusive access to the dictionary.");
#endif

    (void)nv;
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

static inline NAME_VALUE *hashtable_get_unsafe(DICTIONARY *dict, const char *name, size_t name_len) {
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

static inline void hashtable_inserted_name_value_unsafe(DICTIONARY *dict, const char *name, size_t name_len, NAME_VALUE *nv) {
    (void)dict;
    (void)name;
    (void)name_len;
    (void)nv;
    ;
}

#endif // DICTIONARY_WITH_JUDYHS

// ----------------------------------------------------------------------------
// linked list management

static inline void linkedlist_namevalue_link_unsafe(DICTIONARY *dict, NAME_VALUE *nv) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(!(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)))
        error("DICTIONARY: INTERNAL ERROR: adding item to the linked-list without exclusive access to the dictionary.");
#endif

    if (unlikely(!dict->first_item)) {
        // we are the only ones here
        nv->next = NULL;
        nv->prev = NULL;
        dict->first_item = dict->last_item = nv;
        return;
    }

    if(dict->flags & DICTIONARY_FLAG_ADD_IN_FRONT) {
        // add it at the beginning
        nv->prev = NULL;
        nv->next = dict->first_item;

        if (likely(nv->next)) nv->next->prev = nv;
        dict->first_item = nv;
    }
    else {
        // add it at the end
        nv->next = NULL;
        nv->prev = dict->last_item;

        if (likely(nv->prev)) nv->prev->next = nv;
        dict->last_item = nv;
    }
}

static inline void linkedlist_namevalue_unlink_unsafe(DICTIONARY *dict, NAME_VALUE *nv) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(!(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)))
        error("DICTIONARY: INTERNAL ERROR: removing item from the linked-list without exclusive access to the dictionary.");
#endif

    if(nv->next) nv->next->prev = nv->prev;
    if(nv->prev) nv->prev->next = nv->next;
    if(dict->first_item == nv) dict->first_item = nv->next;
    if(dict->last_item == nv) dict->last_item = nv->prev;
}

// ----------------------------------------------------------------------------
// NAME_VALUE methods

static NAME_VALUE *namevalue_create_unsafe(DICTIONARY *dict, const char *name, size_t name_len, void *value, size_t value_len) {
    debug(D_DICTIONARY, "Creating name value entry for name '%s'.", name);

    size_t size = sizeof(NAME_VALUE);
    NAME_VALUE *nv = mallocz(size);
    size_t allocated = size;

    nv->refcount = 0;
    nv->flags = NAME_VALUE_FLAG_NONE;
    nv->name_len = name_len;
    nv->value_len = value_len;

    if(likely(dict->flags & DICTIONARY_FLAG_NAME_LINK_DONT_CLONE))
        nv->name = (char *)name;
    else {
        nv->name = mallocz(name_len);
        memcpy(nv->name, name, name_len);
        allocated += name_len;
    }

    if(likely(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE))
        nv->value = value;
    else {
        if(likely(value_len)) {
            if (value) {
                // a value has been supplied
                // copy it
                nv->value = mallocz(value_len);
                memcpy(nv->value, value, value_len);
            }
            else {
                // no value has been supplied
                // allocate a clear memory block
                nv->value = callocz(1, value_len);
            }
        }
        else {
            // the caller wants an item without any value
            nv->value = NULL;
        }

        allocated += value_len;
    }

    DICTIONARY_STATS_ENTRIES_PLUS1(dict, allocated);

    if(dict->ins_callback)
        dict->ins_callback(nv->name, nv->value, dict->ins_callback_data);

    return nv;
}

static void namevalue_reset_unsafe(DICTIONARY *dict, NAME_VALUE *nv, void *value, size_t value_len) {
    debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", nv->name);

    DICTIONARY_STATS_VALUE_RESETS_PLUS1(dict, nv->value_len, value_len);

    if(dict->del_callback)
        dict->del_callback(nv->name, nv->value, dict->del_callback_data);

    if(likely(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE)) {
        debug(D_DICTIONARY, "Dictionary: linking value to '%s'", nv->name);
        nv->value = value;
        nv->value_len = value_len;
    }
    else {
        debug(D_DICTIONARY, "Dictionary: cloning value to '%s'", nv->name);

        void *oldvalue = nv->value;
        void *newvalue = NULL;
        if(value_len) {
            newvalue = mallocz(value_len);
            if(value) memcpy(newvalue, value, value_len);
            else memset(newvalue, 0, value_len);
        }
        nv->value = newvalue;
        nv->value_len = value_len;

        debug(D_DICTIONARY, "Dictionary: freeing old value of '%s'", nv->name);
        freez(oldvalue);
    }

    if(dict->ins_callback)
        dict->ins_callback(nv->name, nv->value, dict->ins_callback_data);
}

static size_t namevalue_destroy_unsafe(DICTIONARY *dict, NAME_VALUE *nv) {
    debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", nv->name);

    if(dict->del_callback)
        dict->del_callback(nv->name, nv->value, dict->del_callback_data);

    size_t freed = 0;

    if(unlikely(!(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE))) {
        debug(D_DICTIONARY, "Dictionary freeing value of '%s'", nv->name);
        freez(nv->value);
        freed += nv->value_len;
    }

    if(unlikely(!(dict->flags & DICTIONARY_FLAG_NAME_LINK_DONT_CLONE))) {
        debug(D_DICTIONARY, "Dictionary freeing name '%s'", nv->name);
        freez(nv->name);
        freed += nv->name_len;
    }

    freez(nv);
    freed += sizeof(NAME_VALUE);

    DICTIONARY_STATS_ENTRIES_MINUS_MEMORY(dict, freed);

    return freed;
}

// ----------------------------------------------------------------------------
// API - dictionary management

DICTIONARY *dictionary_create(DICTIONARY_FLAGS flags) {
    debug(D_DICTIONARY, "Creating dictionary.");

    DICTIONARY *dict = callocz(1, sizeof(DICTIONARY));
    size_t allocated = sizeof(DICTIONARY);

    dict->flags = flags;
    dict->first_item = dict->last_item = NULL;

    allocated += dictionary_lock_init(dict);
    allocated += reference_counter_init(dict);
    dict->memory = allocated;

    hashtable_init_unsafe(dict);
    return (DICTIONARY *)dict;
}

size_t dictionary_destroy(DICTIONARY *dict) {
    if(!dict) return 0;

    debug(D_DICTIONARY, "Destroying dictionary.");

    dictionary_lock_wrlock(dict);

    size_t freed = 0;
    NAME_VALUE *nv = dict->first_item;
    while (nv) {
        // cache nv->next
        // because we are going to free nv
        NAME_VALUE *nvnext = nv->next;
        freed += namevalue_destroy_unsafe(dict, nv);
        nv = nvnext;
        // to speed up destruction, we don't
        // unlink nv from the linked-list here
    }

    dict->first_item = NULL;
    dict->last_item = NULL;

    // destroy the dictionary
    freed += hashtable_destroy_unsafe(dict);

    dictionary_unlock(dict);
    freed += dictionary_lock_free(dict);
    freed += reference_counter_free(dict);

    freez(dict);
    freed += sizeof(DICTIONARY);

    return freed;
}

// ----------------------------------------------------------------------------
// API - items management

void *dictionary_set_unsafe(DICTIONARY *dict, const char *name, void *value, size_t value_len) {
    if(unlikely(!name || !*name)) {
        error("Attempted to dictionary_set() a dictionary item without a name");
        return NULL;
    }

    if(unlikely(!(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)))
        error("DICTIONARY: INTERNAL ERROR: inserting dictionary item '%s' without exclusive access to dictionary", name);

    size_t name_len = strlen(name) + 1; // we need the terminating null too

    debug(D_DICTIONARY, "SET dictionary entry with name '%s'.", name);

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

    NAME_VALUE *nv, **pnv = hashtable_insert_unsafe(dict, name, name_len);
    if(likely(*pnv == 0)) {
        // a new item added to the index
        nv = *pnv = namevalue_create_unsafe(dict, name, name_len, value, value_len);
        hashtable_inserted_name_value_unsafe(dict, name, name_len, nv);
        linkedlist_namevalue_link_unsafe(dict, nv);
    }
    else {
        // the item is already in the index
        // so, either we will return the old one
        // or overwrite the value, depending on dictionary flags

        nv = *pnv;
        if(!(dict->flags & DICTIONARY_FLAG_DONT_OVERWRITE_VALUE))
            namevalue_reset_unsafe(dict, nv, value, value_len);

        else if(dict->conflict_callback)
            dict->conflict_callback(nv->name, nv->value, value, dict->conflict_callback_data);
    }

    return nv->value;
}

void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len) {
    dictionary_lock_wrlock(dict);
    void *ret = dictionary_set_unsafe(dict, name, value, value_len);
    dictionary_unlock(dict);
    return ret;
}

static NAME_VALUE *dictionary_get_name_value_unsafe(DICTIONARY *dict, const char *name) {
    if(unlikely(!name || !*name)) {
        error("Attempted to dictionary_get() without a name");
        return NULL;
    }

    size_t name_len = strlen(name) + 1; // we need the terminating null too

    debug(D_DICTIONARY, "GET dictionary entry with name '%s'.", name);

    NAME_VALUE *nv = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        return NULL;
    }

    debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
    return nv;
}

void *dictionary_get_unsafe(DICTIONARY *dict, const char *name) {
    NAME_VALUE *nv = dictionary_get_name_value_unsafe(dict, name);

    if(unlikely(!nv))
        return NULL;

    return nv->value;
}

void *dictionary_get(DICTIONARY *dict, const char *name) {
    dictionary_lock_rdlock(dict);
    void *ret = dictionary_get_unsafe(dict, name);
    dictionary_unlock(dict);
    return ret;
}

void *dictionary_acquire_item_unsafe(DICTIONARY *dict, const char *name) {
    NAME_VALUE *nv = dictionary_get_name_value_unsafe(dict, name);

    if(unlikely(!nv))
        return NULL;

    reference_counter_acquire(dict, nv);
    return nv;
}

void *dictionary_acquire_item(DICTIONARY *dict, const char *name) {
    dictionary_lock_rdlock(dict);
    void *ret = dictionary_acquire_item_unsafe(dict, name);
    dictionary_unlock(dict);
    return ret;
}

void *dictionary_acquired_item_value(DICTIONARY *dict, void *item) {
    (void)dict;
    if(!item) return NULL;

    NAME_VALUE *nv = (NAME_VALUE *)item;
    return nv->value;
}

void dictionary_acquired_item_release_unsafe(DICTIONARY *dict, void *item) {
    if(!item) return;
    NAME_VALUE *nv = (NAME_VALUE *)item;
    reference_counter_release(dict, nv);
}

void dictionary_acquired_item_release(DICTIONARY *dict, void *item) {
    dictionary_acquired_item_release_unsafe(dict, item);
}

int dictionary_del_unsafe(DICTIONARY *dict, const char *name) {
    if(unlikely(!name || !*name)) {
        error("Attempted to dictionary_det() without a name");
        return -1;
    }

    if(unlikely(!(dict->flags & DICTIONARY_FLAG_EXCLUSIVE_ACCESS)))
        error("DICTIONARY: INTERNAL ERROR: deleting dictionary item '%s' without exclusive access to dictionary", name);

    size_t name_len = strlen(name) + 1; // we need the terminating null too

    debug(D_DICTIONARY, "DEL dictionary entry with name '%s'.", name);

    // Unfortunately, the JudyHSDel() does not return the value of the
    // item that was deleted, so we have to find it before we delete it,
    // since we need to release our structures too.

    int ret;
    NAME_VALUE *nv = hashtable_get_unsafe(dict, name, name_len);
    if(unlikely(!nv)) {
        debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
        ret = -1;
    }
    else {
        debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);

        if(hashtable_delete_unsafe(dict, name, name_len, nv) == 0)
            error("DICTIONARY: INTERNAL ERROR: tried to delete item with name '%s' that is not in the index", name);

        if(!reference_counter_mark_deleted(dict, nv)) {
            linkedlist_namevalue_unlink_unsafe(dict, nv);
            namevalue_destroy_unsafe(dict, nv);
        }
        ret = 0;

        DICTIONARY_STATS_ENTRIES_MINUS1(dict);

    }
    return ret;
}

int dictionary_del(DICTIONARY *dict, const char *name) {
    dictionary_lock_wrlock(dict);
    int ret = dictionary_del_unsafe(dict, name);
    dictionary_unlock(dict);
    return ret;
}

// ----------------------------------------------------------------------------
// traversal with loop

void *dictionary_foreach_start_rw(DICTFE *dfe, DICTIONARY *dict, char rw) {
    if(unlikely(!dfe || !dict)) return NULL;

    dfe->dict = dict;
    dfe->started_ut = now_realtime_usec();

    if(rw == 'r' || rw == 'R')
        dictionary_lock_rdlock(dict);
    else
        dictionary_lock_wrlock(dict);

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    // get the first item from the list
    NAME_VALUE *nv = dict->first_item;

    // skip all the deleted items
    while(nv && (nv->flags & NAME_VALUE_FLAG_DELETED))
        nv = nv->next;

    if(likely(nv)) {
        dfe->last_position_index = nv;
        dfe->name = nv->name;
        dfe->value = nv->value;
        reference_counter_acquire(dict, nv);
    }
    else {
        dfe->last_position_index = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
    }

    return dfe->value;
}

void *dictionary_foreach_next(DICTFE *dfe) {
    if(unlikely(!dfe || !dfe->dict)) return NULL;

    // the item we just did
    NAME_VALUE *nv = (NAME_VALUE *)dfe->last_position_index;

    // get the next item from the list
    NAME_VALUE *nv_next = (nv) ? nv->next : NULL;

    // skip all the deleted items
    while(nv_next && (nv_next->flags & NAME_VALUE_FLAG_DELETED))
        nv_next = nv_next->next;

    // release the old, so that it can possibly be deleted
    if(likely(nv))
        reference_counter_release(dfe->dict, nv);

    if(likely(nv = nv_next)) {
        dfe->last_position_index = nv;
        dfe->name = nv->name;
        dfe->value = nv->value;
        reference_counter_acquire(dfe->dict, nv);
    }
    else {
        dfe->last_position_index = NULL;
        dfe->name = NULL;
        dfe->value = NULL;
    }

    return dfe->value;
}

usec_t dictionary_foreach_done(DICTFE *dfe) {
    if(unlikely(!dfe || !dfe->dict)) return 0;

    // the item we just did
    NAME_VALUE *nv = (NAME_VALUE *)dfe->last_position_index;

    // release it, so that it can possibly be deleted
    if(likely(nv))
        reference_counter_release(dfe->dict, nv);

    dictionary_unlock((DICTIONARY *)dfe->dict);
    dfe->dict = NULL;
    dfe->last_position_index = NULL;
    dfe->name = NULL;
    dfe->value = NULL;

    usec_t usec = now_realtime_usec() - dfe->started_ut;
    dfe->started_ut = 0;

    return usec;
}

// ----------------------------------------------------------------------------
// API - walk through the dictionary
// the dictionary is locked for reading while this happens
// do not use other dictionary calls while walking the dictionary - deadlock!

int dictionary_walkthrough_rw(DICTIONARY *dict, char rw, int (*callback)(const char *name, void *entry, void *data), void *data) {
    if(unlikely(!dict)) return 0;

    if(rw == 'r' || rw == 'R')
        dictionary_lock_rdlock(dict);
    else
        dictionary_lock_wrlock(dict);

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    // written in such a way, that the callback can delete the active element

    int ret = 0;
    NAME_VALUE *nv = dict->first_item, *nv_next;
    while(nv) {

        // skip the deleted items
        if(nv->flags & NAME_VALUE_FLAG_DELETED) {
            nv = nv->next;
            continue;
        }

        // get a reference counter, so that our item will not be deleted
        // while we are using it
        reference_counter_acquire(dict, nv);

        int r = callback(nv->name, nv->value, data);

        // since we have a reference counter, this item cannot be deleted
        // until we release the reference counter, so the pointers are there
        nv_next = nv->next;
        reference_counter_release(dict, nv);

        if(unlikely(r < 0)) {
            ret = r;
            break;
        }

        ret += r;

        nv = nv_next;
    }

    dictionary_unlock(dict);

    return ret;
}

// ----------------------------------------------------------------------------
// sort

static int dictionary_sort_compar(const void *nv1, const void *nv2) {
    return strcmp((*(NAME_VALUE **)nv1)->name, (*(NAME_VALUE **)nv2)->name);
}

int dictionary_sorted_walkthrough_rw(DICTIONARY *dict, char rw, int (*callback)(const char *name, void *entry, void *data), void *data) {
    if(unlikely(!dict || !dict->entries)) return 0;

    if(rw == 'r' || rw == 'R')
        dictionary_lock_rdlock(dict);
    else
        dictionary_lock_wrlock(dict);

    DICTIONARY_STATS_WALKTHROUGHS_PLUS1(dict);

    size_t count = dict->entries;
    NAME_VALUE **array = mallocz(sizeof(NAME_VALUE *) * count);

    size_t i;
    NAME_VALUE *nv;
    for(nv = dict->first_item, i = 0; nv && i < count ;nv = nv->next) {
        if(likely(!(nv->flags & NAME_VALUE_FLAG_DELETED)))
            array[i++] = nv;
    }

    if(unlikely(nv))
        error("DICTIONARY: during sorting expected to have %zu items in dictionary, but there are more. Sorted results may be incomplete. This is internal error - dictionaries fail to maintain an accurate number of the number of entries they have.", count);

    if(unlikely(i != count)) {
        error("DICTIONARY: during sorting expected to have %zu items in dictionary, but there are %zu. Sorted results may be incomplete. This is internal error - dictionaries fail to maintain an accurate number of the number of entries they have.", count, i);
        count = i;
    }

    qsort(array, count, sizeof(NAME_VALUE *), dictionary_sort_compar);

    int ret = 0;
    for(i = 0; i < count ;i++) {
        nv = array[i];
        reference_counter_acquire(dict, nv);
        int r = callback(nv->name, nv->value, data);
        reference_counter_release(dict, nv);
        if(r < 0) { ret = r; break; }
        ret += r;
    }

    dictionary_unlock(dict);
    freez(array);

    return ret;
}

// ----------------------------------------------------------------------------
// unit test

static void dictionary_unittest_free_char_pp(char **pp, size_t entries) {
    for(size_t i = 0; i < entries ;i++)
        freez(pp[i]);

    freez(pp);
}

static char **dictionary_unittest_generate_names(size_t entries) {
    char **names = mallocz(sizeof(char *) * entries);
    for(size_t i = 0; i < entries ;i++) {
        char buf[25 + 1] = "";
        snprintfz(buf, 25, "name.%zu.0123456789.%zu \t !@#$%%^&*(),./[]{}\\|~`", i, entries / 2 + i);
        names[i] = strdupz(buf);
    }
    return names;
}

static char **dictionary_unittest_generate_values(size_t entries) {
    char **values = mallocz(sizeof(char *) * entries);
    for(size_t i = 0; i < entries ;i++) {
        char buf[25 + 1] = "";
        snprintfz(buf, 25, "value-%zu-0987654321.%zu%%^&*(),. \t !@#$/[]{}\\|~`", i, entries / 2 + i);
        values[i] = strdupz(buf);
    }
    return values;
}

static size_t dictionary_unittest_set_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], values[i], vallen);
        if(val == values[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, values[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_set_null(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    size_t errors = 0;
    size_t i = 0;
    for(; i < entries ;i++) {
        void *val = dictionary_set(dict, names[i], NULL, 0);
        if(val != NULL) { fprintf(stderr, ">>> %s() returns a non NULL value\n", __FUNCTION__); errors++; }
    }
    if(dictionary_stats_entries(dict) != i) {
        fprintf(stderr, ">>> %s() dictionary items do not match\n", __FUNCTION__);
        errors++;
    }
    return errors;
}


static size_t dictionary_unittest_set_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], values[i], vallen);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]) + 1;
        char *val = (char *)dictionary_get(dict, names[i]);
        if(val == values[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, values[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        char *val = (char *)dictionary_get(dict, names[i]);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_nonexisting(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        char *val = (char *)dictionary_get(dict, values[i]);
        if(val) { fprintf(stderr, ">>> %s() returns non-existing item\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_del_nonexisting(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        int ret = dictionary_del(dict, values[i]);
        if(ret != -1) { fprintf(stderr, ">>> %s() deleted non-existing item\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_del_existing(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    size_t errors = 0;

    size_t forward_from = 0, forward_to = entries / 3;
    size_t middle_from = forward_to, middle_to = entries * 2 / 3;
    size_t backward_from = middle_to, backward_to = entries;

    for(size_t i = forward_from; i < forward_to ;i++) {
        int ret = dictionary_del(dict, names[i]);
        if(ret == -1) { fprintf(stderr, ">>> %s() didn't delete (forward) existing item\n", __FUNCTION__); errors++; }
    }

    for(size_t i = middle_to - 1; i >= middle_from ;i--) {
        int ret = dictionary_del(dict, names[i]);
        if(ret == -1) { fprintf(stderr, ">>> %s() didn't delete (middle) existing item\n", __FUNCTION__); errors++; }
    }

    for(size_t i = backward_to - 1; i >= backward_from ;i--) {
        int ret = dictionary_del(dict, names[i]);
        if(ret == -1) { fprintf(stderr, ">>> %s() didn't delete (backward) existing item\n", __FUNCTION__); errors++; }
    }

    return errors;
}

static size_t dictionary_unittest_reset_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val == names[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, names[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_reset_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val != names[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
        if(!val)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_reset_dont_overwrite_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]) + 1;
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static int dictionary_unittest_walkthrough_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)value;
    (void)data;
    return 1;
}

static size_t dictionary_unittest_walkthrough(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    int sum = dictionary_walkthrough_read(dict, dictionary_unittest_walkthrough_callback, NULL);
    if(sum < (int)entries) return entries - sum;
    else return sum - entries;
}

static int dictionary_unittest_walkthrough_delete_this_callback(const char *name, void *value, void *data) {
    (void)value;

    if(dictionary_del_having_write_lock((DICTIONARY *)data, name) == -1)
        return 0;

    return 1;
}

static size_t dictionary_unittest_walkthrough_delete_this(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    int sum = dictionary_walkthrough_write(dict, dictionary_unittest_walkthrough_delete_this_callback, dict);
    if(sum < (int)entries) return entries - sum;
    else return sum - entries;
}

static int dictionary_unittest_walkthrough_stop_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)value;
    (void)data;
    return -1;
}

static size_t dictionary_unittest_walkthrough_stop(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    int sum = dictionary_walkthrough_read(dict, dictionary_unittest_walkthrough_stop_callback, NULL);
    if(sum != -1) return 1;
    return 0;
}

static size_t dictionary_unittest_foreach(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t count = 0;
    char *item;
    dfe_start_read(dict, item)
        count++;
    dfe_done(item);

    if(count > entries) return count - entries;
    return entries - count;
}

static size_t dictionary_unittest_foreach_delete_this(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t count = 0;
    char *item;
    dfe_start_write(dict, item)
        if(dictionary_del_having_write_lock(dict, item_name) != -1) count++;
    dfe_done(item);

    if(count > entries) return count - entries;
    return entries - count;
}

static size_t dictionary_unittest_destroy(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t bytes = dictionary_destroy(dict);
    fprintf(stderr, " %s() freed %zu bytes,", __FUNCTION__, bytes);
    return 0;
}

static usec_t dictionary_unittest_run_and_measure_time(DICTIONARY *dict, char *message, char **names, char **values, size_t entries, size_t *errors, size_t (*callback)(DICTIONARY *dict, char **names, char **values, size_t entries)) {
    fprintf(stderr, "%-40s... ", message);

    usec_t started = now_realtime_usec();
    size_t errs = callback(dict, names, values, entries);
    usec_t ended = now_realtime_usec();
    usec_t dt = ended - started;

    if(callback == dictionary_unittest_destroy) dict = NULL;

    fprintf(stderr, " %zu errors, %zu items in dictionary, %llu usec \n", errs, dict? dictionary_stats_entries(dict):0, dt);
    *errors += errs;
    return dt;
}

static void dictionary_unittest_clone(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_clone);
    dictionary_unittest_run_and_measure_time(dict, "getting entries", names, values, entries, errors, dictionary_unittest_get_clone);
    dictionary_unittest_run_and_measure_time(dict, "getting non-existing entries", names, values, entries, errors, dictionary_unittest_get_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "resetting entries", names, values, entries, errors, dictionary_unittest_reset_clone);
    dictionary_unittest_run_and_measure_time(dict, "deleting non-existing entries", names, values, entries, errors, dictionary_unittest_del_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "deleting existing entries", names, values, entries, errors, dictionary_unittest_del_existing);
    dictionary_unittest_run_and_measure_time(dict, "walking through empty", names, values, 0, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach empty", names, values, 0, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, errors, dictionary_unittest_destroy);
}

static void dictionary_unittest_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "getting entries", names, values, entries, errors, dictionary_unittest_get_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "getting non-existing entries", names, values, entries, errors, dictionary_unittest_get_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "resetting entries", names, values, entries, errors, dictionary_unittest_reset_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "deleting non-existing entries", names, values, entries, errors, dictionary_unittest_del_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "deleting existing entries", names, values, entries, errors, dictionary_unittest_del_existing);
    dictionary_unittest_run_and_measure_time(dict, "walking through empty", names, values, 0, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach empty", names, values, 0, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, errors, dictionary_unittest_destroy);
}

struct dictionary_unittest_sorting {
    const char *oldname;
    const char *oldvalue;
    size_t count;
};

static int dictionary_unittest_sorting_callback(const char *name, void *value, void *data) {
    struct dictionary_unittest_sorting *t = (struct dictionary_unittest_sorting *)data;
    const char *v = (const char *)value;

    int ret = 0;
    if(t->oldname && strcmp(t->oldname, name) > 0) {
        fprintf(stderr, "name '%s' should be after '%s'\n", t->oldname, name);
        ret = 1;
    }
    t->count++;
    t->oldname = name;
    t->oldvalue = v;

    return ret;
}

static size_t dictionary_unittest_sorted_walkthrough(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    struct dictionary_unittest_sorting tmp = { .oldname = NULL, .oldvalue = NULL, .count = 0 };
    size_t errors;
    errors = dictionary_sorted_walkthrough_read(dict, dictionary_unittest_sorting_callback, &tmp);

    if(tmp.count != entries) {
        fprintf(stderr, "Expected %zu entries, counted %zu\n", entries, tmp.count);
        errors++;
    }
    return errors;
}

static void dictionary_unittest_sorting(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_clone);
    dictionary_unittest_run_and_measure_time(dict, "sorted walkthrough", names, values, entries, errors, dictionary_unittest_sorted_walkthrough);
}

static void dictionary_unittest_null_dfe(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding null value entries", names, values, entries, errors, dictionary_unittest_set_null);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
}


static int check_dictionary_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)value;
    (void)data;
    return 1;
}

static size_t check_dictionary(DICTIONARY *dict, size_t entries, size_t linked_list_members) {
    size_t errors = 0;

    fprintf(stderr, "dictionary entries %zu, expected %zu... ", dictionary_stats_entries(dict), entries);
    if (dictionary_stats_entries(dict) != entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    size_t ll = 0;
    void *t;
    dfe_start_read(dict, t)
        ll++;
    dfe_done(t);

    fprintf(stderr, "dictionary foreach entries %zu, expected %zu... ", ll, entries);
    if(ll != entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    ll = dictionary_walkthrough_read(dict, check_dictionary_callback, NULL);
    fprintf(stderr, "dictionary walkthrough entries %zu, expected %zu... ", ll, entries);
    if(ll != entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    ll = dictionary_sorted_walkthrough_read(dict, check_dictionary_callback, NULL);
    fprintf(stderr, "dictionary sorted walkthrough entries %zu, expected %zu... ", ll, entries);
    if(ll != entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    NAME_VALUE *nv;
    for(ll = 0, nv = dict->first_item; nv ;nv = nv->next)
        ll++;

    fprintf(stderr, "dictionary linked list entries %zu, expected %zu... ", ll, linked_list_members);
    if(ll != linked_list_members) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    return errors;
}

static int check_name_value_callback(const char *name, void *value, void *data) {
    (void)name;
    return value == data;
}

static size_t check_name_value(DICTIONARY *dict, NAME_VALUE *nv, const char *name, const char *value, int refcount, NAME_VALUE_FLAGS flags, bool searchable, bool browsable, bool linked) {
    size_t errors = 0;

    fprintf(stderr, "NAME_VALUE name is '%s', expected '%s'... ", nv->name, name);
    if(strcmp(nv->name, name) != 0) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "NAME_VALUE value is '%s', expected '%s'... ", (const char *)nv->value, value);
    if(strcmp((const char *)nv->value, value) != 0) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "NAME_VALUE refcount is %d, expected %d... ", nv->refcount, refcount);
    if (nv->refcount != refcount) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "NAME_VALUE flags is %u, expected %u... ", nv->flags, flags);
    if (nv->flags != flags) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    void *v = dictionary_get(dict, name);
    bool found = v == nv->value;
    fprintf(stderr, "NAME_VALUE %s searchable, expected %s... ", found?"is":"is not", searchable?"is":"is not");
    if(found != searchable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = false;
    void *t;
    dfe_start_read(dict, t) {
        if(t == nv->value) found = true;
    }
    dfe_done(t);

    fprintf(stderr, "NAME_VALUE %s dfe browsable, expected %s... ", found?"is":"is not", browsable?"is":"is not");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = dictionary_walkthrough_read(dict, check_name_value_callback, nv->value);
    fprintf(stderr, "NAME_VALUE %s walkthrough browsable, expected %s... ", found?"is":"is not", browsable?"is":"is not");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = dictionary_sorted_walkthrough_read(dict, check_name_value_callback, nv->value);
    fprintf(stderr, "NAME_VALUE %s sorted walkthrough browsable, expected %s... ", found?"is":"is not", browsable?"is":"is not");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = false;
    NAME_VALUE *n;
    for(n = dict->first_item; n ;n = n->next)
        if(n == nv) found = true;

    fprintf(stderr, "NAME_VALUE %s linked, expected %s... ", found?"is":"is not", linked?"is":"is not");
    if(found != linked) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    return errors;
}

int dictionary_unittest(size_t entries) {
    if(entries < 10) entries = 10;

    DICTIONARY *dict;
    size_t errors = 0;

    fprintf(stderr, "Generating %zu names and values...\n", entries);
    char **names = dictionary_unittest_generate_names(entries);
    char **values = dictionary_unittest_generate_values(entries);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    dictionary_unittest_clone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary multi threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_NONE);
    dictionary_unittest_clone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary single threaded, non-clone, add-in-front options, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED|DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_ADD_IN_FRONT);
    dictionary_unittest_nonclone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary multi threaded, non-clone, add-in-front options, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_ADD_IN_FRONT);
    dictionary_unittest_nonclone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary single-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED|DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "resetting non-overwrite entries", names, values, entries, &errors, dictionary_unittest_reset_dont_overwrite_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, &errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, &errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, &errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary multi-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough write delete this", names, values, entries, &errors, dictionary_unittest_walkthrough_delete_this);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary multi-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "foreach write delete this", names, values, entries, &errors, dictionary_unittest_foreach_delete_this);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop empty", names, values, 0, &errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback empty", names, values, 0, &errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    dictionary_unittest_sorting(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    dictionary_unittest_null_dfe(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, noclone, %zu items\n", entries);
    dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE);
    dictionary_unittest_null_dfe(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    // check reference counters
    {
        fprintf(stderr, "\nTesting reference counters:\n");
        dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
        errors += check_dictionary(dict, 0, 0);

        fprintf(stderr, "\nAdding test item to dictionary and acquiring it\n");
        dictionary_set(dict, "test", "ITEM1", 6);
        NAME_VALUE *nv = dictionary_acquire_item(dict, "test");

        errors += check_dictionary(dict, 1, 1);
        errors += check_name_value(dict, nv, "test", "ITEM1", 1, NAME_VALUE_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nChecking that reference counters are increased:\n");
        void *t;
        dfe_start_read(dict, t) {
            errors += check_dictionary(dict, 1, 1);
            errors += check_name_value(dict, nv, "test", "ITEM1", 2, NAME_VALUE_FLAG_NONE, true, true, true);
        }
        dfe_done(t);

        fprintf(stderr, "\nChecking that reference counters are decreased:\n");
        errors += check_dictionary(dict, 1, 1);
        errors += check_name_value(dict, nv, "test", "ITEM1", 1, NAME_VALUE_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDeleting the item we have acquired:\n");
        dictionary_del(dict, "test");

        errors += check_dictionary(dict, 0, 1);
        errors += check_name_value(dict, nv, "test", "ITEM1", 1, NAME_VALUE_FLAG_DELETED, false, false, true);

        fprintf(stderr, "\nAdding another item with the same name of the item we deleted, while being acquired:\n");
        dictionary_set(dict, "test", "ITEM2", 6);
        errors += check_dictionary(dict, 1, 2);

        fprintf(stderr, "\nAcquiring the second item:\n");
        NAME_VALUE *nv2 = dictionary_acquire_item(dict, "test");
        errors += check_name_value(dict, nv, "test",  "ITEM1", 1, NAME_VALUE_FLAG_DELETED, false, false, true);
        errors += check_name_value(dict, nv2, "test", "ITEM2", 1, NAME_VALUE_FLAG_NONE,    true,  true,  true);

        fprintf(stderr, "\nReleasing the second item (the first is still acquired):\n");
        dictionary_acquired_item_release(dict, nv2);
        errors += check_dictionary(dict, 1, 2);
        errors += check_name_value(dict, nv, "test",  "ITEM1", 1, NAME_VALUE_FLAG_DELETED, false, false, true);
        errors += check_name_value(dict, nv2, "test", "ITEM2", 0, NAME_VALUE_FLAG_NONE,    true,  true,  true);

        fprintf(stderr, "\nDeleting the second item (the first is still acquired):\n");
        dictionary_del(dict, "test");
        errors += check_dictionary(dict, 0, 1);
        errors += check_name_value(dict, nv, "test",  "ITEM1", 1, NAME_VALUE_FLAG_DELETED, false, false, true);

        fprintf(stderr, "\nReleasing the first item (which we have already deleted):\n");
        dictionary_acquired_item_release(dict, nv);
        errors += check_dictionary(dict, 0, 0);

        dictionary_destroy(dict);
    }

    dictionary_unittest_free_char_pp(names, entries);
    dictionary_unittest_free_char_pp(values, entries);

    fprintf(stderr, "\n%zu errors found\n", errors);
    return (int)errors;
}
