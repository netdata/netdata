// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include <Judy.h>

// ----------------------------------------------------------------------------
// STRING implementation - dedup all STRING

#define STRING_PARTITION_SHIFTS (0)
#define STRING_PARTITIONS (256 >> STRING_PARTITION_SHIFTS)
#define string_partition_str(str) ((uint8_t)((str)[0]) >> STRING_PARTITION_SHIFTS)
#define string_partition(string) (string_partition_str((string)->str))

struct netdata_string {
    uint32_t length;    // the string length including the terminating '\0'

    REFCOUNT refcount;  // how many times this string is used
                        // We use a signed number to be able to detect duplicate frees of a string.
                        // If at any point this goes below zero, we have a duplicate free.

    const char str[];   // the string itself, is appended to this structure
};

static struct string_partition {
    RW_SPINLOCK spinlock;       // the R/W spinlock to protect the Judy array

    Pvoid_t JudyHSArray;        // the Judy array - hashtable

    size_t inserts;             // the number of successful inserts to the index
    size_t deletes;             // the number of successful deleted from the index

    long int entries;           // the number of entries in the index
    long int memory;            // the memory used
    long int memory_index;      // JudyHS (accurate)

#ifdef NETDATA_INTERNAL_CHECKS
    // internal statistics

    struct {
        size_t searches;            // the number of successful searches in the index
        size_t releases;            // when a string is unreferenced
        size_t duplications;        // when a string is referenced
        long int active_references; // the number of active references alive
    } atomic;

    size_t found_deleted_on_search;
    size_t found_available_on_search;
    size_t found_deleted_on_insert;
    size_t found_available_on_insert;
    size_t spins;
#endif

} string_base[STRING_PARTITIONS] = { 0 };

#ifdef NETDATA_INTERNAL_CHECKS
#define string_stats_atomic_increment(partition, var) __atomic_add_fetch(&string_base[partition].atomic.var, 1, __ATOMIC_RELAXED)
#define string_stats_atomic_decrement(partition, var) __atomic_sub_fetch(&string_base[partition].atomic.var, 1, __ATOMIC_RELAXED)
#define string_internal_stats_add(partition, var, val) __atomic_add_fetch(&string_base[partition].var, val, __ATOMIC_RELAXED)
#else
#define string_stats_atomic_increment(partition, var) do {;} while(0)
#define string_stats_atomic_decrement(partition, var) do {;} while(0)
#define string_internal_stats_add(partition, var, val) do {;} while(0)
#endif

void string_statistics(size_t *inserts, size_t *deletes, size_t *searches, size_t *entries, size_t *references, size_t *memory, size_t *memory_index, size_t *duplications, size_t *releases) {
    if (inserts) *inserts = 0;
    if (deletes) *deletes = 0;
    if (searches) *searches = 0;
    if (entries) *entries = 0;
    if (references) *references = 0;
    if (memory) *memory = 0;
    if (memory_index) *memory_index = 0;
    if (duplications) *duplications = 0;
    if (releases) *releases = 0;

    for(size_t i = 0; i < STRING_PARTITIONS ;i++) {
        if (inserts)        *inserts        += string_base[i].inserts;
        if (deletes)        *deletes        += string_base[i].deletes;
        if (entries)        *entries        += (size_t) string_base[i].entries;
        if (memory)         *memory         += (size_t) string_base[i].memory;
        if (memory_index)   *memory_index   += (string_base[i].memory_index > 0) ? string_base[i].memory_index : 0;

#ifdef NETDATA_INTERNAL_CHECKS
        if (searches)       *searches       += string_base[i].atomic.searches;
        if (references)     *references     += (size_t) string_base[i].atomic.active_references;
        if (duplications)   *duplications   += string_base[i].atomic.duplications;
        if (releases)       *releases       += string_base[i].atomic.releases;
#endif
    }
}

static inline bool string_entry_check_and_acquire(STRING *se) {
#ifdef NETDATA_INTERNAL_CHECKS
    uint8_t partition = string_partition(se);
#endif

    if(!refcount_acquire(&se->refcount))
        return false;

    // statistics
    // string_base.active_references is altered at the in string_strdupz() and string_freez()
    string_stats_atomic_increment(partition, duplications);

    return true;
}

