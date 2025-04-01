// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

bool nd_log_forked = false;

#define NO_STACK_TRACE_PREFIX "info: stack trace is not available, "

#if defined(HAVE_LIBBACKTRACE)
#include "backtrace-supported.h"
#endif

#if defined(HAVE_LIBBACKTRACE) && BACKTRACE_SUPPORTED == 1 /* && BACKTRACE_SUPPORTS_THREADS == 1 */
#include "backtrace.h"

static struct backtrace_state *backtrace_state = NULL;

typedef struct {
    BUFFER *wb;       // Buffer to write to
    size_t frame_count; // Number of frames processed
    bool first_frame;   // Is this the first frame?
} backtrace_data_t;

// Common function to format and add a stack frame to the buffer
static void add_stack_frame(backtrace_data_t *bt_data, uintptr_t pc, const char *function,
                            const char *filename, int lineno) {
    BUFFER *wb = bt_data->wb;

    if (!wb)
        return;

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
        len = strcatz(error_buf, len, sizeof(error_buf), msg);

    // Add the error number description if available
    if (errnum > 0) {
        if (msg) {
            len = strcatz(error_buf, len, sizeof(error_buf), ": ");
        }
        len = strcatz(error_buf, len, sizeof(error_buf), strerror(errnum));
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

void capture_stack_trace(BUFFER *wb) {
    if (!backtrace_state) {
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "libbacktrace not initialized");
        return;
    }

    backtrace_data_t bt_data = {
        .wb = wb,
        .frame_count = 0,
        .first_frame = true
    };

    // Skip one frame to hide capture_stack_trace() itself
    backtrace_full(backtrace_state, 1, bt_full_handler,
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

void capture_stack_trace(BUFFER *wb) {
    // this function is async-signal-safe, if the buffer has enough space to hold the stack trace

    unw_cursor_t cursor;
    unw_context_t context;
    size_t frames = 0;

    // Initialize context for current thread
    unw_getcontext(&context);
    unw_init_local(&cursor, &context);

    size_t added = 0;
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
    // Format the stack trace (removing the address part)
    for (i = 0; i < size; i++) {
        if(messages[i] && *messages[i]) {
            if(added)
                buffer_putc(wb, '\n');

            buffer_putc(wb, '#');
            buffer_print_uint64(wb, i);
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

void capture_stack_trace(BUFFER *wb) {
    buffer_strcat(wb, NO_STACK_TRACE_PREFIX "no back-end available");

    // probably we can have something like this?
    // https://maskray.me/blog/2022-04-09-unwinding-through-signal-handler
    // (at the end - but it needs the frame pointer)
}

#endif

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

    capture_stack_trace(wb);

    in_stack_trace = false; // Ensure the flag is reset
    return true;
}
