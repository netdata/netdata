// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

bool nd_log_forked = false;

#define NO_STACK_TRACE_PREFIX "info: stack trace is not available, "

// The signal handler function name to filter out in stack traces
static const char *signal_handler_function = "nd_signal_handler";

// List of auxiliary functions that should not be reported as root cause
static const char *auxiliary_functions[] = {
    "nd_uuid_copy",
    "out_of_memory",
    "shutdown_timed_out",
    NULL  // Terminator
};

// List of logging functions to filter out
static const char *logging_functions[] = {
    "netdata_logger",
    "netdata_logger_with_limit",
    "netdata_logger_fatal",
    NULL  // Terminator
};

// Set the signal handler function name to filter out in stack traces
void capture_stack_trace_set_signal_handler_function(const char *function_name) {
    signal_handler_function = function_name;
}

// Exact match check if a function is in the auxiliary list (strcmp)
static inline bool is_auxiliary_function(const char *function) {
    if (!function || !*function)
        return false;
        
    for (int i = 0; auxiliary_functions[i]; i++) {
        if (strcmp(function, auxiliary_functions[i]) == 0)
            return true;
    }
    
    return false;
}

// Exact match check if a function is a logging function (strcmp)
static inline bool is_logging_function(const char *function) {
    if (!function || !*function)
        return false;
        
    for (int i = 0; logging_functions[i]; i++) {
        if (strcmp(function, logging_functions[i]) == 0)
            return true;
    }
    
    return false;
}

// Substring check if a function contains a logging function name (strstr)
static inline bool contains_logging_function(const char *text) {
    if (!text || !*text)
        return false;
        
    for (int i = 0; logging_functions[i]; i++) {
        if (strstr(text, logging_functions[i]) != NULL)
            return true;
    }
    
    return false;
}

static inline bool is_netdata_function(const char *function, const char *filename) {
    return function && *function && filename && *filename &&
           strstr(filename, "/src/") &&
           !strstr(filename, "/vendored/");
}

// Exact match check if a function is the signal handler (strcmp)
static inline bool is_signal_handler_function(const char *function) {
    return function && *function &&
           signal_handler_function && *signal_handler_function &&
           strcmp(function, signal_handler_function) == 0;
}

// Substring check if a function contains the signal handler name (strstr)
static inline bool contains_signal_handler_function(const char *text) {
    return text && *text &&
           signal_handler_function && *signal_handler_function &&
           strstr(text, signal_handler_function) != NULL;
}

// Thread-local buffer to store the first netdata function encountered in a stack trace
static __thread char root_cause_function[48];

// Returns the first netdata function found in the stack trace
const char *capture_stack_trace_root_cause_function(void) {
    return root_cause_function[0] ? root_cause_function : NULL;
}

// Store a function name as the first netdata function found
static inline void keep_first_root_cause_function(const char *function) {
    if (!function || !*function || root_cause_function[0])
        return;  // Already have a function or null input
    
    // Skip auxiliary functions and logging functions   
    if (is_auxiliary_function(function) || is_logging_function(function))
        return;

    strncpyz(root_cause_function, function, sizeof(root_cause_function) - 1);
}

#if defined(HAVE_LIBBACKTRACE)
#include "backtrace-supported.h"
#endif

#if defined(HAVE_LIBBACKTRACE) && BACKTRACE_SUPPORTED == 1 /* && BACKTRACE_SUPPORTS_THREADS == 1 */
#include "backtrace.h"

static struct backtrace_state *backtrace_state = NULL;

typedef struct {
    BUFFER *wb;             // Buffer to write to
    size_t frame_count;     // Number of frames processed
    bool first_frame;       // Is this the first frame?
    bool found_signal_handler;  // Have we found the signal handler frame?
} backtrace_data_t;

