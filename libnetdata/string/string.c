// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include <Judy.h>

typedef int32_t REFCOUNT;

// ----------------------------------------------------------------------------
// STRING implementation - dedup all STRING

struct netdata_string {
    uint32_t length;    // the string length including the terminating '\0'

    REFCOUNT refcount;  // how many times this string is used
                        // We use a signed number to be able to detect duplicate frees of a string.
                        // If at any point this goes below zero, we have a duplicate free.

    const char str[];   // the string itself, is appended to this structure
};

static struct string_hashtable {
    Pvoid_t JudyHSArray;        // the Judy array - hashtable
    netdata_rwlock_t rwlock;    // the R/W lock to protect the Judy array

    long int entries;           // the number of entries in the index
    long int active_references; // the number of active references alive
    long int memory;            // the memory used, without the JudyHS index

    size_t inserts;             // the number of successful inserts to the index
    size_t deletes;             // the number of successful deleted from the index
    size_t searches;            // the number of successful searches in the index
    size_t duplications;        // when a string is referenced
    size_t releases;            // when a string is unreferenced

#ifdef NETDATA_INTERNAL_CHECKS
    // internal statistics
    size_t found_deleted_on_search;
    size_t found_available_on_search;
    size_t found_deleted_on_insert;
    size_t found_available_on_insert;
    size_t spins;
#endif

} string_base = {
    .JudyHSArray = NULL,
    .rwlock = NETDATA_RWLOCK_INITIALIZER,
};

#ifdef NETDATA_INTERNAL_CHECKS
#define string_internal_stats_add(var, val) __atomic_add_fetch(&string_base.var, val, __ATOMIC_RELAXED)
#else
#define string_internal_stats_add(var, val) do {;} while(0)
#endif

#define string_stats_atomic_increment(var) __atomic_add_fetch(&string_base.var, 1, __ATOMIC_RELAXED)
#define string_stats_atomic_decrement(var) __atomic_sub_fetch(&string_base.var, 1, __ATOMIC_RELAXED)

void string_get_statistics(struct string_statistics *stats) {
    stats->inserts = string_base.inserts;
    stats->deletes = string_base.deletes;
    stats->searches = string_base.searches;
    stats->entries = (size_t) string_base.entries;
    stats->references = (size_t) string_base.active_references;
    stats->memory = (size_t) string_base.memory;
    stats->duplications = string_base.duplications;
    stats->releases = string_base.releases;
}

#define string_entry_acquire(se) __atomic_add_fetch(&((se)->refcount), 1, __ATOMIC_SEQ_CST);
#define string_entry_release(se) __atomic_sub_fetch(&((se)->refcount), 1, __ATOMIC_SEQ_CST);

static inline bool string_entry_check_and_acquire(STRING *se) {
    REFCOUNT expected, desired, count = 0;
    do {
        count++;

        expected = __atomic_load_n(&se->refcount, __ATOMIC_SEQ_CST);

        if(expected <= 0) {
            // We cannot use this.
            // The reference counter reached value zero,
            // so another thread is deleting this.
            string_internal_stats_add(spins, count - 1);
            return false;
        }

        desired = expected + 1;
    }
    while(!__atomic_compare_exchange_n(&se->refcount, &expected, desired, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST));

    string_internal_stats_add(spins, count - 1);

    // statistics
    // string_base.active_references is altered at the in string_strdupz() and string_freez()
    string_stats_atomic_increment(duplications);

    return true;
}

STRING *string_dup(STRING *string) {
    if(unlikely(!string)) return NULL;

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(__atomic_load_n(&string->refcount, __ATOMIC_SEQ_CST) <= 0))
        fatal("STRING: tried to %s() a string that is freed (it has %d references).", __FUNCTION__, string->refcount);
#endif

    string_entry_acquire(string);

    // statistics
    string_stats_atomic_increment(active_references);
    string_stats_atomic_increment(duplications);

    return string;
}

// Search the index and return an ACQUIRED string entry, or NULL
static inline STRING *string_index_search(const char *str, size_t length) {
    STRING *string;

    // Find the string in the index
    // With a read-lock so that multiple readers can use the index concurrently.

    netdata_rwlock_rdlock(&string_base.rwlock);

    Pvoid_t *Rc;
    Rc = JudyHSGet(string_base.JudyHSArray, (void *)str, length);
    if(likely(Rc)) {
        // found in the hash table
        string = *Rc;

        if(string_entry_check_and_acquire(string)) {
            // we can use this entry
            string_internal_stats_add(found_available_on_search, 1);
        }
        else {
            // this entry is about to be deleted by another thread
            // do not touch it, let it go...
            string = NULL;
            string_internal_stats_add(found_deleted_on_search, 1);
        }
    }
    else {
        // not found in the hash table
        string = NULL;
    }

    string_stats_atomic_increment(searches);
    netdata_rwlock_unlock(&string_base.rwlock);

    return string;
}

