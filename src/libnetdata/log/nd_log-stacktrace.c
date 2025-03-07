// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

bool nd_log_forked = false;

#define NO_STACK_TRACE_PREFIX "stack trace not available: "

#if defined(HAVE_LIBUNWIND)
#include <libunwind.h>

void capture_stack_trace(BUFFER *wb) {
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
        if (unw_get_proc_name(&cursor, sym, sizeof(sym), &offset) == 0) {
            if (frames++) buffer_strcat(wb, "\n");
            buffer_sprintf(wb, "#%d %s+0x%lx", added, name, (unsigned long)offset);
        }
        else {
            if (frames++)
                buffer_strcat(wb, "\n");
            buffer_sprintf(wb, "#%d <unknown>", added);
        }

        added++;
    }

    if (!added)
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "libunwind reports no frames");
}

#elif defined(HAVE_BACKTRACE)

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
            char *p = strstr(messages[i], " [");
            size_t len = p ? (size_t)(p - messages[i]) : strlen(messages[i]);
            buffer_sprintf(wb, "#%d %.*s\n", i, (int)len, messages[i]);
            added++;
        }
    }

    if(!added)
        buffer_strcat(wb, NO_STACK_TRACE_PREFIX "backtrace() reports no frames");

    free(messages);
}

#else

void capture_stack_trace(BUFFER *wb) {
    buffer_strcat(wb, NO_STACK_TRACE_PREFIX "no back-end available");
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
