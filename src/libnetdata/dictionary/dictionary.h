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
 * Set DICT_OPTION_NAME_LINK_DONT_CLONE to link names.
 * Set DICT_OPTION_VALUE_LINK_DONT_CLONE to link names.
 *
 * ORDERED
 * Items are ordered in the order they are added (new items are appended at the end).
 * You may reverse the order by setting the flag DICT_OPTION_ADD_IN_FRONT.
 *
 * LOOKUP
 * The dictionary uses JudyHS to maintain a very fast randomly accessible hash table.
 *
 * MULTI-THREADED and SINGLE-THREADED
 * Each dictionary may be single threaded (no locks), or multi-threaded (multiple readers or one writer).
 * The default is multi-threaded. Add the flag DICT_OPTION_SINGLE_THREADED for single-threaded.
 *
 * WALK-THROUGH and FOREACH traversal
 * The dictionary can be traversed on read or write mode, either with a callback (walkthrough) or with
 * a loop (foreach).
 *
 * In write mode traversal, the caller may delete only the current item, but may add as many items as needed.
 *
 */

#ifdef NETDATA_INTERNAL_CHECKS
#define DICT_WITH_STATS 1
#endif

#ifdef DICTIONARY_INTERNALS
#define DICTFE_CONST
#define DICT_ITEM_CONST
#else
#define DICTFE_CONST const
#define DICT_ITEM_CONST const
#endif

typedef struct dictionary DICTIONARY;
typedef struct dictionary_item DICTIONARY_ITEM;

typedef enum __attribute__((packed)) dictionary_options {
    DICT_OPTION_NONE                    = 0,        // the default is the opposite of all below
    DICT_OPTION_SINGLE_THREADED         = (1 << 0), // don't use any locks (default: use locks)
    DICT_OPTION_VALUE_LINK_DONT_CLONE   = (1 << 1), // don't copy the value, just point to the one provided (default: copy)
    DICT_OPTION_NAME_LINK_DONT_CLONE    = (1 << 2), // don't copy the name, just point to the one provided (default: copy)
    DICT_OPTION_DONT_OVERWRITE_VALUE    = (1 << 3), // don't overwrite values of dictionary items (default: overwrite)
    DICT_OPTION_ADD_IN_FRONT            = (1 << 4), // add dictionary items at the front of the linked list (default: at the end)
    DICT_OPTION_FIXED_SIZE              = (1 << 5), // the items of the dictionary have a fixed size
    DICT_OPTION_INDEX_JUDY              = (1 << 6), // the default, if no other indexing is set
//    DICT_OPTION_INDEX_HASHTABLE         = (1 << 7), // use SIMPLE_HASHTABLE for indexing
} DICT_OPTIONS;

struct dictionary_stats {
    const char *name;               // the name of the category

    struct {
        PAD64(size_t) active;              // the number of active dictionaries
        PAD64(size_t) deleted;             // the number of dictionaries queued for destruction
    } dictionaries;

    struct {
        PAD64(long) entries;               // active items in the dictionary
        PAD64(long) pending_deletion;      // pending deletion items in the dictionary
        PAD64(long) referenced;            // referenced items in the dictionary
    } items;

    struct {
        PAD64(size_t) creations;           // dictionary creations
        PAD64(size_t) destructions;        // dictionary destructions
        PAD64(size_t) flushes;             // dictionary flushes
        PAD64(size_t) traversals;          // dictionary foreach
        PAD64(size_t) walkthroughs;        // dictionary walkthrough
        PAD64(size_t) garbage_collections; // dictionary garbage collections
        PAD64(size_t) searches;            // item searches
        PAD64(size_t) inserts;             // item inserts
        PAD64(size_t) resets;              // item resets
        PAD64(size_t) deletes;             // item deletes
    } ops;

    struct {
        PAD64(size_t) inserts;             // number of times the insert callback is called
        PAD64(size_t) conflicts;           // number of times the conflict callback is called
        PAD64(size_t) reacts;              // number of times the react callback is called
        PAD64(size_t) deletes;             // number of times the delete callback is called
    } callbacks;

