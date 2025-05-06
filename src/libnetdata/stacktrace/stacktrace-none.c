// SPDX-License-Identifier: GPL-3.0-or-later

#include "stacktrace-common.h"

// This implementation is used when no stacktrace backend is available
#if defined(USE_NOTRACE)

const char *stacktrace_backend(void) {
    return "none";
}

bool stacktrace_available(void) {
    return false;
}

void impl_stacktrace_init(void) {
    // Nothing to initialize for null backend
}

void stacktrace_flush(void) {
    // Nothing to flush
}

bool stacktrace_capture_is_async_signal_safe(void) {
    return false;
}

NEVER_INLINE
void stacktrace_capture(BUFFER *wb) {
    root_cause_function[0] = '\0';

    buffer_strcat(wb, NO_STACK_TRACE_PREFIX "no back-end available");

    // probably we can have something like this?
    // https://maskray.me/blog/2022-04-09-unwinding-through-signal-handler
    // (at the end - but it needs the frame pointer)
}

// Simple dummy implementation for platforms without stack trace support
int impl_stacktrace_get_frames(void **frames, int max_frames, int skip_frames) {
    if (!frames || max_frames <= 0)
        return 0;
    
    // No need to adjust skip_frames here as we're just creating a dummy frame
    // but we include the comment for consistency with other implementations
    // skip_frames += 1; // Skip this function itself
    
    // Just use a counter to create a unique "frame"
    static uint64_t counter = 0;
    uint64_t id = __atomic_fetch_add(&counter, 1, __ATOMIC_SEQ_CST);
    
    // Store one dummy frame
    frames[0] = (void *)(uintptr_t)id;
    
    return 1;
}

void impl_stacktrace_to_buffer(STACKTRACE trace, BUFFER *wb) {
    struct stacktrace *st = (struct stacktrace *)trace;
    
    // Simple representation for platforms without stack trace support
    buffer_sprintf(wb, NO_STACK_TRACE_PREFIX "no back-end available (id: %" PRIu64 ")", st->hash);
}

#endif // USE_NOTRACE