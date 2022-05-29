// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_H
#define NETDATA_DICTIONARY_H 1

#include "../libnetdata.h"

typedef void DICTIONARY;

#define DICTIONARY_FLAG_NONE                    0x00 // the default is the opposite of all below
#define DICTIONARY_FLAG_SINGLE_THREADED         0x01 // don't use any locks
#define DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE   0x02 // don't copy the value, just point to the one provided
#define DICTIONARY_FLAG_NAME_LINK_DONT_CLONE    0x04 // don't copy the name, just point to the one provided
#define DICTIONARY_FLAG_WITH_STATISTICS         0x08 // maintain statistics about dictionary operations
#define DICTIONARY_FLAG_DONT_OVERWRITE_VALUE    0x10 // don't overwrite a value of an item in the dictionary

// Create a dictionary
extern DICTIONARY *dictionary_create(uint8_t flags);

// Destroy a dictionary
extern void dictionary_destroy(DICTIONARY *dict);

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
#define dictionary_set(dict, name, value, value_len) dictionary_set_with_name_ptr(dict, name, value, value_len, NULL)
extern void *dictionary_set_with_name_ptr(DICTIONARY *dict, const char *name, void *value, size_t value_len, char **name_ptr) NEVERNULL;

// Get an item from the dictionary
// If it returns NULL, the item is not found
extern void *dictionary_get(DICTIONARY *dict, const char *name);

// Delete an item from the dictionary
// returns 0 if the item was found and has been deleted
// returns -1 if the item was not found in the index
extern int dictionary_del(DICTIONARY *dict, const char *name);

extern int dictionary_walkthrough(DICTIONARY *dict, int (*callback)(void *value, void *data), void *data);
extern int dictionary_walkthrough_with_name(DICTIONARY *dict, int (*callback)(const char *name, void *value, void *data), void *data);

// Get statistics about the dictionary
// If DICTIONARY_FLAG_WITH_STATISTICS is not set, these return zero
extern size_t dictionary_allocated_memory(DICTIONARY *dict);
extern size_t dictionary_entries(DICTIONARY *dict);

extern int dictionary_unittest(size_t entries);

#endif /* NETDATA_DICTIONARY_H */
