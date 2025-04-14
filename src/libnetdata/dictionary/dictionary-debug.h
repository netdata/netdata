// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_DEBUG_H
#define NETDATA_DICTIONARY_DEBUG_H

#include "dictionary.h"

#ifdef FSANITIZE_ADDRESS

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

void dictionary_debug_internal_check_with_trace(DICTIONARY *dict, DICTIONARY_ITEM *item, const char *function, bool allow_null_dict, bool allow_null_item);
#define dictionary_debug_internal_check(dict, item, allow_null_dict, allow_null_item) \
    dictionary_debug_internal_check_with_trace(dict, item, __FUNCTION__, allow_null_dict, allow_null_item)

// For internal_error with dictionary stacktrace
#define dictionary_internal_error(condition, dict, fmt, args...) do { \
    if(unlikely(condition)) { \
        BUFFER *__wb = buffer_create(1024, NULL); \
        if ((dict) && ((dict)->stacktraces.num_stacktraces > 0)) { \
            buffer_sprintf(__wb, fmt " Dictionary accessed at:\n", ##args); \
            for (int i = 0; i < (dict)->stacktraces.num_stacktraces && i < 3; i++) { \
                if ((dict)->stacktraces.stacktraces[i]) { \
                    buffer_sprintf(__wb, "Stacktrace #%d:\n", i+1); \
                    stacktrace_to_buffer((dict)->stacktraces.stacktraces[i], __wb); \
                    buffer_strcat(__wb, "\n"); \
                } \
            } \
            if ((dict)->stacktraces.num_stacktraces > 3) \
                buffer_sprintf(__wb, "...and %d more stacktraces\n", (dict)->stacktraces.num_stacktraces - 3); \
        } else { \
            buffer_sprintf(__wb, fmt " Dictionary stacktrace not available", ##args); \
        } \
        netdata_logger(NDLS_DAEMON, NDLP_DEBUG, __FILE__, __FUNCTION__, __LINE__, "BEGIN --\n%s\n-- END", buffer_tostring(__wb)); \
        buffer_free(__wb); \
    } \
} while(0)

// For internal_fatal with dictionary stacktrace
#define dictionary_internal_fatal(condition, dict, fmt, args...) do { \
    if(unlikely(condition)) { \
        BUFFER *__wb = buffer_create(1024, NULL); \
        if ((dict) && ((dict)->stacktraces.num_stacktraces > 0)) { \
            buffer_sprintf(__wb, fmt " Dictionary accessed at:\n", ##args); \
            for (int i = 0; i < (dict)->stacktraces.num_stacktraces && i < 3; i++) { \
                if ((dict)->stacktraces.stacktraces[i]) { \
                    buffer_sprintf(__wb, "Stacktrace #%d:\n", i+1); \
                    stacktrace_to_buffer((dict)->stacktraces.stacktraces[i], __wb); \
                    buffer_strcat(__wb, "\n"); \
                } \
            } \
            if ((dict)->stacktraces.num_stacktraces > 3) \
                buffer_sprintf(__wb, "...and %d more stacktraces\n", (dict)->stacktraces.num_stacktraces - 3); \
        } else { \
            buffer_sprintf(__wb, fmt " Dictionary stacktrace not available", ##args); \
        } \
        netdata_logger_fatal(__FILE__, __FUNCTION__, __LINE__, "BEGIN --\n%s\n--END", buffer_tostring(__wb)); \
        buffer_free(__wb); \
    } \
} while(0)

#else

#define dictionary_debug_init(void) debug_dummy()
#define dictionary_debug_track_dict(dict) debug_dummy()
#define dictionary_debug_untrack_dict(dict) debug_dummy()
#define dictionary_print_still_allocated_stacktraces(void) debug_dummy()
#define dictionary_debug_print_delayed_dictionaries(destroyed_dicts) debug_dummy()
#define dictionary_debug_shutdown(void) debug_dummy()

#define dictionary_debug_internal_check(dict, item, allow_null_dict, allow_null_item) debug_dummy()
#define dictionary_debug_internal_check_with_trace(dict, item, function, allow_null_dict, allow_null_item) debug_dummy()

#define dictionary_internal_error(condition, dict, fmt, args...) debug_dummy()
#define dictionary_internal_fatal(condition, dict, fmt, args...) debug_dummy()
#endif

#endif // NETDATA_DICTIONARY_DEBUG_H