// Common function to format and add a stack frame to the buffer
static void add_stack_frame(backtrace_data_t *bt_data, uintptr_t pc, const char *function,
                            const char *filename, int lineno) {
    BUFFER *wb = bt_data->wb;

    if (!wb)
        return;

    // Check if we found the signal handler frame
    if (!bt_data->found_signal_handler && is_signal_handler_function(function)) {
        // We found the signal handler, reset the buffer and clear function name
        buffer_flush(wb);
        bt_data->frame_count = 0;
        bt_data->first_frame = true;
        bt_data->found_signal_handler = true;
        root_cause_function[0] = '\0';
        return; // Skip adding the signal handler itself
    }
    
    // Check for logging functions, but only if we haven't found a signal handler yet
    // This prevents double resets when crashing inside logging code
    if (!bt_data->found_signal_handler && is_logging_function(function)) {
        // Found a logging function, reset the buffer and clear function name
        buffer_flush(wb);
        bt_data->frame_count = 0;
        bt_data->first_frame = true;
        root_cause_function[0] = '\0';
        // continue to add the function to the stack trace
    }

    // Check if this is a netdata source file and store the function name if it is
    // (but only if we haven't already stored one)
    if (!root_cause_function[0] && is_netdata_function(function, filename))
        keep_first_root_cause_function(function);

    // Add a newline between frames
    if (!bt_data->first_frame)
        buffer_putc(wb, '\n');
    else
        bt_data->first_frame = false;

    // Format: #ID function (filename.c:NNN)
    buffer_putc(wb, '#');
    buffer_print_uint64(wb, bt_data->frame_count);
    buffer_putc(wb, ' ');

    if (function && *function)
        buffer_strcat(wb, function);
    else
        buffer_strcat(wb, "<unknown>");

    if(pc) {
        buffer_strcat(wb, " [");
        buffer_print_uint64_hex(wb, pc);
        buffer_putc(wb, ']');
    }

    if (filename && *filename) {
        buffer_strcat(wb, " (");

        const char *f = strstr(filename, "/src/");
        if (f) {
            const char *f2 = strstr(f + 1, "/src/");
            if(f2) f = f2;
        }
        if(!f) f = filename;

        buffer_strcat(wb, f);

        if (lineno > 0) {
            buffer_strcat(wb, ":");
            buffer_print_uint64(wb, (uint64_t)lineno);
        }

        buffer_putc(wb, ')');
    }

    bt_data->frame_count++;
}

// Error callback for libbacktrace
static void bt_error_handler(void *data, const char *msg, int errnum) {
    backtrace_data_t *bt_data = (backtrace_data_t *)data;

    if (!bt_data || !bt_data->wb)
        return;

    // Use <unknown> for function name in error cases
    const char *function = "<unknown>";

    // Format the error message as the filename
    char error_buf[512] = "error: ";
    size_t len = 7; // Length of "error: "

    // Add the error message
    if (msg)
        len = strcatz(error_buf, len, msg, sizeof(error_buf));

    // Add the error number description if available
    if (errnum > 0) {
        if (msg) {
            len = strcatz(error_buf, len, ": ", sizeof(error_buf));
        }
        len = strcatz(error_buf, len, strerror(errnum), sizeof(error_buf));
    }

    add_stack_frame(bt_data, 0, function, error_buf, 0);
}

// Full callback for libbacktrace
static int bt_full_handler(void *data, uintptr_t pc,
                           const char *filename, int lineno,
                           const char *function) {
    backtrace_data_t *bt_data = (backtrace_data_t *)data;
    if (!bt_data)
        return 0;

    add_stack_frame(bt_data, pc, function, filename, lineno);

    return 0; // Continue backtrace
}

const char *capture_stack_trace_backend(void) {
#if BACKTRACE_SUPPORTS_DATA
#define BACKTRACE_DATA "data"
#else
#define BACKTRACE_DATA "no-data"
#endif

#if BACKTRACE_USES_MALLOC
#define BACKTRACE_MEMORY "malloc"
#else
#define BACKTRACE_MEMORY "mmap"
#endif

#if BACKTRACE_SUPPORTS_THREADS
#define BACKTRACE_THREADS "threads"
#else
#define BACKTRACE_THREADS "no-threads"
#endif

    return "libbacktrace (" BACKTRACE_MEMORY ", " BACKTRACE_THREADS ", " BACKTRACE_DATA ")";
}

void capture_stack_trace_init(void) {
    if (!backtrace_state) {
        backtrace_state = backtrace_create_state(NULL, BACKTRACE_SUPPORTS_THREADS,
                                                 NULL, // We'll handle errors in bt_full_handler
                                                 NULL);
    }
}

void capture_stack_trace_flush(void) {
    // Nothing to flush with libbacktrace
}

bool capture_stack_trace_is_async_signal_safe(void) {
// libbacktrace may use malloc depending on configuration
// Check the BACKTRACE_USES_MALLOC define
#if BACKTRACE_USES_MALLOC
    return false;
#else
    return true;
#endif
}

bool capture_stack_trace_available(void) {
    return backtrace_state != NULL && BACKTRACE_SUPPORTED;
}

NEVER_INLINE
void capture_stack_trace(BUFFER *wb) {
    if (!backtrace_state) {
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "libbacktrace not initialized");
        return;
    }

    backtrace_data_t bt_data = {
        .wb = wb,
        .frame_count = 0,
        .first_frame = true,
        .found_signal_handler = false
    };

    // Skip one frame to hide capture_stack_trace() itself
    backtrace_full(backtrace_state, 0, bt_full_handler,
                   bt_error_handler, &bt_data);

    // If no frames were reported
    if (bt_data.frame_count == 0) {
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "libbacktrace reports no frames");
    }
}

#elif defined(HAVE_LIBUNWIND)
#if !defined(STATIC_BUILD)
#define UNW_LOCAL_ONLY
#endif
#include <libunwind.h>

