// SPDX-License-Identifier: GPL-3.0-or-later

#include "dictionary-internals.h"

#if defined(FSANITIZE_ADDRESS)

// Judyl for tracking all dictionaries ever created
static SPINLOCK all_dictionaries_spinlock = SPINLOCK_INITIALIZER;
static Pvoid_t all_dictionaries = NULL;
static bool all_dictionaries_initialized = false;

// Initialize the tracking system
static void all_dictionaries_initialize(void) {
    // Only do this once
    if (all_dictionaries_initialized)
        return;
    
    spinlock_lock(&all_dictionaries_spinlock);
    if (!all_dictionaries_initialized) {
        all_dictionaries = NULL;
        all_dictionaries_initialized = true;
    }
    spinlock_unlock(&all_dictionaries_spinlock);
}

// Comparison function for sorting stacktraces by count (descending)
static int stacktrace_count_compare(const void *a, const void *b) {
    typedef struct {
        STACKTRACE stacktrace;
        Word_t count;
    } StacktraceInfo;
    
    const StacktraceInfo *sta = (const StacktraceInfo *)a;
    const StacktraceInfo *stb = (const StacktraceInfo *)b;
    
    if (stb->count > sta->count) return 1;
    if (stb->count < sta->count) return -1;
    return 0;
}