    // memory
    struct {
        PAD64(ssize_t) index;              // bytes of keys indexed (indication of the index size)
        PAD64(ssize_t) values;             // bytes of caller structures
        PAD64(ssize_t) dict;               // bytes of the structures dictionary needs
    } memory;

    // spin locks
    struct {
        PAD64(size_t) use_spins;           // number of times a reference to item had to spin to acquire it or ignore it
        PAD64(size_t) search_spins;        // number of times a successful search result had to be thrown away
        PAD64(size_t) insert_spins;        // number of times an insertion to the hash table had to be repeated
        PAD64(size_t) delete_spins;        // number of times a deletion had to spin to get a decision
    } spin_locks;
};

// Create a dictionary
#ifdef NETDATA_INTERNAL_CHECKS
#define dictionary_create(options) dictionary_create_advanced_with_trace(options, NULL, 0, __FUNCTION__, __LINE__, __FILE__)
#define dictionary_create_advanced(options, stats, fixed_size) dictionary_create_advanced_with_trace(options, stats, fixed_size, __FUNCTION__, __LINE__, __FILE__)
DICTIONARY *dictionary_create_advanced_with_trace(DICT_OPTIONS options, struct dictionary_stats *stats, size_t fixed_size, const char *function, size_t line, const char *file);
#else
#define dictionary_create(options) dictionary_create_advanced(options, NULL, 0)
DICTIONARY *dictionary_create_advanced(DICT_OPTIONS options, struct dictionary_stats *stats, size_t fixed_size);
#endif

// Create a view on a dictionary
#ifdef NETDATA_INTERNAL_CHECKS
#define dictionary_create_view(master) dictionary_create_view_with_trace(master, __FUNCTION__, __LINE__, __FILE__)
DICTIONARY *dictionary_create_view_with_trace(DICTIONARY *master, const char *function, size_t line, const char *file);
#else
DICTIONARY *dictionary_create_view(DICTIONARY *master);
#endif

// an insert callback to be called just after an item is added to the dictionary
// this callback is called while the dictionary is write locked!
typedef void (*dict_cb_insert_t)(const DICTIONARY_ITEM *item, void *value, void *data);
void dictionary_register_insert_callback(DICTIONARY *dict, dict_cb_insert_t insert_callback, void *data);

// a delete callback to be called just before an item is deleted forever
// this callback is called while the dictionary is write locked!
typedef void (*dict_cb_delete_t)(const DICTIONARY_ITEM *item, void *value, void *data);
void dictionary_register_delete_callback(DICTIONARY *dict, dict_cb_delete_t delete_callback, void *data);

// a merge callback to be called when DICT_OPTION_DONT_OVERWRITE_VALUE
// and an item is already found in the dictionary - the dictionary does nothing else in this case
// the old_value will remain in the dictionary - the new_value is ignored
// The callback should return true if the value has been updated (it increases the dictionary version).
typedef bool (*dict_cb_conflict_t)(const DICTIONARY_ITEM *item, void *old_value, void *new_value, void *data);
void dictionary_register_conflict_callback(DICTIONARY *dict, dict_cb_conflict_t conflict_callback, void *data);

// a reaction callback to be called after every item insertion or conflict
// after the constructors have finished and the items are fully available for use
// and the dictionary is not write locked anymore
typedef void (*dict_cb_react_t)(const DICTIONARY_ITEM *item, void *value, void *data);
void dictionary_register_react_callback(DICTIONARY *dict, dict_cb_react_t react_callback, void *data);

// Destroy a dictionary
// Returns the number of bytes freed
// The returned value will not include name/key sizes
// Registered delete callbacks will be run for each item in the dictionary.
size_t dictionary_destroy(DICTIONARY *dict);

