// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STACKTRACE_H
#define NETDATA_STACKTRACE_H 1

#include "libnetdata/common.h"

#define STACK_TRACE_INFO_PREFIX "info: "

// Opaque pointer to a stack trace
typedef struct stacktrace *STACKTRACE;

// Set the signal handler function name to filter out in stack traces
void stacktrace_set_signal_handler_function(const char *function_name);

// Returns the first netdata function found in the stack trace
const char *stacktrace_root_cause_function(void);

// Initialize the stacktrace capture mechanism
void stacktrace_init(void);

// Free any resources used by the stacktrace mechanism
void stacktrace_flush(void);

// Return true if the stacktrace mechanism can be safely used in signal handlers
bool stacktrace_capture_is_async_signal_safe(void);

// Return true if stacktrace capture is available on this platform
bool stacktrace_available(void);

// Capture a stacktrace to a buffer
struct web_buffer;
void stacktrace_capture(BUFFER *wb);

// Return a string describing the backend used for capturing stacktraces
const char *stacktrace_backend(void);

// Get the current stacktrace, hash it, and store it in a cache
STACKTRACE stacktrace_get(int skip_frames);

// Convert a stacktrace to a buffer
void stacktrace_to_buffer(STACKTRACE trace, struct web_buffer *wb);

void stacktrace_forked(void);
bool stack_trace_formatter(struct web_buffer *wb, void *data);

// Unit testing
int stacktrace_unittest(void);

#endif /* NETDATA_STACKTRACE_H */