STRING *string_dup(STRING *string) {
    if(unlikely(!string)) return NULL;

    if(!refcount_acquire(&string->refcount))
        fatal("STRING: tried to %s() a string that is deleted (refcount %d).", __FUNCTION__, string->refcount);

#ifdef NETDATA_INTERNAL_CHECKS
    uint8_t partition = string_partition(string);
#endif

    // statistics
    string_stats_atomic_increment(partition, active_references);
    string_stats_atomic_increment(partition, duplications);

    return string;
}

// Search the index and return an ACQUIRED string entry, or NULL
static inline STRING *string_index_search(const char *str, size_t length) {
    STRING *string;

    uint8_t partition = string_partition_str(str);

    // Find the string in the index
    // With a read-lock so that multiple readers can use the index concurrently.

    rw_spinlock_read_lock(&string_base[partition].spinlock);

    Pvoid_t *Rc;
    Rc = JudyHSGet(string_base[partition].JudyHSArray, (void *)str, length - 1);
    if(likely(Rc)) {
        // found in the hash table
        string = *Rc;

        if(string_entry_check_and_acquire(string)) {
            // we can use this entry
            string_internal_stats_add(partition, found_available_on_search, 1);
        }
        else {
            // this entry is about to be deleted by another thread
            // do not touch it, let it go...
            string = NULL;
            string_internal_stats_add(partition, found_deleted_on_search, 1);
        }
    }
    else {
        // not found in the hash table
        string = NULL;
    }

    string_stats_atomic_increment(partition, searches);
    rw_spinlock_read_unlock(&string_base[partition].spinlock);

    return string;
}

// Insert a string to the index and return an ACQUIRED string entry,
// or NULL if the call needs to be retried (a deleted entry with the same key is still in the index)
// The returned entry is ACQUIRED, and it can either be:
//   1. a new item inserted, or
//   2. an item found in the index that is not currently deleted
static inline STRING *string_index_insert(const char *str, size_t length) {
    STRING *string;

    uint8_t partition = string_partition_str(str);

    rw_spinlock_write_lock(&string_base[partition].spinlock);

    int64_t judy_mem = 0;

    STRING **ptr;
    {
        JError_t J_Error;

        JudyAllocThreadPulseReset();

        Pvoid_t *Rc = JudyHSIns(&string_base[partition].JudyHSArray, (void *)str, length - 1, &J_Error);

        judy_mem = JudyAllocThreadPulseGetAndReset();

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
        long mem_size = (long)sizeof(STRING) + (long)length;
        string = mallocz(mem_size);
        strcpy((char *)string->str, str);
        string->length = length;
        string->refcount = 1;
        *ptr = string;
        string_base[partition].inserts++;
        string_base[partition].entries++;
        string_base[partition].memory += mem_size;
        string_base[partition].memory_index += judy_mem;
    }
    else {
        // the item is already in the index
        string = *ptr;

        if(string_entry_check_and_acquire(string)) {
            // we can use this entry
            string_internal_stats_add(partition, found_available_on_insert, 1);
        }
        else {
            // this entry is about to be deleted by another thread
            // do not touch it, let it go...
            string = NULL;
            string_internal_stats_add(partition, found_deleted_on_insert, 1);
        }

        string_stats_atomic_increment(partition, searches);
    }

    rw_spinlock_write_unlock(&string_base[partition].spinlock);
    return string;
}

// delete an entry from the index
static inline void string_index_delete(STRING *string) {
    uint8_t partition = string_partition(string);

    rw_spinlock_write_lock(&string_base[partition].spinlock);

    bool deleted = false;
    int64_t judy_mem = 0;

    if (likely(string_base[partition].JudyHSArray)) {
        JError_t J_Error;

        JudyAllocThreadPulseReset();

        int ret = JudyHSDel(&string_base[partition].JudyHSArray, (void *)string->str, string->length - 1, &J_Error);

        judy_mem = JudyAllocThreadPulseGetAndReset();

        if (unlikely(ret == JERR)) {
            netdata_log_error(
                "STRING: Cannot delete entry with name '%s' from JudyHS, JU_ERRNO_* == %u, ID == %d",
                string->str,
                JU_ERRNO(&J_Error),
                JU_ERRID(&J_Error));
        } else
            deleted = true;
    }

    if (unlikely(!deleted))
        netdata_log_error("STRING: tried to delete '%s' that is not in the index. Ignoring it.", string->str);
    else {
        long mem_size = (long)sizeof(STRING) + (long)string->length;
        string_base[partition].deletes++;
        string_base[partition].entries--;
        string_base[partition].memory -= mem_size;
        string_base[partition].memory_index += judy_mem;
        freez(string);
    }

    rw_spinlock_write_unlock(&string_base[partition].spinlock);
}