// Empties a dictionary
// Referenced items will survive, but are not offered anymore.
// Registered delete callbacks will be run for each item in the dictionary.
void dictionary_flush(DICTIONARY *dict);

void dictionary_version_increment(DICTIONARY *dict);

void dictionary_garbage_collect(DICTIONARY *dict);

void cleanup_destroyed_dictionaries(void);

// ----------------------------------------------------------------------------
// Set an item in the dictionary
//
// - if an item with the same name does not exist, create one
// - if an item with the same name exists, then:
//        a) if DICT_OPTION_DONT_OVERWRITE_VALUE is set, just return the existing value (ignore the new value)
//   else b) reset the value to the new value passed at the call
//
// When DICT_OPTION_VALUE_LINK_DONT_CLONE is set, the value is linked, otherwise it is copied
// When DICT_OPTION_NAME_LINK_DONT_CLONE is set, the name is linked, otherwise it is copied
//
// When neither DICT_OPTION_VALUE_LINK_DONT_CLONE nor DICT_OPTION_NAME_LINK_DONT_CLONE are set, all the
// memory management for names and values is done by the dictionary.
//
// Passing NULL as value, the dictionary will callocz() the newly allocated value, otherwise it will copy it.
// Passing 0 as value_len, the dictionary will set the value to NULL (no allocations for value will be made).
#define dictionary_set(dict, name, value, value_len) dictionary_set_advanced(dict, name, -1, value, value_len, NULL)
void *dictionary_set_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data);

#define dictionary_set_and_acquire_item(dict, name, value, value_len) dictionary_set_and_acquire_item_advanced(dict, name, -1, value, value_len, NULL)
DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_set_and_acquire_item_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, void *value, size_t value_len, void *constructor_data);

// set an item in a dictionary view
#define dictionary_view_set_and_acquire_item(dict, name, master_item) dictionary_view_set_and_acquire_item_advanced(dict, name, -1, master_item)
DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_view_set_and_acquire_item_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, DICTIONARY_ITEM *master_item);
#define dictionary_view_set(dict, name, master_item) dictionary_view_set_advanced(dict, name, -1, master_item)
void *dictionary_view_set_advanced(DICTIONARY *dict, const char *name, ssize_t name_len, DICT_ITEM_CONST DICTIONARY_ITEM *master_item);

// ----------------------------------------------------------------------------
// Get an item from the dictionary
// If it returns NULL, the item is not found

#define dictionary_get(dict, name) dictionary_get_advanced(dict, name, -1)
void *dictionary_get_advanced(DICTIONARY *dict, const char *name, ssize_t name_len);

#define dictionary_get_and_acquire_item(dict, name) dictionary_get_and_acquire_item_advanced(dict, name, -1)
DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_get_and_acquire_item_advanced(DICTIONARY *dict, const char *name, ssize_t name_len);


// ----------------------------------------------------------------------------
// Delete an item from the dictionary
// returns true if the item was found and has been deleted
// returns false if the item was not found in the index

#define dictionary_del(dict, name) dictionary_del_advanced(dict, name, -1)
bool dictionary_del_advanced(DICTIONARY *dict, const char *name, ssize_t name_len);

// ----------------------------------------------------------------------------
// reference counters management

void dictionary_acquired_item_release(DICTIONARY *dict, DICT_ITEM_CONST DICTIONARY_ITEM *item);

DICT_ITEM_CONST DICTIONARY_ITEM *dictionary_acquired_item_dup(DICTIONARY *dict, DICT_ITEM_CONST DICTIONARY_ITEM *item);

const char *dictionary_acquired_item_name(DICT_ITEM_CONST DICTIONARY_ITEM *item);
void *dictionary_acquired_item_value(DICT_ITEM_CONST DICTIONARY_ITEM *item);

size_t dictionary_acquired_item_references(DICT_ITEM_CONST DICTIONARY_ITEM *item);

// ----------------------------------------------------------------------------
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
typedef int (*dict_walkthrough_callback_t)(const DICTIONARY_ITEM *item, void *value, void *data);

