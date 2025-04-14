// SPDX-License-Identifier: GPL-3.0-or-later

#include "stacktrace-common.h"

// Stacktrace cache
STACKTRACE_JudyLSet stacktrace_cache;
SPINLOCK stacktrace_lock = SPINLOCK_INITIALIZER;
bool cache_initialized = false;

// The signal handler function name to filter out in stack traces
const char *signal_handler_function = "nd_signal_handler";

// List of auxiliary functions that should not be reported as root cause
const char *auxiliary_functions[] = {
    "nd_uuid_copy",
    "out_of_memory",
    "shutdown_timed_out",
    NULL  // Terminator
};

// List of logging functions to filter out
const char *logging_functions[] = {
    "netdata_logger",
    "netdata_logger_with_limit",
    "netdata_logger_fatal",
    NULL  // Terminator
};

// Thread-local buffer to store the first netdata function encountered in a stack trace
__thread char root_cause_function[48];

// Set the signal handler function name to filter out in stack traces
void stacktrace_set_signal_handler_function(const char *function_name) {
    signal_handler_function = function_name;
}

// Returns the first netdata function found in the stack trace
const char *stacktrace_root_cause_function(void) {
    return root_cause_function[0] ? root_cause_function : NULL;
}

// Initialize the stacktrace cache
void stacktrace_cache_init(void) {
    if (cache_initialized)
        return;

    spinlock_lock(&stacktrace_lock);
    if (!cache_initialized) {
        STACKTRACE_INIT(&stacktrace_cache);
        cache_initialized = true;
    }
    spinlock_unlock(&stacktrace_lock);
}

// Main initialization function - this ensures the cache is always initialized
void stacktrace_init(void) {
    // Always initialize the cache
    stacktrace_cache_init();
    
    // Call the backend-specific initialization
    impl_stacktrace_init();
}

// Allocate a new stacktrace structure with the given number of frames
struct stacktrace *stacktrace_create(int num_frames) {
    size_t size = sizeof(struct stacktrace) + ((num_frames - 1) * sizeof(void *));
    struct stacktrace *trace = callocz(1, size);
    trace->frame_count = num_frames;
    return trace;
}

// Exact match check if a function is in the auxiliary list (strcmp)
bool stacktrace_is_auxiliary_function(const char *function) {
    if (!function || !*function)
        return false;
        
    for (int i = 0; auxiliary_functions[i]; i++) {
        if (strcmp(function, auxiliary_functions[i]) == 0)
            return true;
    }
    
    return false;
}

// Exact match check if a function is a logging function (strcmp)
bool stacktrace_is_logging_function(const char *function) {
    if (!function || !*function)
        return false;
        
    for (int i = 0; logging_functions[i]; i++) {
        if (strcmp(function, logging_functions[i]) == 0)
            return true;
    }
    
    return false;
}

// Substring check if a function contains a logging function name (strstr)
bool stacktrace_contains_logging_function(const char *text) {
    if (!text || !*text)
        return false;
        
    for (int i = 0; logging_functions[i]; i++) {
        if (strstr(text, logging_functions[i]) != NULL)
            return true;
    }
    
    return false;
}

bool stacktrace_is_netdata_function(const char *function, const char *filename) {
    return function && *function && filename && *filename &&
           strstr(filename, "/src/") &&
           !strstr(filename, "/vendored/") &&
           !stacktrace_contains_logging_function(function);
}

// Exact match check if a function is the signal handler (strcmp)
bool stacktrace_is_signal_handler_function(const char *function) {
    return function && *function &&
           signal_handler_function && *signal_handler_function &&
           strcmp(function, signal_handler_function) == 0;
}

// Substring check if a function contains the signal handler name (strstr)
bool stacktrace_contains_signal_handler_function(const char *text) {
    return text && *text &&
           signal_handler_function && *signal_handler_function &&
           strstr(text, signal_handler_function) != NULL;
}

// Store a function name as the first netdata function found
void stacktrace_keep_first_root_cause_function(const char *function) {
    if (!function || !*function || root_cause_function[0])
        return;  // Already have a function or null input
    
    // Skip auxiliary functions and logging functions   
    if (stacktrace_is_auxiliary_function(function) || stacktrace_is_logging_function(function))
        return;

    strncpyz(root_cause_function, function, sizeof(root_cause_function) - 1);
}

// Get the current stacktrace - public API
NEVER_INLINE
STACKTRACE stacktrace_get(int skip_frames) {
    // Make sure cache is initialized
    stacktrace_cache_init();
    
    // Get the frames from the implementation
    void *frames[50] = {0};
    // Add 1 to skip_frames to also skip stacktrace_get() itself
    int num_frames = impl_stacktrace_get_frames(frames, 50, skip_frames + 1);
    
    if (num_frames <= 0)
        return NULL;
    
    // Calculate hash
    uint64_t hash = XXH3_64bits(frames, num_frames * sizeof(void *));
    
    // Look up in cache first
    spinlock_lock(&stacktrace_lock);
    
    struct stacktrace *trace = STACKTRACE_GET(&stacktrace_cache, hash);
    
    // If existing trace found, verify it's the same frames
    // This handles hash collisions
    if (trace) {
        if (trace->frame_count != num_frames || 
            memcmp(trace->frames, frames, num_frames * sizeof(void *)) != 0) {
            // Hash collision - use linear probing
            int i = 1;
            uint64_t new_hash = hash;
            do {
                new_hash = hash + i;
                trace = STACKTRACE_GET(&stacktrace_cache, new_hash);
                
                if (!trace || 
                    (trace->frame_count == num_frames && 
                     memcmp(trace->frames, frames, num_frames * sizeof(void *)) == 0)) {
                    break; // Either found a match or empty slot
                }
                i++;
            } while (i < 10);  // Limit search to avoid infinite loops
            
            hash = new_hash;
        }
    }
    
    // If not found or hash collision, create new entry
    if (!trace) {
        trace = stacktrace_create(num_frames);
        trace->hash = hash;
        memcpy(trace->frames, frames, num_frames * sizeof(void *));
        STACKTRACE_SET(&stacktrace_cache, hash, trace);
    }
    
    spinlock_unlock(&stacktrace_lock);
    
    return trace;
}

// Convert a stacktrace to a buffer - public API
void stacktrace_to_buffer(STACKTRACE trace, BUFFER *wb) {
    if (!trace || !wb) {
        if (wb)
            buffer_strcat(wb, NO_STACK_TRACE_PREFIX "invalid stacktrace");
        return;
    }
    
    struct stacktrace *st = (struct stacktrace *)trace;
    
    // If we already have cached text representation, use it
    if (st->text) {
        buffer_strcat(wb, st->text);
        return;
    }
    
    // Use the implementation-specific function for conversion
    impl_stacktrace_to_buffer(trace, wb);
    
    // Cache the text representation
    spinlock_lock(&stacktrace_lock);
    if (!st->text)
        st->text = strdupz(buffer_tostring(wb));
    spinlock_unlock(&stacktrace_lock);
}