STRING *string_strdupz(const char *str) {
    if(unlikely(!str || !*str)) return NULL;

#ifdef NETDATA_INTERNAL_CHECKS
    uint8_t partition = string_partition_str(str);
#endif

    size_t length = strlen(str) + 1;
    STRING *string = string_index_search(str, length);

    while(!string) {
        // The search above did not find anything,
        // We loop here, because during insert we may find an entry that is being deleted by another thread.
        // So, we have to let it go and retry to insert it again.

        string = string_index_insert(str, length);
    }

    // statistics
    string_stats_atomic_increment(partition, active_references);

    return string;
}

STRING *string_strndupz(const char *str, size_t len) {
    if(unlikely(!str || !*str || !len)) return NULL;

#ifdef NETDATA_INTERNAL_CHECKS
    uint8_t partition = string_partition_str(str);
#endif

    char buf[len + 1];
    memcpy(buf, str, len);
    buf[len] = '\0';

    STRING *string = string_index_search(buf, len + 1);
    while(!string)
        string = string_index_insert(buf, len + 1);

    string_stats_atomic_increment(partition, active_references);
    return string;
}

void string_freez(STRING *string) {
    if(unlikely(!string)) return;

#ifdef NETDATA_INTERNAL_CHECKS
    uint8_t partition = string_partition(string);
#endif

    if(unlikely(refcount_release_and_acquire_for_deletion(&string->refcount)))
        string_index_delete(string);

    // statistics
    string_stats_atomic_decrement(partition, active_references);
    string_stats_atomic_increment(partition, releases);
}

inline size_t string_strlen(const STRING *string) {
    if(unlikely(!string)) return 0;
    return string->length - 1;
}

inline const char *string2str(const STRING *string) {
    if(unlikely(!string)) return "";
    return string->str;
}

bool string_ends_with_string(const STRING *whole, const STRING *end) {
    if(whole == end) return true;
    if(!whole || !end) return false;
    if(end->length > whole->length) return false;
    if(end->length == whole->length) return strcmp(string2str(whole), string2str(end)) == 0;
    const char *we = string2str(whole);
    we = &we[string_strlen(whole) - string_strlen(end)];
    return strncmp(we, end->str, string_strlen(end)) == 0;
}

