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

#ifndef DICTIONARY_INTERNALS
typedef void DICTIONARY;
#endif

typedef enum dictionary_flags {
    DICTIONARY_FLAG_NONE                   = 0,        // the default is the opposite of all below
    DICTIONARY_FLAG_SINGLE_THREADED        = (1 << 0), // don't use any locks (default: use locks)
    DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE  = (1 << 1), // don't copy the value, just point to the one provided (default: copy)
    DICTIONARY_FLAG_NAME_LINK_DONT_CLONE   = (1 << 2), // don't copy the name, just point to the one provided (default: copy)
    DICTIONARY_FLAG_WITH_STATISTICS        = (1 << 3), // maintain statistics about dictionary operations (default: disabled)
    DICTIONARY_FLAG_DONT_OVERWRITE_VALUE   = (1 << 4), // don't overwrite values of dictionary items (default: overwrite)
    DICTIONARY_FLAG_ADD_IN_FRONT           = (1 << 5), // add dictionary items at the front of the linked list (default: at the end)
    DICTIONARY_FLAG_RESERVED1              = (1 << 6), // this is reserved for DICTIONARY_FLAG_REFERENCE_COUNTERS
} DICTIONARY_FLAGS;

// Create a dictionary
extern DICTIONARY *dictionary_create(DICTIONARY_FLAGS flags);

// an insert callback to be called just after an item is added to the dictionary
// this callback is called while the dictionary is write locked!
extern void dictionary_register_insert_callback(DICTIONARY *dict, void (*ins_callback)(const char *name, void *value, void *data), void *data);

// a delete callback to be called just before an item is deleted forever
// this callback is called while the dictionary is write locked!
extern void dictionary_register_delete_callback(DICTIONARY *dict, void (*del_callback)(const char *name, void *value, void *data), void *data);

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
extern void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len) NEVERNULL;

// Get an item from the dictionary
// If it returns NULL, the item is not found
extern void *dictionary_get(DICTIONARY *dict, const char *name);

// Delete an item from the dictionary
// returns 0 if the item was found and has been deleted
// returns -1 if the item was not found in the index
extern int dictionary_del(DICTIONARY *dict, const char *name);

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

typedef DICTFE_CONST struct dictionary_foreach {
    DICTFE_CONST char *name;    // the dictionary name of the last item used
    void *value;                // the dictionary value of the last item used
                                // same as the return value of dictfe_start() and dictfe_next()

    // the following are for internal use only - to keep track of the point we are
    usec_t started_ut;          // the time the caller started iterating (now_realtime_usec())
    DICTIONARY *dict;           // the dictionary upon we work
    void *last_position_index;  // the internal position index, to remember the position we are at
    void *next_position_index;  // the internal position index, of the next item
} DICTFE;

#define dfe_start_read(dict, value) dfe_start_rw(dict, value, 'r')
#define dfe_start_write(dict, value) dfe_start_rw(dict, value, 'w')
#define dfe_start_rw(dict, value, mode) \
        do { \
            DICTFE dfe_ ## value = {};  \
            const char *value ## _name; (void)(value ## _name); \
            for((value) = dictionary_foreach_start_rw(&dfe_ ## value, (dict), (mode)), ( value ## _name ) = dfe_ ## value.name; \
                (value) ;\
                (value) = dictionary_foreach_next(&dfe_ ## value), ( value ## _name ) = dfe_ ## value.name)

#define dfe_done(value) \
            dictionary_foreach_done(&dfe_ ## value); \
        } while(0)

extern void * dictionary_foreach_start_rw(DICTFE *dfe, DICTIONARY *dict, char rw);
extern void * dictionary_foreach_next(DICTFE *dfe);
extern usec_t dictionary_foreach_done(DICTFE *dfe);

// Get statistics about the dictionary
// If DICTIONARY_FLAG_WITH_STATISTICS is not set, these return zero
extern size_t dictionary_stats_allocated_memory(DICTIONARY *dict);
extern size_t dictionary_stats_entries(DICTIONARY *dict);
extern size_t dictionary_stats_inserts(DICTIONARY *dict);
extern size_t dictionary_stats_searches(DICTIONARY *dict);
extern size_t dictionary_stats_deletes(DICTIONARY *dict);
extern size_t dictionary_stats_resets(DICTIONARY *dict);

extern int dictionary_unittest(size_t entries);

#endif /* NETDATA_DICTIONARY_H */
