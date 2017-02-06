#ifndef NETDATA_ADAPTIVE_RESORTABLE_LIST_H
#define NETDATA_ADAPTIVE_RESORTABLE_LIST_H

/**
 * @file adaptive_resortable_list.h
 * @brief Adaptive resortable linked list to read key value pairs.
 *
 * It maintains a linked list of all `name` (keywords), sorted in the
 * same order as found in the source.
 * The linked list is kept sorted at all times - the source order may change at
 * any time, the list will adapt.
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
 * __LIMITATIONS:__
 * Do not use this if a keyword may apperar more than once in the source data set.
 */

#include "common.h"

#define ARL_ENTRY_FLAG_FOUND    0x01    ///< ARL entry has been found in the source.
#define ARL_ENTRY_FLAG_EXPECTED 0x02    ///< ARL entry is expected by the program.
#define ARL_ENTRY_FLAG_DYNAMIC  0x04    ///< ARL entry was dynamically allocated, from source data.

/** Double linked list of expected entries in an ARL */
typedef struct arl_entry {
    char *name;             ///< Expected keyword.
    uint32_t hash;          ///< Hash of `name`.

    void *dst;              ///< `dst` to pass to processor()

    uint8_t flags;          ///< ARL_ENTRY_FLAG_*

    struct arl_entry *prev; ///< Previous item.
    struct arl_entry *next; ///< Next item.
} ARL_ENTRY;

/** Adaptive resortable list. */
typedef struct arl_base {
    char *name;         ///< Name of the list. 

    size_t iteration;   ///< Incremented on each iteration (arl_begin()).
    size_t found;       ///< Number of expected keywords found in current iteration.
    size_t expected;    ///< Number of expected keywords.
    size_t wanted;      ///< Number of wanted keywords
                        ///< i.e. the number of keywords found and expected

    size_t relinkings;  ///< Number of relinkings made so far.

    size_t allocated;   ///< Number of keyword allocations made.
    size_t fred;        ///< Number of keyword cleaneups made.

    size_t rechecks;    ///< Number of iterations between re-checks of the
                        ///< wanted number of keywords.
                        ///< This is only needed in cases where the source
                        ///< is having less lines over time.

    size_t added;       ///< Number of new keywords in current iteration.
                        ///< This is only needed to detect new lines have
                        ///< been added to the file, over time.

#ifdef NETDATA_INTERNAL_CHECKS
    size_t fast;        ///< Number of times we have taken the fast path.
    size_t slow;        ///< Number of times we have taken the slow path.
#endif

    /**
     * Callback to update `dst` with `value` if we found an expected name.
     *
     * @param name The matching keyword.
     * @param hash of `name`
     * @param value of `name` 
     * @param dst to copy `value` into
     */
    void (*processor)(const char *name, uint32_t hash, const char *value, void *dst);

    /// Linked list of the keywords
    ARL_ENTRY *head;

    /// The next keyword we expect to find in the source data.
    ARL_ENTRY *next_keyword;
} ARL_BASE;

/**
 * Create a new ARL.
 *
 * If `processor` is `NULL` the default processor is used.
 * It converts the value to an unsigned long long.
 * \dontinclude adaptive_resortable_list.c
 * \skip arl_callback_str2ull
 * \until }
 *
 * \todo How to choose a value for rechecks?
 *
 * @param name of the ARL
 * @param processor Callback to process a match.
 * @param rechecks The number of iterations between re-checks of the wanted number of keywords.
 * @return the ARL. 
 */
extern ARL_BASE *arl_create(const char *name, void (*processor)(const char *, uint32_t, const char *, void *), size_t rechecks);

/**
 * Free an ARL
 *
 * @param arl_base allocated with arl_create()
 */
extern void arl_free(ARL_BASE *arl_base);

/**
 * Register an expected keyword to the ARL together with its destination.
 *
 * The processor of AVL will write to `dst` if arl_check() matches `keyword`.
 *
 * @param base The ARL.
 * @param keyword to register
 * @param dst Destination.
 * @return the registered ARL_ENTRY
 */
extern ARL_ENTRY *arl_expect(ARL_BASE *base, const char *keyword, void *dst);

/**
 * Internal call to complete the check() call.
 *
 * Do not use this as a caller.
 *
 * @param base The ARL.
 * @param s Keywoard of arl_check().
 * @param value of arl_check()
 * @return 1 if all expected keywords where found in this iteration. 0 if not.
 */
int arl_find_or_create_and_relink(ARL_BASE *base, const char *s, const char *value);

/**
 * Begin an iteration.
 *
 * @param base The ARL.
 */
extern void arl_begin(ARL_BASE *base);

/**
 * Check a keyword against the ARL.
 *
 * This is to be called for each keyword read from source.
 *
 * It is defined in the header file in order to be inlined.
 *
 * @param base The ARL.
 * @param keyword read from source
 * @param value of `keyword`
 * @return 1 if all expected keywords where found in this iteration. 0 if not.
 */
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
            base->processor(e->name, e->hash, value, e->dst);
            base->found++;
        }

        // be prepared for the next iteration
        base->next_keyword = e->next;
        if(unlikely(!base->next_keyword))
            base->next_keyword = base->head;

        // stop if we collected all the values for this iteration
        if(unlikely(base->found == base->wanted))
            return 1;

        return 0;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    base->slow++;
#endif

    // we read from source, a not-expected keyword
    return arl_find_or_create_and_relink(base, keyword, value);
}

#endif //NETDATA_ADAPTIVE_RESORTABLE_LIST_H