// Report all allocated dictionaries that are not part of the destroyed list
static size_t report_allocated_dictionaries(void) {
    // Ensure initialization
    if (!all_dictionaries_initialized)
        all_dictionaries_initialize();
        
    spinlock_lock(&all_dictionaries_spinlock);

    Word_t index = 0;
    Pvoid_t PValue;
    Word_t count = 0;

    // First count dictionaries
    PValue = JudyLFirst(all_dictionaries, &index, PJE0);
    while (PValue != NULL) {
        count++;
        PValue = JudyLNext(all_dictionaries, &index, PJE0);
    }

    if (count > 0) {
        fprintf(stderr, "\nASAN: ===== DICTIONARY TRACKING: Detected %lu dictionaries that are still allocated =====\n", count);
        fflush(stderr);
        
        // First, group by stacktrace
        // Key: stacktrace pointer, Value: count
        Pvoid_t stacktrace_counts = NULL;
        // Key: stacktrace pointer, Value: array of dictionary pointers
        Pvoid_t stacktrace_dictionaries = NULL;
        
        // Group dictionaries by stacktrace
        index = 0;
        PValue = JudyLFirst(all_dictionaries, &index, PJE0);
        while (PValue != NULL) {
            DICTIONARY *dict = (DICTIONARY *)index;
            STACKTRACE st = dict->stacktrace;
            
            if (st) {
                Word_t st_key = (Word_t)st;
                
                // Update count for this stacktrace
                Pvoid_t PCount;
                PCount = JudyLIns(&stacktrace_counts, st_key, PJE0);
                if (PCount) {
                    (*(Word_t*)PCount)++;
                } else {
                    *(Word_t*)PCount = 1;
                }
                
                // Add dictionary to the list for this stacktrace
                Pvoid_t PDictList;
                PDictList = JudyLGet(stacktrace_dictionaries, st_key, PJE0);
                if (!PDictList) {
                    // Create a new array (starting with size 16)
                    DICTIONARY **dict_list = (DICTIONARY **)calloc(16, sizeof(DICTIONARY*));
                    if (dict_list) {
                        dict_list[0] = dict;
                        JudyLIns(&stacktrace_dictionaries, st_key, PJE0);
                        PDictList = JudyLGet(stacktrace_dictionaries, st_key, PJE0);
                        if (PDictList) {
                            *(void**)PDictList = dict_list;
                        }
                    }
                } else {
                    // Add to existing array
                    DICTIONARY **dict_list = *(DICTIONARY***)PDictList;
                    size_t i = 0;
                    while (dict_list[i] && i < 1024) i++; // Find first empty slot, limit to 1024
                    if (i < 1024) {
                        dict_list[i] = dict;
                    }
                }
            }
            
            PValue = JudyLNext(all_dictionaries, &index, PJE0);
        }
        
        // Now build a sorted array of stacktraces
        // Use the StacktraceInfo type defined for the comparator
        typedef struct {
            STACKTRACE stacktrace;
            Word_t count;
        } StacktraceInfo;
        
        Word_t num_stacktraces = 0;
        
        // Count stacktraces
        index = 0;
        PValue = JudyLFirst(stacktrace_counts, &index, PJE0);
        while (PValue != NULL) {
            num_stacktraces++;
            PValue = JudyLNext(stacktrace_counts, &index, PJE0);
        }
        
        if (num_stacktraces > 0) {
            StacktraceInfo *stacktraces = (StacktraceInfo *)calloc(num_stacktraces, sizeof(StacktraceInfo));
            if (stacktraces) {
                // Fill array
                Word_t i = 0;
                index = 0;
                PValue = JudyLFirst(stacktrace_counts, &index, PJE0);
                while (PValue != NULL) {
                    stacktraces[i].stacktrace = (STACKTRACE)index;
                    stacktraces[i].count = *(Word_t*)PValue;
                    i++;
                    PValue = JudyLNext(stacktrace_counts, &index, PJE0);
                }
                
                // Sort by count (descending)
                qsort(stacktraces, num_stacktraces, sizeof(StacktraceInfo), stacktrace_count_compare);
                
                // Print stacktraces in order
                for (i = 0; i < num_stacktraces; i++) {
                    STACKTRACE st = stacktraces[i].stacktrace;
                    Word_t st_count = stacktraces[i].count;
                    
                    fprintf(stderr, "\n > DICTIONARY STACKTRACE GROUP %lu/%lu (count: %lu):\n", 
                            i+1, num_stacktraces, st_count);
                    fflush(stderr);
                    
                    // Print stacktrace
                    BUFFER *wb = buffer_create(16384, NULL);
                    stacktrace_to_buffer(st, wb);
                    fprintf(stderr, "%s\n", buffer_tostring(wb));
                    buffer_free(wb);
                    fflush(stderr);
                    
                    // Print dictionary pointers
                    Pvoid_t PDictList = JudyLGet(stacktrace_dictionaries, (Word_t)st, PJE0);
                    if (PDictList) {
                        DICTIONARY **dict_list = *(DICTIONARY***)PDictList;
                        fprintf(stderr, "  Dictionary pointers:");
                        int displayed = 0;
                        
                        for (int j = 0; j < 1024 && dict_list[j] && displayed < 10; j++) {
                            fprintf(stderr, " %p", dict_list[j]);
                            displayed++;
                        }
                        
                        if (st_count > 10) {
                            fprintf(stderr, " ... (plus %lu more)", st_count - 10);
                        }
                        fprintf(stderr, "\n");
                        fflush(stderr);
                    }
                }
                
                free(stacktraces);
            }
            
            // Clean up
            index = 0;
            PValue = JudyLFirst(stacktrace_dictionaries, &index, PJE0);
            while (PValue != NULL) {
                free(*(void**)PValue);
                PValue = JudyLNext(stacktrace_dictionaries, &index, PJE0);
            }
            
            JudyLFreeArray(&stacktrace_dictionaries, PJE0);
            JudyLFreeArray(&stacktrace_counts, PJE0);
        }
    }
    else {
        fprintf(stderr, "\nASAN: ===== DICTIONARY TRACKING: No allocated dictionaries found =====\n");
        fflush(stderr);
    }

    spinlock_unlock(&all_dictionaries_spinlock);

    return count;
}

/**
 * @brief Print information about dictionaries that have delayed destruction
 *
 * This function is called during shutdown to print information about dictionaries that
 * could not be destroyed because they have referenced items
 */
void dictionary_debug_print_delayed_dictionaries(size_t destroyed_dicts) {
    if (destroyed_dicts > 0) {
        fprintf(stderr, "\nASAN: ===== DICTIONARY TRACKING: %zu dictionaries with references couldn't be destroyed =====\n", 
                destroyed_dicts);
        fflush(stderr);
    }
}

// Public API functions
void dictionary_debug_init(void) {
    all_dictionaries_initialize();
}

