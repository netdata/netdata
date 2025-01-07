// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#ifdef HAVE_LIBUNWIND
#include <libunwind.h>

const char *stack_trace_annotator(struct log_field *lf __maybe_unused) {
    static __thread char stack[4096];
    static __thread bool in_stack_trace = false;

    // prevent recursion
    if(in_stack_trace)
        return "stack trace recursion detected";

    in_stack_trace = true;

    unw_cursor_t cursor;
    unw_context_t context;
    char *d = stack;
    size_t frames = 0;

    // Initialize context for current thread
    unw_getcontext(&context);
    unw_init_local(&cursor, &context);

    // Skip first 3 frames (our annotator and the logging infrastructure)
    unw_step(&cursor);
    unw_step(&cursor);
    unw_step(&cursor);

    while (unw_step(&cursor) > 0) {
        unw_word_t offset, pc;
        char sym[256];

        unw_get_reg(&cursor, UNW_REG_IP, &pc);
        if (pc == 0)
            break;

        const char *name = sym;
        if (unw_get_proc_name(&cursor, sym, sizeof(sym), &offset) == 0) {
            if(frames++)
                d += snprintfz(d, sizeof(stack) - (d - stack), "\n");
            d += snprintfz(d, sizeof(stack) - (d - stack), "%s+0x%lx", name, (unsigned long)offset);
        }
        else {
            if(frames++)
                d += snprintfz(d, sizeof(stack) - (d - stack), "\n");
            d += snprintfz(d, sizeof(stack) - (d - stack), "<unknown>");
        }
    }

    in_stack_trace = false;
    return stack;
}

#else
bool stack_trace_annotator(struct log_field *lf __maybe_unused) {
    return "libunwind not available";
}
#endif