const char *capture_stack_trace_backend(void) {
    return "libunwind";
}

void capture_stack_trace_init(void) {
    unw_set_caching_policy(unw_local_addr_space, UNW_CACHE_NONE);
}

void capture_stack_trace_flush(void) {
    unw_flush_cache(unw_local_addr_space, 0, 0);
}

bool capture_stack_trace_is_async_signal_safe(void) {
#if defined(STATIC_BUILD)
    return false;
#else
    return true;
#endif
}

bool capture_stack_trace_available(void) {
    return true;
}

NEVER_INLINE
void capture_stack_trace(BUFFER *wb) {
    // this function is async-signal-safe, if the buffer has enough space to hold the stack trace

    unw_cursor_t cursor;
    unw_context_t context;
    size_t frames = 0;

    // Initialize context for current thread
    unw_getcontext(&context);
    unw_init_local(&cursor, &context);

    size_t added = 0;
    bool found_signal_handler = false;

    while (unw_step(&cursor) > 0) {
        unw_word_t offset, pc;
        char sym[256];

        unw_get_reg(&cursor, UNW_REG_IP, &pc);
        if (!pc)
            break;

        const char *name = sym;
        if (unw_get_proc_name(&cursor, sym, sizeof(sym), &offset) != 0) {
            name = "<unknown>";
            offset = 0;
        }

        // Check if we found the signal handler frame
        if (!found_signal_handler && is_signal_handler_function(name)) {
            // We found the signal handler, reset the buffer
            buffer_flush(wb);
            added = 0;
            frames = 0;
            found_signal_handler = true;
            continue; // Skip adding the signal handler itself
        }
        
        // Check for logging functions, but only if we haven't found a signal handler yet
        if (!found_signal_handler && is_logging_function(name)) {
            // Found a logging function, reset the buffer
            buffer_flush(wb);
            added = 0;
            frames = 0;
            // continue to add the function to the stack trace
        }

        if (frames++)
            buffer_putc(wb, '\n');

        buffer_putc(wb, '#');
        buffer_print_uint64(wb, added);
        buffer_putc(wb, ' ');
        buffer_strcat(wb, name);

        if(offset) {
            buffer_putc(wb, '+');
            buffer_print_uint64_hex(wb, offset);
        }

        added++;
    }

    if (!added)
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "libunwind reports no frames");
}

#elif defined(HAVE_BACKTRACE)

const char *capture_stack_trace_backend(void) {
    return "backtrace";
}

bool capture_stack_trace_available(void) {
    return true;
}

void capture_stack_trace_init(void) {
    ;
}

void capture_stack_trace_flush(void) {
    ;
}

bool capture_stack_trace_is_async_signal_safe(void) {
    return false;
}

NEVER_INLINE
void capture_stack_trace(BUFFER *wb) {
    void *array[50];
    char **messages;
    int size, i;

    size = backtrace(array, _countof(array));
    messages = backtrace_symbols(array, size);

    if (!messages) {
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "backtrace() reports no symbols");
        return;
    }

    size_t added = 0;
    bool found_signal_handler = false;

    // Format the stack trace (removing the address part)
    for (i = 0; i < size; i++) {
        if(messages[i] && *messages[i]) {
            // Check if we found the signal handler frame
            if (!found_signal_handler && contains_signal_handler_function(messages[i])) {
                // We found the signal handler, reset the buffer
                buffer_flush(wb);
                added = 0;
                found_signal_handler = true;
                continue; // Skip adding the signal handler itself
            }
            
            // Check for logging functions, but only if we haven't found a signal handler yet
            if (!found_signal_handler && contains_logging_function(messages[i])) {
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

#else

const char *capture_stack_trace_backend(void) {
    return "none";
}

bool capture_stack_trace_available(void) {
    return false;
}

void capture_stack_trace_init(void) {
    ;
}

void capture_stack_trace_flush(void) {
    ;
}

bool capture_stack_trace_is_async_signal_safe(void) {
    return false;
}

NEVER_INLINE
void capture_stack_trace(BUFFER *wb) {
    buffer_strcat(wb, NO_STACK_TRACE_PREFIX "no back-end available");

    // probably we can have something like this?
    // https://maskray.me/blog/2022-04-09-unwinding-through-signal-handler
    // (at the end - but it needs the frame pointer)
}

#endif

NEVER_INLINE
bool stack_trace_formatter(BUFFER *wb, void *data __maybe_unused) {
    static __thread bool in_stack_trace = false;

    if (nd_log_forked) {
        // libunwind freezes in forked children
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "stack trace after fork is disabled");
        return true;
    }

    if (in_stack_trace) {
        // Prevent recursion
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "stack trace recursion detected");
        return true;
    }

    in_stack_trace = true;

    root_cause_function[0] = '\0';
    capture_stack_trace(wb);

    in_stack_trace = false; // Ensure the flag is reset
    return true;
}
