// SPDX-License-Identifier: GPL-3.0-or-later

#include "stacktrace-common.h"

#if defined(USE_LIBUNWIND)
#if !defined(STATIC_BUILD)
#define UNW_LOCAL_ONLY
#endif
#include <libunwind.h>
#include <dlfcn.h>

const char *stacktrace_capture_backend(void) {
    return "libunwind";
}

void impl_stacktrace_init(void) {
    unw_set_caching_policy(unw_local_addr_space, UNW_CACHE_NONE);
}

void stacktrace_flush(void) {
    unw_flush_cache(unw_local_addr_space, 0, 0);
}

bool stacktrace_capture_is_async_signal_safe(void) {
#if defined(STATIC_BUILD)
    return false;
#else
    return true;
#endif
}

bool stacktrace_available(void) {
    return true;
}

NEVER_INLINE
void stack_trace_capture(BUFFER *wb) {
    // this function is async-signal-safe, if the buffer has enough space to hold the stack trace

    root_cause_function[0] = '\0';

    unw_cursor_t cursor;
    unw_context_t context;
    size_t frames = 0;

    // Initialize context for current thread
    unw_getcontext(&context);
    unw_init_local(&cursor, &context);
    
    // Skip one frame to hide stacktrace_capture() itself
    unw_step(&cursor);

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
        if (!found_signal_handler && stacktrace_is_signal_handler_function(name)) {
            // We found the signal handler, reset the buffer
            buffer_flush(wb);
            added = 0;
            frames = 0;
            found_signal_handler = true;
            continue; // Skip adding the signal handler itself
        }
        
        // Check for logging functions, but only if we haven't found a signal handler yet
        if (!found_signal_handler && stacktrace_is_logging_function(name)) {
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

// Collect frames using libunwind
static int collect_frames_libunwind(void **frames, int max_frames, int skip) {
    unw_cursor_t cursor;
    unw_context_t context;
    
    unw_getcontext(&context);
    unw_init_local(&cursor, &context);
    
    // Collect all frames first, including the ones we'll skip
    // This way we ensure we capture all frames, including inlined ones
    void *all_frames[150]; // Allocate a larger buffer to hold all frames
    int total_frames = 0;
    
    // Collect as many frames as possible
    while (total_frames < 150 && unw_step(&cursor) > 0) {
        unw_word_t pc;
        unw_get_reg(&cursor, UNW_REG_IP, &pc);
        if (!pc)
            break;
            
        all_frames[total_frames++] = (void *)pc;
    }
    
    // Now copy the frames we want, skipping the requested number
    int frame_count = 0;
    for (int i = skip; i < total_frames && frame_count < max_frames; i++) {
        frames[frame_count++] = all_frames[i];
    }
    
    return frame_count;
}

// Implementation-specific function to collect stack trace frames
int impl_stacktrace_get_frames(void **frames, int max_frames, int skip_frames) {
    if (!frames || max_frames <= 0)
        return 0;
    
    // Add 1 to skip_frames to also skip this function itself
    return collect_frames_libunwind(frames, max_frames, skip_frames + 1);
}

// Implementation-specific function to convert a stacktrace to a buffer
void impl_stacktrace_to_buffer(STACKTRACE trace, BUFFER *wb) {
    struct stacktrace *st = (struct stacktrace *)trace;
    
    // Format each frame
    for (int i = 0; i < st->frame_count; i++) {
        if (i > 0)
            buffer_putc(wb, '\n');
            
        buffer_putc(wb, '#');
        buffer_print_uint64(wb, i);
        buffer_putc(wb, ' ');
        
        // Try to resolve symbol name
        unw_cursor_t cursor;
        unw_context_t context;
        char sym[256] = "<unknown>";
        unw_word_t offset = 0;
        
        // We don't have the context anymore, so do the best we can
        // Try to resolve the address to a symbol name
        if (dladdr(st->frames[i], (Dl_info *)sym) == 0) {
            buffer_strcat(wb, "<unknown>");
        } else {
            Dl_info *info = (Dl_info *)sym;
            if (info->dli_sname)
                buffer_strcat(wb, info->dli_sname);
            else
                buffer_strcat(wb, "<unknown>");
        }
        
        buffer_strcat(wb, " [");
        buffer_print_uint64_hex(wb, (uint64_t)st->frames[i]);
        buffer_putc(wb, ']');
    }
}

#endif // USE_LIBUNWIND
