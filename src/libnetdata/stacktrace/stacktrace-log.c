// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "stacktrace.h"

// External variable needed by other modules
static bool nd_log_forked = false;

void stacktrace_forked(void) {
    nd_log_forked = true;
}

// The stack trace formatter called by logger
NEVER_INLINE
bool stack_trace_formatter(BUFFER *wb, void *data __maybe_unused) {
    static __thread bool in_stack_trace = false;

    extern bool nd_log_forked;
    if (nd_log_forked) {
        // libunwind freezes in forked children
        buffer_strcat(wb, STACK_TRACE_INFO_PREFIX "stack trace is not available, stack trace after fork is disabled");
        return true;
    }

    if (in_stack_trace) {
        // Prevent recursion
        buffer_strcat(wb, STACK_TRACE_INFO_PREFIX "stack trace is not available, stack trace recursion detected");
        return true;
    }

    in_stack_trace = true;

    // Use the existing stacktrace_capture
    stacktrace_capture(wb);

    in_stack_trace = false; // Ensure the flag is reset
    return true;
}