// Insert a string to the index and return an ACQUIRED string entry,
// or NULL if the call needs to be retried (a deleted entry with the same key is still in the index)
// The returned entry is ACQUIRED, and it can either be:
//   1. a new item inserted, or
//   2. an item found in the index that is not currently deleted
static inline STRING *string_index_insert(const char *str, size_t length) {
    STRING *string;

    netdata_rwlock_wrlock(&string_base.rwlock);

    STRING **ptr;
    {
        JError_t J_Error;
        Pvoid_t *Rc = JudyHSIns(&string_base.JudyHSArray, (void *)str, length, &J_Error);
        if (unlikely(Rc == PJERR)) {
            fatal(
                "STRING: Cannot insert entry with name '%s' to JudyHS, JU_ERRNO_* == %u, ID == %d",
                str,
                JU_ERRNO(&J_Error),
                JU_ERRID(&J_Error));
        }
        ptr = (STRING **)Rc;
    }

    if (likely(*ptr == 0)) {
        // a new item added to the index
        size_t mem_size = sizeof(STRING) + length;
        string = mallocz(mem_size);
        strcpy((char *)string->str, str);
        string->length = length;
        string->refcount = 1;
        *ptr = string;
        string_base.inserts++;
        string_base.entries++;
        string_base.memory += (long)mem_size;
    }
    else {
        // the item is already in the index
        string = *ptr;

        if(string_entry_check_and_acquire(string)) {
            // we can use this entry
            string_internal_stats_add(found_available_on_insert, 1);
        }
        else {
            // this entry is about to be deleted by another thread
            // do not touch it, let it go...
            string = NULL;
            string_internal_stats_add(found_deleted_on_insert, 1);
        }

        string_stats_atomic_increment(searches);
    }

    netdata_rwlock_unlock(&string_base.rwlock);
    return string;
}

// delete an entry from the index
static inline void string_index_delete(STRING *string) {
    netdata_rwlock_wrlock(&string_base.rwlock);

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(__atomic_load_n(&string->refcount, __ATOMIC_SEQ_CST) != 0))
        fatal("STRING: tried to delete a string at %s() that is already freed (it has %d references).", __FUNCTION__, string->refcount);
#endif

    bool deleted = false;

    if (likely(string_base.JudyHSArray)) {
        JError_t J_Error;
        int ret = JudyHSDel(&string_base.JudyHSArray, (void *)string->str, string->length, &J_Error);
        if (unlikely(ret == JERR)) {
            error(
                "STRING: Cannot delete entry with name '%s' from JudyHS, JU_ERRNO_* == %u, ID == %d",
                string->str,
                JU_ERRNO(&J_Error),
                JU_ERRID(&J_Error));
        } else
            deleted = true;
    }

    if (unlikely(!deleted))
        error("STRING: tried to delete '%s' that is not in the index. Ignoring it.", string->str);
    else {
        size_t mem_size = sizeof(STRING) + string->length;
        string_base.deletes++;
        string_base.entries--;
        string_base.memory -= (long)mem_size;
        freez(string);
    }

    netdata_rwlock_unlock(&string_base.rwlock);
}

STRING *string_strdupz(const char *str) {
    if(unlikely(!str || !*str)) return NULL;

    size_t length = strlen(str) + 1;
    STRING *string = string_index_search(str, length);

    while(!string) {
        // The search above did not find anything,
        // We loop here, because during insert we may find an entry that is being deleted by another thread.
        // So, we have to let it go and retry to insert it again.

        string = string_index_insert(str, length);
    }

    // statistics
    string_stats_atomic_increment(active_references);

    return string;
}

void string_freez(STRING *string) {
    if(unlikely(!string)) return;

    REFCOUNT refcount = string_entry_release(string);

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(refcount < 0))
        fatal("STRING: tried to %s() a string that is already freed (it has %d references).", __FUNCTION__, string->refcount);
#endif

    if(unlikely(refcount == 0))
        string_index_delete(string);

    // statistics
    string_stats_atomic_decrement(active_references);
    string_stats_atomic_increment(releases);
}

size_t string_strlen(STRING *string) {
    if(unlikely(!string)) return 0;
    return string->length - 1;
}

const char *string2str(STRING *string) {
    if(unlikely(!string)) return "";
    return string->str;
}

STRING *string_2way_merge(STRING *a, STRING *b) {
    static STRING *X = NULL;

    if(unlikely(!X)) {
        X = string_strdupz("[x]");
    }

    if(unlikely(a == b)) return string_dup(a);
    if(unlikely(a == X)) return string_dup(a);
    if(unlikely(b == X)) return string_dup(b);
    if(unlikely(!a)) return string_dup(X);
    if(unlikely(!b)) return string_dup(X);

    size_t alen = string_strlen(a);
    size_t blen = string_strlen(b);
    size_t length = alen + blen + string_strlen(X) + 1;
    char buf1[length + 1], buf2[length + 1], *dst1;
    const char *s1, *s2;

    s1 = string2str(a);
    s2 = string2str(b);
    dst1 = buf1;
    for( ; *s1 && *s2 && *s1 == *s2 ;s1++, s2++)
        *dst1++ = *s1;

    *dst1 = '\0';

    if(*s1 != '\0' || *s2 != '\0') {
        *dst1++ = '[';
        *dst1++ = 'x';
        *dst1++ = ']';

        s1 = &(string2str(a))[alen - 1];
        s2 = &(string2str(b))[blen - 1];
        char *dst2 = &buf2[length];
        *dst2 = '\0';
        for (; *s1 && *s2 && *s1 == *s2; s1--, s2--)
            *(--dst2) = *s1;

        strcpy(dst1, dst2);
    }

    return string_strdupz(buf1);
}
