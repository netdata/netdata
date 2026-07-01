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
    if (unlikely(num_frames <= 0))
        return NULL;

    size_t extra_frames = (size_t)num_frames - 1;
    if (unlikely(extra_frames > (SIZE_MAX - sizeof(struct stacktrace)) / sizeof(void *)))
        return NULL;

    size_t size = sizeof(struct stacktrace) + extra_frames * sizeof(void *);
    struct stacktrace *trace = callocz(1, size);
    trace->frame_count = num_frames;
    return trace;
}

static bool stacktrace_frames_equal(const struct stacktrace *trace, const void *frames, int num_frames) {
    return trace && trace->frame_count == num_frames &&
           memcmp(trace->frames, frames, (size_t)num_frames * sizeof(void *)) == 0;
}

// The global cache caller holds stacktrace_lock; unit tests use a private cache.
static struct stacktrace *stacktrace_cache_get_or_create(
    STACKTRACE_JudyLSet *cache,
    uint64_t hash,
    const void *frames,
    int num_frames)
{
    if (unlikely(!cache || !frames || num_frames <= 0))
        return NULL;

    Word_t key = (Word_t)hash;
    struct stacktrace *last = NULL;

    for (struct stacktrace *trace = STACKTRACE_GET(cache, key); trace; trace = trace->hash_next) {
        if (stacktrace_frames_equal(trace, frames, num_frames))
            return trace;

        last = trace;
    }

    struct stacktrace *trace = stacktrace_create(num_frames);
    if (unlikely(!trace))
        return NULL;

    trace->hash = hash;
    memcpy(trace->frames, frames, (size_t)num_frames * sizeof(void *));

    if (last)
        last->hash_next = trace;
    else if (!STACKTRACE_SET(cache, key, trace)) {
        freez(trace);
        return NULL;
    }

    return trace;
}

static void stacktrace_free_chain(Word_t index __maybe_unused, struct stacktrace *trace, void *data __maybe_unused) {
    while (trace) {
        struct stacktrace *next = trace->hash_next;
        freez(trace->text);
        freez(trace);
        trace = next;
    }
}

int stacktrace_cache_unittest(void) {
    STACKTRACE_JudyLSet cache;
    STACKTRACE_INIT(&cache);

    int frame_a0, frame_a1, frame_b1;
    void *frames_a[] = { &frame_a0, &frame_a1 };
    void *frames_a_again[] = { &frame_a0, &frame_a1 };
    void *frames_b[] = { &frame_a0, &frame_b1 };
    uint64_t hash = UINT64_C(0x1122334455667788);

    struct stacktrace *first = stacktrace_cache_get_or_create(&cache, hash, frames_a, _countof(frames_a));
    struct stacktrace *second = stacktrace_cache_get_or_create(&cache, hash, frames_b, _countof(frames_b));
    struct stacktrace *first_again = stacktrace_cache_get_or_create(&cache, hash, frames_a_again, _countof(frames_a_again));

    size_t chain_length = 0;
    for (struct stacktrace *trace = STACKTRACE_GET(&cache, (Word_t)hash); trace; trace = trace->hash_next)
        chain_length++;

    bool success = first && second &&
                   first != second &&
                   first_again == first &&
                   first->hash == hash &&
                   second->hash == hash &&
                   chain_length == 2;

    STACKTRACE_FREE(&cache, stacktrace_free_chain, NULL);

    return success ? 0 : 1;
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
    if (unlikely(skip_frames < 0 || skip_frames > INT_MAX - 2))
        return NULL;

    // Make sure cache is initialized
    stacktrace_cache_init();
    
    // Get the frames from the implementation
    void *frames[50] = {0};
    // Add 1 to skip_frames to also skip stacktrace_get() itself
    int num_frames = impl_stacktrace_get_frames(frames, 50, skip_frames + 1);
    
    if (num_frames <= 0 || num_frames > (int)_countof(frames))
        return NULL;
    
    // Calculate hash
    uint64_t hash = XXH3_64bits(frames, num_frames * sizeof(void *));
    
    spinlock_lock(&stacktrace_lock);
    struct stacktrace *trace = stacktrace_cache_get_or_create(&stacktrace_cache, hash, frames, num_frames);
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
