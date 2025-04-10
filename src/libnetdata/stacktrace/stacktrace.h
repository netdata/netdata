// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STACKTRACE_H
#define NETDATA_STACKTRACE_H 1

#include "../libnetdata.h"

#define STACK_TRACE_INFO_PREFIX "info: "

// Opaque pointer to a stack trace
typedef struct stacktrace *STACKTRACE;

// Set the signal handler function name to filter out in stack traces
void capture_stack_trace_set_signal_handler_function(const char *function_name);

// Returns the first netdata function found in the stack trace
const char *capture_stack_trace_root_cause_function(void);

// Initialize the stacktrace capture mechanism
void capture_stack_trace_init(void);

// Free any resources used by the stacktrace mechanism
void capture_stack_trace_flush(void);

// Return true if the stacktrace mechanism can be safely used in signal handlers
bool capture_stack_trace_is_async_signal_safe(void);

// Return true if stacktrace capture is available on this platform
bool capture_stack_trace_available(void);

// Capture a stacktrace to a buffer
void capture_stack_trace(BUFFER *wb);

// Return a string describing the backend used for capturing stacktraces
const char *capture_stack_trace_backend(void);

// New API functions
// Get the current stacktrace, hash it, and store it in a cache
STACKTRACE stacktrace_get(void);

// Convert a stacktrace to a buffer
void stacktrace_to_buffer(STACKTRACE trace, BUFFER *wb);

#endif /* NETDATA_STACKTRACE_H */