#define dictionary_walkthrough_read(dict, callback, data) dictionary_walkthrough_rw(dict, 'r', callback, data)
#define dictionary_walkthrough_write(dict, callback, data) dictionary_walkthrough_rw(dict, 'w', callback, data)
int dictionary_walkthrough_rw(DICTIONARY *dict, char rw, dict_walkthrough_callback_t walkthrough_callback, void *data);

typedef int (*dict_item_comparator_t)(const DICTIONARY_ITEM **item1, const DICTIONARY_ITEM **item2);

#define dictionary_sorted_walkthrough_read(dict, callback, data) dictionary_sorted_walkthrough_rw(dict, 'r', callback, data, NULL)
#define dictionary_sorted_walkthrough_write(dict, callback, data) dictionary_sorted_walkthrough_rw(dict, 'w', callback, data, NULL)
int dictionary_sorted_walkthrough_rw(DICTIONARY *dict, char rw, dict_walkthrough_callback_t walkthrough_callback, void *data, dict_item_comparator_t item_comparator_callback);

// ----------------------------------------------------------------------------
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

#define DICTIONARY_LOCK_READ      'r'
#define DICTIONARY_LOCK_WRITE     'w'
#define DICTIONARY_LOCK_REENTRANT 'z'

void dictionary_write_lock(DICTIONARY *dict);
void dictionary_write_unlock(DICTIONARY *dict);

typedef DICTFE_CONST struct dictionary_foreach {
    DICTIONARY *dict; // the dictionary upon we work

    DICTIONARY_ITEM *item;      // the item we work on, to remember the position we are at
                                // this can be used with dictionary_acquired_item_dup() to
                                // acquire the currently working item.

    const char *name;           // the dictionary name of the last item used
    void *value;                // the dictionary value of the last item used
                                // same as the return value of dictfe_start() and dictfe_next()

    size_t counter;             // counts the number of iterations made, starting from zero

    char rw;                    // the lock mode 'r' or 'w'
    bool locked;                // true when the dictionary is locked
} DICTFE;

#define dfe_start_read(dict, value) dfe_start_rw(dict, value, DICTIONARY_LOCK_READ)
#define dfe_start_write(dict, value) dfe_start_rw(dict, value, DICTIONARY_LOCK_WRITE)
#define dfe_start_reentrant(dict, value) dfe_start_rw(dict, value, DICTIONARY_LOCK_REENTRANT)

#define dfe_start_rw(dict, value, mode)                                                             \
        do {                                                                                        \
            /* automatically cleanup DFE, to allow using return from within the loop */             \
            DICTFE _cleanup_(dictionary_foreach_done) value ## _dfe = {};                           \
            (void)(value); /* needed to avoid warning when looping without using this */            \
            for((value) = dictionary_foreach_start_rw(&value ## _dfe, (dict), (mode));              \
                (value ## _dfe.item) || (value) ;                                                   \
                (value) = dictionary_foreach_next(&value ## _dfe))                                  \
            {

#define dfe_done(value)                                                                             \
            }                                                                                       \
        } while(0)

#define dfe_unlock(value) dictionary_foreach_unlock(&value ## _dfe)

void *dictionary_foreach_start_rw(DICTFE *dfe, DICTIONARY *dict, char rw);
void *dictionary_foreach_next(DICTFE *dfe);
void  dictionary_foreach_done(DICTFE *dfe);
void  dictionary_foreach_unlock(DICTFE *dfe);

// ----------------------------------------------------------------------------
// Get statistics about the dictionary

size_t dictionary_version(DICTIONARY *dict);
size_t dictionary_entries(DICTIONARY *dict);
size_t dictionary_referenced_items(DICTIONARY *dict);

// for all cases that the caller does not provide a stats structure, this is where they are accumulated.
extern struct dictionary_stats dictionary_stats_category_other;

int dictionary_unittest(size_t entries);

#endif /* NETDATA_DICTIONARY_H */
