// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

bool nd_log_forked = false;

#define NO_STACK_TRACE_PREFIX "stack trace not available: "

#if defined(HAVE_LIBUNWIND)
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

#if defined(OS_MACOS)
            // remove the address part
            char *p = strstr(messages[i], "0x");
            char *e = p ? strchr(p, ' ') : NULL;
            if (e) {
                e++;
                buffer_putc(wb, '#');
                buffer_print_uint64(wb, added);
                buffer_putc(wb, ' ');
                buffer_strcat(wb, e);
            }
            else
                buffer_strcat(wb, messages[i]);
#else
            buffer_putc(wb, '#');
            buffer_print_uint64(wb, i);
            buffer_putc(wb, ' ');

            // remove the address part
            char *p = strstr(messages[i], " [");
            size_t len = p ? (size_t)(p - messages[i]) : strlen(messages[i]);
            buffer_fast_strcat(wb, messages[i], len);
#endif
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
