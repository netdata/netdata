// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_H
#define NETDATA_DICTIONARY_H 1

#include "../libnetdata.h"


/*
 * Netdata DICTIONARY features:
 *
 * CLONE or LINK
 * Names and Values in the dictionary can be cloned or linked.
 * In clone mode, the dictionary does all the memory management.
 * The default is clone for both names and values.
 * Set DICTIONARY_FLAG_NAME_LINK_DONT_CLONE to link names.
 * Set DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE to link names.
 *
 * ORDERED
 * Items are ordered in the order they are added (new items are appended at the end).
 * You may reverse the order by setting the flag DICTIONARY_FLAG_ADD_IN_FRONT.
 *
 * LOOKUP
 * The dictionary uses JudyHS to maintain a very fast randomly accessible hash table.
 *
 * MULTI-THREADED and SINGLE-THREADED
 * Each dictionary may be single threaded (no locks), or multi-threaded (multiple readers or one writer).
 * The default is multi-threaded. Add the flag DICTIONARY_FLAG_SINGLE_THREADED for single-threaded.
 *
 * WALK-THROUGH and FOREACH traversal
 * The dictionary can be traversed on read or write mode, either with a callback (walkthrough) or with
 * a loop (foreach).
 *
 * In write mode traversal, the caller may delete only the current item, but may add as many items as needed.
 *
 */

typedef struct dictionary DICTIONARY;
typedef struct dictionary_item DICTIONARY_ITEM;

typedef enum dictionary_flags {
    DICTIONARY_FLAG_NONE                    = 0,        // the default is the opposite of all below
    DICTIONARY_FLAG_SINGLE_THREADED         = (1 << 0), // don't use any locks (default: use locks)
    DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE   = (1 << 1), // don't copy the value, just point to the one provided (default: copy)
    DICTIONARY_FLAG_NAME_LINK_DONT_CLONE    = (1 << 2), // don't copy the name, just point to the one provided (default: copy)
    DICTIONARY_FLAG_DONT_OVERWRITE_VALUE    = (1 << 3), // don't overwrite values of dictionary items (default: overwrite)
    DICTIONARY_FLAG_ADD_IN_FRONT            = (1 << 4), // add dictionary items at the front of the linked list (default: at the end)

    // to change the value of the following, you also need to change the corresponding #defines in dictionary.c
    DICTIONARY_FLAG_RESERVED1               = (1 << 29), // reserved for DICTIONARY_FLAG_EXCLUSIVE_ACCESS
    DICTIONARY_FLAG_RESERVED2               = (1 << 30), // reserved for DICTIONARY_FLAG_DESTROYED
    DICTIONARY_FLAG_RESERVED3               = (1 << 31), // reserved for DICTIONARY_FLAG_DEFER_ALL_DELETIONS
} DICTIONARY_FLAGS;

// Create a dictionary
#ifdef NETDATA_INTERNAL_CHECKS
#define dictionary_create(flags) dictionary_create_advanced_with_trace(flags, 0, __FUNCTION__, __LINE__, __FILE__);
#define dictionary_create_advanced(flags) dictionary_create_advanced_with_trace(flags, 0, __FUNCTION__, __LINE__, __FILE__);
extern DICTIONARY *dictionary_create_advanced_with_trace(DICTIONARY_FLAGS flags, size_t scratchpad_size, const char *function, size_t line, const char *file);
#else
#define dictionary_create(flags) dictionary_create_advanced(flags, 0);
extern DICTIONARY *dictionary_create_advanced(DICTIONARY_FLAGS flags, size_t scratchpad_size);
#endif

extern void *dictionary_scratchpad(DICTIONARY *dict);

// an insert callback to be called just after an item is added to the dictionary
// this callback is called while the dictionary is write locked!
extern void dictionary_register_insert_callback(DICTIONARY *dict, void (*ins_callback)(const char *name, void *value, void *data), void *data);

// a delete callback to be called just before an item is deleted forever
// this callback is called while the dictionary is write locked!
extern void dictionary_register_delete_callback(DICTIONARY *dict, void (*del_callback)(const char *name, void *value, void *data), void *data);

// a merge callback to be called when DICTIONARY_FLAG_DONT_OVERWRITE_VALUE
// and an item is already found in the dictionary - the dictionary does nothing else in this case
// the old_value will remain in the dictionary - the new_value is ignored
extern void dictionary_register_conflict_callback(DICTIONARY *dict, void (*conflict_callback)(const char *name, void *old_value, void *new_value, void *data), void *data);

// a reaction callback to be called after every item insertion or conflict
// after the constructors have finished and the items are fully available for use
// and the dictionary is not write locked anymore
extern void dictionary_register_react_callback(DICTIONARY *dict, void (*react_callback)(const char *name, void *value, void *data), void *data);