bool string_starts_with_string(const STRING *whole, const STRING *end) {
    if(whole == end) return true;
    if(!whole || !end) return false;
    if(end->length > whole->length) return false;
    if(end->length == whole->length) return strcmp(string2str(whole), string2str(end)) == 0;
    return strncmp(string2str(whole), string2str(end), string_strlen(end)) == 0;
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

// ----------------------------------------------------------------------------
// STRING unit test

struct thread_unittest {
    int join;
    int dups;
};

static void *string_thread(void *arg) {
    struct thread_unittest *tu = arg;

    for(; 1 ;) {
        if(__atomic_load_n(&tu->join, __ATOMIC_RELAXED))
            break;

        STRING *s = string_strdupz("string thread checking 1234567890");

        for(int i = 0; i < tu->dups ; i++)
            string_dup(s);

        for(int i = 0; i < tu->dups ; i++)
            string_freez(s);

        string_freez(s);
    }

    return arg;
}

static char **string_unittest_generate_names(size_t entries) {
    char **names = mallocz(sizeof(char *) * entries);
    for(size_t i = 0; i < entries ;i++) {
        char buf[25 + 1] = "";
        snprintfz(buf, sizeof(buf) - 1, "name.%zu.0123456789.%zu \t !@#$%%^&*(),./[]{}\\|~`", i, entries / 2 + i);
        names[i] = strdupz(buf);
    }
    return names;
}

static void string_unittest_free_char_pp(char **pp, size_t entries) {
    for(size_t i = 0; i < entries ;i++)
        freez(pp[i]);

    freez(pp);
}

static long unittest_string_entries(void) {
    long entries = 0;
    for(size_t p = 0; p < STRING_PARTITIONS ;p++)
        entries += string_base[p].entries;

    return entries;
}

#ifdef NETDATA_INTERNAL_CHECKS

static size_t unittest_string_found_deleted_on_search(void) {
    size_t entries = 0;
    for(size_t p = 0; p < STRING_PARTITIONS ;p++)
        entries += string_base[p].found_deleted_on_search;

    return entries;
}
static size_t unittest_string_found_available_on_search(void) {
    size_t entries = 0;
    for(size_t p = 0; p < STRING_PARTITIONS ;p++)
        entries += string_base[p].found_available_on_search;

    return entries;
}
static size_t unittest_string_found_deleted_on_insert(void) {
    size_t entries = 0;
    for(size_t p = 0; p < STRING_PARTITIONS ;p++)
        entries += string_base[p].found_deleted_on_insert;

    return entries;
}
static size_t unittest_string_found_available_on_insert(void) {
    size_t entries = 0;
    for(size_t p = 0; p < STRING_PARTITIONS ;p++)
        entries += string_base[p].found_available_on_insert;

    return entries;
}
static size_t unittest_string_spins(void) {
    size_t entries = 0;
    for(size_t p = 0; p < STRING_PARTITIONS ;p++)
        entries += string_base[p].spins;

    return entries;
}

#endif // NETDATA_INTERNAL_CHECKS

int string_unittest(size_t entries) {
    size_t errors = 0;

    fprintf(stderr, "Generating %zu names and values...\n", entries);
    char **names = string_unittest_generate_names(entries);

    // check string
    {
        long entries_starting = unittest_string_entries();

        fprintf(stderr, "\nChecking strings...\n");

        STRING *s1 = string_strdupz("hello unittest");
        STRING *s2 = string_strdupz("hello unittest");
        if(s1 != s2) {
            errors++;
            fprintf(stderr, "ERROR: duplicating strings are not deduplicated\n");
        }
        else
            fprintf(stderr, "OK: duplicating string are deduplicated\n");

        STRING *s3 = string_dup(s1);
        if(s3 != s1) {
            errors++;
            fprintf(stderr, "ERROR: cloning strings are not deduplicated\n");
        }
        else
            fprintf(stderr, "OK: cloning string are deduplicated\n");

        if(s1->refcount != 3) {
            errors++;
            fprintf(stderr, "ERROR: string refcount is not 3\n");
        }
        else
            fprintf(stderr, "OK: string refcount is 3\n");

        STRING *s4 = string_strdupz("world unittest");
        if(s4 == s1) {
            errors++;
            fprintf(stderr, "ERROR: string is sharing pointers on different strings\n");
        }
        else
            fprintf(stderr, "OK: string is properly handling different strings\n");

        usec_t start_ut, end_ut;
        STRING **strings = mallocz(entries * sizeof(STRING *));

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            strings[i] = string_strdupz(names[i]);
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Created %zu strings in %"PRIu64" usecs\n", entries, end_ut - start_ut);

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            strings[i] = string_dup(strings[i]);
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Cloned %zu strings in %"PRIu64" usecs\n", entries, end_ut - start_ut);

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            strings[i] = string_strdupz(string2str(strings[i]));
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Found %zu existing strings in %"PRIu64" usecs\n", entries, end_ut - start_ut);

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            string_freez(strings[i]);
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Released %zu referenced strings in %"PRIu64" usecs\n", entries, end_ut - start_ut);

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            string_freez(strings[i]);
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Released (again) %zu referenced strings in %"PRIu64" usecs\n", entries, end_ut - start_ut);

        start_ut = now_realtime_usec();
        for(size_t i = 0; i < entries ;i++) {
            string_freez(strings[i]);
        }
        end_ut = now_realtime_usec();
        fprintf(stderr, "Freed %zu strings in %"PRIu64" usecs\n", entries, end_ut - start_ut);

        freez(strings);

        if(unittest_string_entries() != entries_starting + 2) {
            errors++;
            fprintf(stderr, "ERROR: strings dictionary should have %ld items but it has %ld\n",
                    entries_starting + 2, unittest_string_entries());
        }
        else
            fprintf(stderr, "OK: strings dictionary has 2 items\n");
    }

    // check 2-way merge
    {
        struct testcase {
            char *src1; char *src2; char *expected;
        } tests[] = {
            { "", "", ""},
            { "a", "", "[x]"},
            { "", "a", "[x]"},
            { "a", "a", "a"},
            { "abcd", "abcd", "abcd"},
            { "foo_cs", "bar_cs", "[x]_cs"},
            { "cp_UNIQUE_INFIX_cs", "cp_unique_infix_cs", "cp_[x]_cs"},
            { "cp_UNIQUE_INFIX_ci_unique_infix_cs", "cp_unique_infix_ci_UNIQUE_INFIX_cs", "cp_[x]_cs"},
            { "foo[1234]", "foo[4321]", "foo[[x]]"},
            { NULL, NULL, NULL },
        };

        for (struct testcase *tc = &tests[0]; tc->expected != NULL; tc++) {
            STRING *src1 = string_strdupz(tc->src1);
            STRING *src2 = string_strdupz(tc->src2);
            STRING *expected = string_strdupz(tc->expected);

            STRING *result = string_2way_merge(src1, src2);
            if (string_cmp(result, expected) != 0) {
                fprintf(stderr, "string_2way_merge(\"%s\", \"%s\") -> \"%s\" (expected=\"%s\")\n",
                        string2str(src1),
                        string2str(src2),
                        string2str(result),
                        string2str(expected));
                errors++;
            }

            string_freez(src1);
            string_freez(src2);
            string_freez(expected);
            string_freez(result);
        }
    }

    // threads testing of string
    {
        struct thread_unittest tu = {
            .dups = 1,
            .join = 0,
        };

#ifdef NETDATA_INTERNAL_CHECKS
        size_t ofound_deleted_on_search = unittest_string_found_deleted_on_search(),
               ofound_available_on_search = unittest_string_found_available_on_search(),
               ofound_deleted_on_insert = unittest_string_found_deleted_on_insert(),
               ofound_available_on_insert = unittest_string_found_available_on_insert(),
               ospins = unittest_string_spins();
#endif

        size_t oinserts, odeletes, osearches, oentries, oreferences, omemory, omemory_index, oduplications, oreleases;
        string_statistics(&oinserts, &odeletes, &osearches, &oentries, &oreferences, &omemory, &omemory_index, &oduplications, &oreleases);

        time_t seconds_to_run = 5;
        int threads_to_create = 2;
        fprintf(
            stderr,
            "Checking string concurrency with %d threads for %lld seconds...\n",
            threads_to_create,
            (long long)seconds_to_run);
        // check string concurrency
        ND_THREAD *threads[threads_to_create];
        tu.join = 0;
        for (int i = 0; i < threads_to_create; i++) {
            char buf[100 + 1];
            snprintf(buf, 100, "string%d", i);
            threads[i] = nd_thread_create(buf, NETDATA_THREAD_OPTION_DONT_LOG | NETDATA_THREAD_OPTION_JOINABLE, string_thread, &tu);
        }
        sleep_usec(seconds_to_run * USEC_PER_SEC);

        __atomic_store_n(&tu.join, 1, __ATOMIC_RELAXED);
        for (int i = 0; i < threads_to_create; i++)
            nd_thread_join(threads[i]);

        size_t inserts, deletes, searches, sentries, references, memory, memory_index, duplications, releases;
        string_statistics(&inserts, &deletes, &searches, &sentries, &references, &memory, &memory_index, &duplications, &releases);

        fprintf(stderr, "inserts %zu, deletes %zu, searches %zu, entries %zu, references %zu, memory %zu, duplications %zu, releases %zu\n",
                inserts - oinserts, deletes - odeletes, searches - osearches, sentries - oentries, references - oreferences, memory - omemory, duplications - oduplications, releases - oreleases);

#ifdef NETDATA_INTERNAL_CHECKS
        size_t found_deleted_on_search = unittest_string_found_deleted_on_search(),
               found_available_on_search = unittest_string_found_available_on_search(),
               found_deleted_on_insert = unittest_string_found_deleted_on_insert(),
               found_available_on_insert = unittest_string_found_available_on_insert(),
               spins = unittest_string_spins();

        fprintf(stderr, "on insert: %zu ok + %zu deleted\non search: %zu ok + %zu deleted\nspins: %zu\n",
                found_available_on_insert - ofound_available_on_insert,
                found_deleted_on_insert - ofound_deleted_on_insert,
                found_available_on_search - ofound_available_on_search,
                found_deleted_on_search - ofound_deleted_on_search,
                spins - ospins
        );
#endif
    }

    string_unittest_free_char_pp(names, entries);

    fprintf(stderr, "\n%zu errors found\n", errors);
    return  errors ? 1 : 0;
}

void string_init(void) {
    for (size_t i = 0; i != STRING_PARTITIONS; i++)
        rw_spinlock_init(&string_base[i].spinlock);
}
