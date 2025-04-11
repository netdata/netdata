// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_DEBUG_H
#define NETDATA_DICTIONARY_DEBUG_H

#include "dictionary-internals.h"

// Initialize dictionary debugging
void dictionary_debug_init(void);

// Register a dictionary for tracking (when FSANITIZE_ADDRESS is defined)
void dictionary_debug_track_dict(DICTIONARY *dict);

// Unregister a dictionary from tracking
void dictionary_debug_untrack_dict(DICTIONARY *dict);

// Print information about dictionaries that are still allocated
void dictionary_print_still_allocated_stacktraces(void);

// Print information about dictionaries that have delayed destruction due to referenced items
void dictionary_debug_print_delayed_dictionaries(size_t destroyed_dicts);

// Clean up resources used for tracking
void dictionary_debug_shutdown(void);

// API internal check functions - moved from dictionary.c
#ifdef NETDATA_INTERNAL_CHECKS
void dictionary_debug_internal_check_with_trace(DICTIONARY *dict, DICTIONARY_ITEM *item, const char *function, bool allow_null_dict, bool allow_null_item);
#define dictionary_debug_internal_check(dict, item, allow_null_dict, allow_null_item) \
    dictionary_debug_internal_check_with_trace(dict, item, __FUNCTION__, allow_null_dict, allow_null_item)
#else
#define dictionary_debug_internal_check(dict, item, allow_null_dict, allow_null_item) debug_dummy()
#define dictionary_debug_internal_check_with_trace(dict, item, function, allow_null_dict, allow_null_item) debug_dummy()
#endif

#endif // NETDATA_DICTIONARY_DEBUG_H