// Destroy a dictionary
// returns the number of bytes freed
// the returned value will not include name and value sizes if DICTIONARY_FLAG_WITH_STATISTICS is not set
extern size_t dictionary_destroy(DICTIONARY *dict);

// Set an item in the dictionary
// - if an item with the same name does not exist, create one
// - if an item with the same name exists, then:
//        a) if DICTIONARY_FLAG_DONT_OVERWRITE_VALUE is set, just return the existing value (ignore the new value)
//   else b) reset the value to the new value passed at the call
//
// When DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE is set, the value is linked, otherwise it is copied
// When DICTIONARY_FLAG_NAME_LINK_DONT_CLONE is set, the name is linked, otherwise it is copied
//
// When neither DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE nor DICTIONARY_FLAG_NAME_LINK_DONT_CLONE are set, all the
// memory management for names and values is done by the dictionary.
//
// Passing NULL as value, the dictionary will callocz() the newly allocated value, otherwise it will copy it.
// Passing 0 as value_len, the dictionary will set the value to NULL (no allocations for value will be made).
extern void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len);

// Get an item from the dictionary
// If it returns NULL, the item is not found
extern void *dictionary_get(DICTIONARY *dict, const char *name);

// Delete an item from the dictionary
// returns 0 if the item was found and has been deleted
// returns -1 if the item was not found in the index
extern int dictionary_del(DICTIONARY *dict, const char *name);

extern DICTIONARY_ITEM *dictionary_get_and_acquire_item_unsafe(DICTIONARY *dict, const char *name);
extern DICTIONARY_ITEM *dictionary_get_and_acquire_item(DICTIONARY *dict, const char *name);

extern DICTIONARY_ITEM *dictionary_set_and_acquire_item_unsafe(DICTIONARY *dict, const char *name, void *value, size_t value_len);
extern DICTIONARY_ITEM *dictionary_set_and_acquire_item(DICTIONARY *dict, const char *name, void *value, size_t value_len);

extern void dictionary_acquired_item_release_unsafe(DICTIONARY *dict, DICTIONARY_ITEM *item);
extern void dictionary_acquired_item_release(DICTIONARY *dict, DICTIONARY_ITEM *item);

extern DICTIONARY_ITEM *dictionary_acquired_item_dup(DICTIONARY_ITEM *item);
extern const char *dictionary_acquired_item_name(DICTIONARY_ITEM *item);
extern void *dictionary_acquired_item_value(DICTIONARY_ITEM *item);

// UNSAFE functions, without locks
// to be used when the user is traversing with the right lock type
// Read lock is acquired by dictionary_walktrhough_read() and dfe_start_read()
// Write lock is acquired by dictionary_walktrhough_write() and dfe_start_write()
// For code readability, please use these macros:
#define dictionary_get_having_read_lock(dict, name) dictionary_get_unsafe(dict, name)
#define dictionary_get_having_write_lock(dict, name) dictionary_get_unsafe(dict, name)
#define dictionary_set_having_write_lock(dict, name, value, value_len) dictionary_set_unsafe(dict, name, value, value_len)
#define dictionary_del_having_write_lock(dict, name) dictionary_del_unsafe(dict, name)

extern void *dictionary_get_unsafe(DICTIONARY *dict, const char *name);
extern void *dictionary_set_unsafe(DICTIONARY *dict, const char *name, void *value, size_t value_len);
extern int dictionary_del_unsafe(DICTIONARY *dict, const char *name);

// Traverse (walk through) the items of the dictionary.
// The order of traversal is currently the order of insertion.
//
// The callback function may return a negative number to stop the traversal,
// in which case that negative value is returned to the caller.
//
// If all callback calls return zero or positive numbers, the sum of all of
// them is returned to the caller.
//
// You cannot alter the dictionary from inside a dictionary_walkthrough_read() - deadlock!
// You can only delete the current item from inside a dictionary_walkthrough_write() - you can add as many as you want.
//
#define dictionary_walkthrough_read(dict, callback, data) dictionary_walkthrough_rw(dict, 'r', callback, data)
#define dictionary_walkthrough_write(dict, callback, data) dictionary_walkthrough_rw(dict, 'w', callback, data)
extern int dictionary_walkthrough_rw(DICTIONARY *dict, char rw, int (*callback)(const char *name, void *value, void *data), void *data);

