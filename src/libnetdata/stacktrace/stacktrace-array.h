// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STACKTRACE_ARRAY_H
#define NETDATA_STACKTRACE_ARRAY_H 1

#include "libnetdata/libnetdata.h"
#include "stacktrace.h"

// Default maximum number of stacktraces to track per array
#ifndef STACKTRACE_ARRAY_MAX_TRACES
#define STACKTRACE_ARRAY_MAX_TRACES 100
#endif

// Structure to track multiple stacktraces
typedef struct stacktrace_array {
    SPINLOCK spinlock;                   // spinlock to protect the stacktraces array
    int num_stacktraces;                 // number of stored stacktraces (0 to STACKTRACE_ARRAY_MAX_TRACES)
    STACKTRACE stacktraces[STACKTRACE_ARRAY_MAX_TRACES]; // array of stacktraces from different acquisition points
} STACKTRACE_ARRAY;

// Initialize a stacktrace array
void stacktrace_array_init(STACKTRACE_ARRAY *array);

// Add a stacktrace to an array (captures current stacktrace)
// Only adds if it's not already present
// Returns true if the stacktrace was added, false if it was already there or array is full
bool stacktrace_array_add(STACKTRACE_ARRAY *array, int skip_frames);

// Report stacktraces to a buffer
// If total_count is not NULL, it will be updated with the total number of array elements
// If brief_output is true, only summary information is included
// Returns the number of unique stacktraces reported
size_t stacktrace_array_to_buffer(STACKTRACE_ARRAY *array, BUFFER *wb, size_t *total_count, const char *prefix, bool brief_output);

#endif /* NETDATA_STACKTRACE_ARRAY_H */