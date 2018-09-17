// SPDX-License-Identifier: GPL-3.0+
#ifndef NETDATA_ADAPTIVE_RESORTABLE_LIST_H
#define NETDATA_ADAPTIVE_RESORTABLE_LIST_H

/*
 * ADAPTIVE RE-SORTABLE LIST
 * This structure allows netdata to read a file of NAME VALUE lines
 * in the fastest possible way.
 *
 * It maintains a linked list of all NAME (keywords), sorted in the
 * same order as found in the source data file.
 * The linked list is kept sorted at all times - the source file
 * may change at any time, the list will adapt.
 *
 * The caller:
 *
 * 1. calls arl_create() to create a list
 *
 * 2. calls arl_expect() to register the expected keyword
 *
 * Then:
 *
 * 3. calls arl_begin() to initiate a data collection iteration.
 *    This is to be called just ONCE every time the source is re-scanned.
 *
 * 4. calls arl_check() for each line read from the file.
 *
 * Finally:
 *
 * 5. calls arl_free() to destroy this and free all memory.
 *
 * The program will call the processor() function, given to
 * arl_create(), for each expected keyword found.
 * The default processor() expects dst to be an unsigned long long *.
 *
 * LIMITATIONS
 * DO NOT USE THIS IF THE A NAME/KEYWORD MAY APPEAR MORE THAN
 * ONCE IN THE SOURCE DATA SET.
 */

#include "common.h"

#define ARL_ENTRY_FLAG_FOUND    0x01    // the entry has been found in the source data
#define ARL_ENTRY_FLAG_EXPECTED 0x02    // the entry is expected by the program
#define ARL_ENTRY_FLAG_DYNAMIC  0x04    // the entry was dynamically allocated, from source data

typedef struct arl_entry {
    char *name;             // the keywords
    uint32_t hash;          // the hash of the keyword

    void *dst;              // the dst to pass to the processor

    uint8_t flags;          // ARL_ENTRY_FLAG_*

    // the processor to do the job
    void (*processor)(const char *name, uint32_t hash, const char *value, void *dst);

    // double linked list for fast re-linkings
    struct arl_entry *prev, *next;
} ARL_ENTRY;

typedef struct arl_base {
    char *name;

    size_t iteration;   // incremented on each iteration (arl_begin())
    size_t found;       // the number of expected keywords found in this iteration
    size_t expected;    // the number of expected keywords
    size_t wanted;      // the number of wanted keywords
                        // i.e. the number of keywords found and expected

    size_t relinkings;  // the number of relinkings we have made so far

    size_t allocated;   // the number of keywords allocated
    size_t fred;        // the number of keywords cleaned up

    size_t rechecks;    // the number of iterations between re-checks of the
                        // wanted number of keywords
                        // this is only needed in cases where the source
                        // is having less lines over time.

    size_t added;       // it is non-zero if new keywords have been added
                        // this is only needed to detect new lines have
                        // been added to the file, over time.

#ifdef NETDATA_INTERNAL_CHECKS
    size_t fast;        // the number of times we have taken the fast path
    size_t slow;        // the number of times we have taken the slow path
#endif

    // the processor to do the job
    void (*processor)(const char *name, uint32_t hash, const char *value, void *dst);

    // the linked list of the keywords
    ARL_ENTRY *head;

    // since we keep the list of keywords sorted (as found in the source data)
    // this is next keyword that we expect to find in the source data.
    ARL_ENTRY *next_keyword;
} ARL_BASE;

// create a new ARL
extern ARL_BASE *arl_create(const char *name, void (*processor)(const char *, uint32_t, const char *, void *), size_t rechecks);

// free an ARL
extern void arl_free(ARL_BASE *arl_base);

// register an expected keyword to the ARL
// together with its destination ( i.e. the output of the processor() )
extern ARL_ENTRY *arl_expect_custom(ARL_BASE *base, const char *keyword, void (*processor)(const char *name, uint32_t hash, const char *value, void *dst), void *dst);
#define arl_expect(base, keyword, dst) arl_expect_custom(base, keyword, NULL, dst)

// an internal call to complete the check() call
extern int arl_find_or_create_and_relink(ARL_BASE *base, const char *s, const char *value);

// begin an ARL iteration
extern void arl_begin(ARL_BASE *base);

extern void arl_callback_str2ull(const char *name, uint32_t hash, const char *value, void *dst);
extern void arl_callback_str2kernel_uint_t(const char *name, uint32_t hash, const char *value, void *dst);
extern void arl_callback_ssize_t(const char *name, uint32_t hash, const char *value, void *dst);

// check a keyword against the ARL
// this is to be called for each keyword read from source data
// s = the keyword, as collected
// src = the src data to be passed to the processor
// it is defined in the header file in order to be inlined
static inline int arl_check(ARL_BASE *base, const char *keyword, const char *value) {
    ARL_ENTRY *e = base->next_keyword;

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely((base->fast + base->slow) % (base->expected + base->allocated) == 0 && (base->fast + base->slow) > (base->expected + base->allocated) * base->iteration))
        info("ARL '%s': Did you forget to call arl_begin()?", base->name);
#endif

    // it should be the first entry (pointed by base->next_keyword)
    if(likely(!strcmp(keyword, e->name))) {
        // it is

#ifdef NETDATA_INTERNAL_CHECKS
        base->fast++;
#endif

        e->flags |= ARL_ENTRY_FLAG_FOUND;

        // execute the processor
        if(unlikely(e->dst)) {
            e->processor(e->name, e->hash, value, e->dst);
            base->found++;
        }

        // be prepared for the next iteration
        base->next_keyword = e->next;
        if(unlikely(!base->next_keyword))
            base->next_keyword = base->head;

        // stop if we collected all the values for this iteration
        if(unlikely(base->found == base->wanted)) {
            // fprintf(stderr, "FOUND ALL WANTED 2: found = %zu, wanted = %zu, expected %zu\n", base->found, base->wanted, base->expected);
            return 1;
        }

        return 0;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    base->slow++;
#endif

    // we read from source, a not-expected keyword
    return arl_find_or_create_and_relink(base, keyword, value);
}

#endif //NETDATA_ADAPTIVE_RESORTABLE_LIST_H
