// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STACKTRACE_COMMON_H
#define NETDATA_STACKTRACE_COMMON_H 1

#include "libnetdata/libnetdata.h"
#include "stacktrace.h"
#include "../libjudy/judyl-typed.h"

#define NO_STACK_TRACE_PREFIX STACK_TRACE_INFO_PREFIX "stack trace is not available, "

#if defined(HAVE_LIBBACKTRACE)
#include "backtrace-supported.h"
#if BACKTRACE_SUPPORTED == 1
#define USE_LIBBACKTRACE 1
#endif
#endif

#if !defined(USE_LIBBACKTRACE) && defined(HAVE_LIBUNWIND)
#define USE_LIBUNWIND 1
#endif

#if !defined(USE_LIBBACKTRACE) && !defined(USE_LIBUNWIND) && defined(HAVE_BACKTRACE)
#define USE_BACKTRACE 1
#endif

#if !defined(USE_LIBBACKTRACE) && !defined(USE_LIBUNWIND) && !defined(USE_BACKTRACE) && !defined(HAVE_BACKTRACE)
#define USE_NOTRACE 1
#endif

// The structure for stacktrace storage
struct stacktrace {
    uint64_t hash;          // Hash of the stack trace
    char *text;             // Text representation (cached, lazy-initialized)
    int frame_count;        // Number of frames
    void *frames[1];        // Variable-length array of frame pointers
};

// Cache for storing stack traces
DEFINE_JUDYL_TYPED(STACKTRACE, struct stacktrace *);
extern STACKTRACE_JudyLSet stacktrace_cache;
extern SPINLOCK stacktrace_lock;
extern bool cache_initialized;

// Filter names
extern const char *signal_handler_function;
extern const char *auxiliary_functions[];
extern const char *logging_functions[];

// Thread-local storage for root cause
extern __thread char root_cause_function[48];

// Common helper functions
void stacktrace_cache_init(void);
struct stacktrace *stacktrace_create(int num_frames);
bool stacktrace_is_auxiliary_function(const char *function);
bool stacktrace_is_logging_function(const char *function);
bool stacktrace_contains_logging_function(const char *text);
bool stacktrace_is_netdata_function(const char *function, const char *filename);
bool stacktrace_is_signal_handler_function(const char *function);
bool stacktrace_contains_signal_handler_function(const char *text);
void stacktrace_keep_first_root_cause_function(const char *function);

// Implementation-specific declarations
void impl_stacktrace_init(void);
int impl_stacktrace_get_frames(void **frames, int max_frames, int skip_frames);
void impl_stacktrace_to_buffer(STACKTRACE trace, BUFFER *wb);

#endif /* NETDATA_STACKTRACE_COMMON_H */