void dictionary_debug_track_dict(DICTIONARY *dict) {
    // Make sure we're initialized
    if (!all_dictionaries_initialized)
        all_dictionaries_initialize();
        
    spinlock_lock(&all_dictionaries_spinlock);
    // No need to capture return value
    JudyLIns(&all_dictionaries, (Word_t)dict, PJE0);
    spinlock_unlock(&all_dictionaries_spinlock);
}

void dictionary_debug_untrack_dict(DICTIONARY *dict) {
    // Only attempt to untrack if we've initialized the tracking
    if (!all_dictionaries_initialized)
        return;
        
    spinlock_lock(&all_dictionaries_spinlock);
    // No need to capture return value
    JudyLDel(&all_dictionaries, (Word_t)dict, PJE0);
    spinlock_unlock(&all_dictionaries_spinlock);
}

void dictionary_print_still_allocated_stacktraces(void) {
    size_t allocated = report_allocated_dictionaries();
    if (allocated > 0) {
        fprintf(stderr, "\nASAN: ===== DICTIONARY TRACKING: Found %zu dictionaries that are still allocated but not in the destroyed list =====\n",
                allocated);
        fflush(stderr);
    }
}

void dictionary_debug_shutdown(void) {
    if (!all_dictionaries_initialized)
        return;

    spinlock_lock(&all_dictionaries_spinlock);
    JudyLFreeArray(&all_dictionaries, PJE0);
    spinlock_unlock(&all_dictionaries_spinlock);
}

void dictionary_debug_internal_check_with_trace(DICTIONARY *dict, DICTIONARY_ITEM *item, const char *function, bool allow_null_dict, bool allow_null_item) {
    if(!allow_null_dict && !dict) {
        // Create a buffer for the item's dict stacktrace
        BUFFER *wb = buffer_create(1024, NULL);
        if (item && item->dict && item->dict->stacktrace) {
            buffer_strcat(wb, "\nItem's dictionary creation stacktrace:\n");
            stacktrace_to_buffer(item->dict->stacktrace, wb);
        } else {
            buffer_strcat(wb, "\nItem's dictionary stacktrace not available");
        }

        internal_error(
            item,
            "DICTIONARY: attempted to %s() with a NULL dictionary, passing an item. %s",
            function,
            buffer_tostring(wb));

        buffer_free(wb);
        fatal("DICTIONARY: attempted to %s() but dict is NULL", function);
    }

    if(!allow_null_item && !item) {
        dictionary_internal_error(true, dict,
            "DICTIONARY: attempted to %s() without an item on a dictionary",
            function);
        fatal("DICTIONARY: attempted to %s() but item is NULL", function);
    }

    if(dict && item && dict != item->dict) {
        // Create buffer for both dictionaries' stacktraces
        BUFFER *wb = buffer_create(1024, NULL);

        if (dict->stacktrace) {
            buffer_strcat(wb, "\nDictionary stacktrace:\n");
            stacktrace_to_buffer(dict->stacktrace, wb);
        } else {
            buffer_strcat(wb, "\nDictionary stacktrace not available");
        }

        if (item->dict && item->dict->stacktrace) {
            buffer_strcat(wb, "\nItem's dictionary stacktrace:\n");
            stacktrace_to_buffer(item->dict->stacktrace, wb);
        } else {
            buffer_strcat(wb, "\nItem's dictionary stacktrace not available");
        }

        internal_error(
            true,
            "DICTIONARY: attempted to %s() an item on a dictionary different from the item's dictionary. %s",
            function,
            buffer_tostring(wb));

        buffer_free(wb);
        fatal("DICTIONARY: %s(): item does not belong to this dictionary.", function);
    }

    if(item) {
        REFCOUNT refcount = DICTIONARY_ITEM_REFCOUNT_GET(dict, item);
        if (unlikely(refcount <= 0)) {
            dictionary_internal_error(true, item->dict,
                "DICTIONARY: attempted to %s() of an item with reference counter = %d on a dictionary",
                function,
                refcount);
            fatal("DICTIONARY: attempted to %s but item is having refcount = %d", function, refcount);
        }
    }
}

#endif // FSANITIZE_ADDRESS
