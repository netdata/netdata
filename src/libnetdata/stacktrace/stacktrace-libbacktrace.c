// SPDX-License-Identifier: GPL-3.0-or-later

#include "stacktrace-common.h"

#if defined(USE_LIBBACKTRACE)
#include "backtrace.h"

static struct backtrace_state *backtrace_state = NULL;

typedef struct {
    BUFFER *wb;             // Buffer to write to
    size_t frame_count;     // Number of frames processed
    bool first_frame;       // Is this the first frame?
    bool found_signal_handler;  // Have we found the signal handler frame?
} backtrace_data_t;

// For collecting raw frames
typedef struct {
    void **frames;
    int max_frames;
    int num_frames;
    int skip_frames;
} collect_frames_data_t;

// Simple callback for collecting PC addresses
static int bt_collect_frames_callback(void *data, uintptr_t pc) {
    collect_frames_data_t *cf_data = (collect_frames_data_t *)data;
    
    // Skip frames at the top of the stack
    if (cf_data->skip_frames > 0) {
        cf_data->skip_frames--;
        return 0;
    }
    
    if (cf_data->num_frames < cf_data->max_frames) {
        cf_data->frames[cf_data->num_frames++] = (void *)pc;
    }
    
    return 0;
}

// Common function to format and add a stack frame to the buffer
static void add_stack_frame(backtrace_data_t *bt_data, uintptr_t pc, const char *function,
                          const char *filename, int lineno) {
    BUFFER *wb = bt_data->wb;

    if (!wb)
        return;

    // Check if we found the signal handler frame
    if (!bt_data->found_signal_handler && stacktrace_is_signal_handler_function(function)) {
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
    if (!bt_data->found_signal_handler && stacktrace_is_logging_function(function)) {
        // Found a logging function, reset the buffer and clear function name
        buffer_flush(wb);
        bt_data->frame_count = 0;
        bt_data->first_frame = true;
        root_cause_function[0] = '\0';
        // continue to add the function to the stack trace
    }

    // Check if this is a netdata source file and store the function name if it is
    // (but only if we haven't already stored one)
    if (!root_cause_function[0] && stacktrace_is_netdata_function(function, filename))
        stacktrace_keep_first_root_cause_function(function);

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

const char *stacktrace_backend(void) {
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

void impl_stacktrace_init(void) {
    if (!backtrace_state) {
        backtrace_state = backtrace_create_state(NULL, BACKTRACE_SUPPORTS_THREADS,
                                               bt_error_handler, NULL);
    }
}

void stacktrace_flush(void) {
    // Nothing to flush with libbacktrace
}

bool stacktrace_capture_is_async_signal_safe(void) {
// libbacktrace may use malloc depending on configuration
// Check the BACKTRACE_USES_MALLOC define
#if BACKTRACE_USES_MALLOC
    return false;
#else
    return true;
#endif
}

bool stacktrace_available(void) {
    return backtrace_state != NULL;
}

NEVER_INLINE
void stacktrace_capture(BUFFER *wb) {
    root_cause_function[0] = '\0';

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

    // Skip one frame to hide stacktrace_capture() itself
    backtrace_full(backtrace_state, 1, bt_full_handler,
                 bt_error_handler, &bt_data);

    // If no frames were reported
    if (bt_data.frame_count == 0) {
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "libbacktrace reports no frames");
    }
}

// Implementation-specific function to collect stack trace frames
NEVER_INLINE
int impl_stacktrace_get_frames(void **frames, int max_frames, int skip_frames) {
    if (!backtrace_state || !frames || max_frames <= 0)
        return 0;
    
    // Collect frames
    collect_frames_data_t data = {
        .frames = frames,
        .max_frames = max_frames,
        .num_frames = 0,
        .skip_frames = skip_frames + 1  // +1 to also skip this function itself
    };
    
    // Pass 0 as skip_frames to backtrace_simple and let the callback handle skipping
    backtrace_simple(backtrace_state, 0, bt_collect_frames_callback, bt_error_handler, &data);
    
    return data.num_frames;
}

// Implementation-specific function to convert a stacktrace to a buffer
void impl_stacktrace_to_buffer(STACKTRACE trace, BUFFER *wb) {
    struct stacktrace *st = (struct stacktrace *)trace;
    
    // Resolve each frame
    backtrace_data_t bt_data = {
        .wb = wb,
        .frame_count = 0,
        .first_frame = true,
        .found_signal_handler = false
    };
    
    for (int i = 0; i < st->frame_count; i++) {
        backtrace_pcinfo(
            backtrace_state,
            (uintptr_t)st->frames[i],
            bt_full_handler,
            bt_error_handler,
            &bt_data
        );
    }
    
    // If we couldn't resolve any frames, use addresses
    if (bt_data.frame_count == 0) {
        for (int i = 0; i < st->frame_count; i++) {
            if (i > 0)
                buffer_putc(wb, '\n');
                
            buffer_putc(wb, '#');
            buffer_print_uint64(wb, i);
            buffer_strcat(wb, " <unknown> [");
            buffer_print_uint64_hex(wb, (uint64_t)st->frames[i]);
            buffer_putc(wb, ']');
        }
    }
}

#endif // USE_LIBBACKTRACE
