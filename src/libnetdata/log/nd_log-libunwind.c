// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

bool nd_log_forked = false;

#ifdef HAVE_LIBUNWIND
#include <libunwind.h>

bool stack_trace_formatter(BUFFER *wb, void *data __maybe_unused) {
    static __thread bool in_stack_trace = false;

    if (nd_log_forked) {
        // libunwind freezes in forked children
        buffer_strcat(wb, "stack trace after fork is disabled");
        return true;
    }

    if (in_stack_trace) {
        // Prevent recursion
        buffer_strcat(wb, "stack trace recursion detected");
        return true;
    }

    in_stack_trace = true;

    unw_cursor_t cursor;
    unw_context_t context;
    size_t frames = 0;

    // Initialize context for current thread
    unw_getcontext(&context);
    unw_init_local(&cursor, &context);

    // Skip first 3 frames (our logging infrastructure)
    for (int i = 0; i < 3; i++) {
        if (unw_step(&cursor) <= 0)
            goto cleanup; // Ensure proper cleanup if unwinding fails early
    }

    while (unw_step(&cursor) > 0) {
        unw_word_t offset, pc;
        char sym[256];

        unw_get_reg(&cursor, UNW_REG_IP, &pc);
        if (pc == 0)
            break;

        const char *name = sym;
        if (unw_get_proc_name(&cursor, sym, sizeof(sym), &offset) == 0) {
            if (frames++) buffer_strcat(wb, "\n");
            buffer_sprintf(wb, "%s+0x%lx", name, (unsigned long)offset);
        }
        else {
            if (frames++)
                buffer_strcat(wb, "\n");
            buffer_strcat(wb, "<unknown>");
        }
    }

cleanup:
    in_stack_trace = false; // Ensure the flag is reset
    return true;
}

#else // !HAVE_LIBUNWIND

bool stack_trace_formatter(BUFFER *wb, void *data __maybe_unused) {
    buffer_strcat(wb, "libunwind not available");
    return true;
}

#endif