#define dictionary_sorted_walkthrough_read(dict, callback, data) dictionary_sorted_walkthrough_rw(dict, 'r', callback, data)
#define dictionary_sorted_walkthrough_write(dict, callback, data) dictionary_sorted_walkthrough_rw(dict, 'w', callback, data)
int dictionary_sorted_walkthrough_rw(DICTIONARY *dict, char rw, int (*callback)(const char *name, void *entry, void *data), void *data);

// Traverse with foreach
//
// Use like this:
//
//  DICTFE dfe = {};
//  for(MY_ITEM *item = dfe_start_read(&dfe, dict); item ; item = dfe_next(&dfe)) {
//     // do things with the item and its dfe.name
//  }
//  dfe_done(&dfe);
//
// You cannot alter the dictionary from within a dfe_read_start() - deadlock!
// You can only delete the current item from inside a dfe_start_write() - you can add as many as you want.
//

#ifdef DICTIONARY_INTERNALS
#define DICTFE_CONST
#else
#define DICTFE_CONST const
#endif

#define DICTIONARY_LOCK_READ     'r'
#define DICTIONARY_LOCK_WRITE    'w'
#define DICTIONARY_LOCK_REETRANT 'z'
#define DICTIONARY_LOCK_NONE     'u'

typedef DICTFE_CONST struct dictionary_foreach {
    DICTFE_CONST char *name;    // the dictionary name of the last item used
    void *value;                // the dictionary value of the last item used
                                // same as the return value of dictfe_start() and dictfe_next()

    // the following are for internal use only - to keep track of the point we are
    char rw;                    // the lock mode 'r' or 'w'
    usec_t started_ut;          // the time the caller started iterating (now_realtime_usec())
    DICTIONARY *dict;           // the dictionary upon we work
    void *last_item;            // the item we work on, to remember the position we are at
} DICTFE;

#define dfe_start_read(dict, value) dfe_start_rw(dict, value, DICTIONARY_LOCK_READ)
#define dfe_start_write(dict, value) dfe_start_rw(dict, value, DICTIONARY_LOCK_WRITE)
#define dfe_start_rw(dict, value, mode) \
        do { \
            DICTFE value ## _dfe = {};  \
            const char *value ## _name; (void)(value ## _name); (void)value; \
            for((value) = dictionary_foreach_start_rw(&value ## _dfe, (dict), (mode)), ( value ## _name ) = value ## _dfe.name; \
                (value ## _dfe.name) ;\
                (value) = dictionary_foreach_next(&value ## _dfe), ( value ## _name ) = value ## _dfe.name) \
            {

#define dfe_done(value) \
            }           \
            dictionary_foreach_done(&value ## _dfe); \
        } while(0)

extern void * dictionary_foreach_start_rw(DICTFE *dfe, DICTIONARY *dict, char rw);
extern void * dictionary_foreach_next(DICTFE *dfe);
extern usec_t dictionary_foreach_done(DICTFE *dfe);

// Get statistics about the dictionary
extern long int dictionary_stats_allocated_memory(DICTIONARY *dict);
extern long int dictionary_stats_entries(DICTIONARY *dict);
extern size_t dictionary_stats_version(DICTIONARY *dict);
extern size_t dictionary_stats_inserts(DICTIONARY *dict);
extern size_t dictionary_stats_searches(DICTIONARY *dict);
extern size_t dictionary_stats_deletes(DICTIONARY *dict);
extern size_t dictionary_stats_resets(DICTIONARY *dict);
extern size_t dictionary_stats_walkthroughs(DICTIONARY *dict);
extern size_t dictionary_stats_referenced_items(DICTIONARY *dict);

extern int dictionary_unittest(size_t entries);

// ----------------------------------------------------------------------------
// STRING implementation

typedef struct netdata_string STRING;
extern STRING *string_strdupz(const char *str);
extern STRING *string_dup(STRING *string);
extern void string_freez(STRING *string);
extern size_t string_length(STRING *string);
extern const char *string2str(STRING *string) NEVERNULL;

// keep common prefix/suffix and replace everything else with [x]
extern STRING *string_2way_merge(STRING *a, STRING *b);

static inline int string_cmp(STRING *s1, STRING *s2) {
    // STRINGs are deduplicated, so the same strings have the same pointer
    if(unlikely(s1 == s2)) return 0;

    // they differ, do the typical comparison
    return strcmp(string2str(s1), string2str(s2));
}

extern void string_statistics(size_t *inserts, size_t *deletes, size_t *searches, size_t *entries, size_t *references, size_t *memory, size_t *duplications, size_t *releases);

// ----------------------------------------------------------------------------
// THREAD CACHE

extern void *thread_cache_entry_get(const char *str, void *(*prepare_the_value)(const char *str, void *data), void *data);
extern void thread_cache_destroy(void);

#endif /* NETDATA_DICTIONARY_H */
