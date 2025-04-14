// SPDX-License-Identifier: GPL-3.0-or-later

#include "stacktrace-common.h"

#if defined(USE_BACKTRACE)
#include <execinfo.h>

const char *stacktrace_backend(void) {
    return "backtrace";
}

bool stacktrace_available(void) {
    return true;
}

void impl_stacktrace_init(void) {
    // Nothing to initialize for backtrace backend
}

void stacktrace_flush(void) {
    // Nothing to flush
}

bool stacktrace_capture_is_async_signal_safe(void) {
    return false;
}

NEVER_INLINE
void stacktrace_capture(BUFFER *wb) {
    void *array[50];
    char **messages;
    int size, i;

    root_cause_function[0] = '\0';

    size = backtrace(array, _countof(array));
    messages = backtrace_symbols(array, size);

    if (!messages) {
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "backtrace() reports no symbols");
        return;
    }

    size_t added = 0;
    bool found_signal_handler = false;

    // Format the stack trace (removing the address part)
    // Skip the first frame (stacktrace_capture itself)
    for (i = 1; i < size; i++) {
        if(messages[i] && *messages[i]) {
            // Check if we found the signal handler frame
            if (!found_signal_handler && stacktrace_contains_signal_handler_function(messages[i])) {
                // We found the signal handler, reset the buffer
                buffer_flush(wb);
                added = 0;
                found_signal_handler = true;
                continue; // Skip adding the signal handler itself
            }
            
            // Check for logging functions, but only if we haven't found a signal handler yet
            if (!found_signal_handler && stacktrace_contains_logging_function(messages[i])) {
                // Found a logging function, reset the buffer
                buffer_flush(wb);
                added = 0;
                // continue to add the function to the stack trace
            }

            if(added)
                buffer_putc(wb, '\n');

            buffer_putc(wb, '#');
            buffer_print_uint64(wb, added);
            buffer_putc(wb, ' ');
            buffer_strcat(wb, messages[i]);
            added++;
        }
    }

    if(!added)
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "backtrace() reports no frames");

    free(messages);
}

// Implementation-specific function to collect stack trace frames
int impl_stacktrace_get_frames(void **frames, int max_frames, int skip_frames) {
    if (!frames || max_frames <= 0)
        return 0;
    
    // Add 1 to skip_frames to also skip this function itself
    skip_frames += 1;
    
    // Collect all stack frames without skipping at this level
    void *array[100 + 50]; // Use a larger array to account for skipped frames (100) plus the max we need (50)
    int size = backtrace(array, _countof(array));
    
    if (size <= skip_frames) // Not enough frames after skipping
        return 0;
    
    // Apply skip_frames here, in the aftermath of backtrace, not during capture
    // This ensures we're not skipping inlined functions
    int available_frames = size - skip_frames;
    int frames_to_copy = available_frames < max_frames ? available_frames : max_frames;
    
    // Copy the frames, skipping the requested number of frames
    memcpy(frames, array + skip_frames, frames_to_copy * sizeof(void *));
    
    return frames_to_copy;
}

// Implementation-specific function to convert a stacktrace to a buffer
void impl_stacktrace_to_buffer(STACKTRACE trace, BUFFER *wb) {
    struct stacktrace *st = (struct stacktrace *)trace;
    
    // Convert to text representation
    char **messages = backtrace_symbols(st->frames, st->frame_count);
    
    if (!messages) {
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "backtrace_symbols() failed");
        return;
    }
    
    bool found_signal_handler = false;
    int added = 0;
    
    for (int i = 0; i < st->frame_count; i++) {
        if (!messages[i] || !*messages[i])
            continue;
            
        // Handle filtering
        if (!found_signal_handler && stacktrace_contains_signal_handler_function(messages[i])) {
            buffer_flush(wb);
            added = 0;
            found_signal_handler = true;
            continue;
        }
        
        if (!found_signal_handler && stacktrace_contains_logging_function(messages[i])) {
            buffer_flush(wb);
            added = 0;
        }
        
        if (added > 0)
            buffer_putc(wb, '\n');
            
        buffer_putc(wb, '#');
        buffer_print_uint64(wb, added);
        buffer_putc(wb, ' ');
        buffer_strcat(wb, messages[i]);
        added++;
    }
    
    free(messages);
    
    if (added == 0)
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "no valid frames");
}

#endif // USE_BACKTRACE